package integrations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadarrClient_TestConnection_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/status" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != testTautulliAPIKey {
			t.Errorf("Missing or wrong API key header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"0.3.0"}`))
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	if err := client.TestConnection(); err != nil {
		t.Fatalf("TestConnection should succeed: %v", err)
	}
}

func TestReadarrClient_TestConnection_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, "bad-key")
	err := client.TestConnection()
	if err == nil {
		t.Fatal("TestConnection should fail with 401")
	}
}

func TestReadarrClient_TestConnection_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	err := client.TestConnection()
	if err == nil {
		t.Fatal("TestConnection should fail with 500")
	}
}

func TestReadarrClient_GetDiskSpace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/diskspace" {
			resp := []arrDiskSpace{
				{Path: "/media/books", TotalSpace: 500000000000, FreeSpace: 200000000000},
				{Path: "/media/audiobooks", TotalSpace: 1000000000000, FreeSpace: 750000000000},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode response: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	disks, err := client.GetDiskSpace()
	if err != nil {
		t.Fatalf("GetDiskSpace should succeed: %v", err)
	}
	if len(disks) != 2 {
		t.Fatalf("Expected 2 disks, got %d", len(disks))
	}
	if disks[0].Path != "/media/books" {
		t.Errorf("Expected path '/media/books', got %q", disks[0].Path)
	}
	if disks[0].TotalBytes != 500000000000 {
		t.Errorf("Expected TotalBytes 500000000000, got %d", disks[0].TotalBytes)
	}
	if disks[1].Path != "/media/audiobooks" {
		t.Errorf("Expected second path '/media/audiobooks', got %q", disks[1].Path)
	}
}

func TestReadarrClient_GetMediaItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case testLidarrPathQuality:
			_, _ = w.Write([]byte(`[{"id":1,"name":"eBook"},{"id":2,"name":"Audiobook"}]`))
		case testLidarrPathTag:
			_, _ = w.Write([]byte(`[{"id":1,"label":"sci-fi"},{"id":2,"label":"classic"}]`))
		case "/api/v1/book":
			resp := []readarrBook{
				{
					ID:               1,
					Title:            "Dune",
					AuthorID:         10,
					SizeOnDisk:       2000000,
					Added:            "2024-03-01T10:30:00Z",
					Monitored:        true,
					Path:             "/media/books/Dune",
					QualityProfileID: 1,
					Tags:             []int{1, 2},
					Genres:           []string{"Science Fiction", "Adventure"},
					Author: struct {
						AuthorName string `json:"authorName"`
					}{AuthorName: "Frank Herbert"},
					Ratings: struct {
						Value float64 `json:"value"`
					}{Value: 8.5},
				},
				{
					// Book with no file on disk — should be skipped
					ID:         2,
					Title:      "Empty Book",
					SizeOnDisk: 0,
				},
				{
					ID:               3,
					Title:            "Neuromancer",
					AuthorID:         20,
					SizeOnDisk:       1500000,
					Added:            "2024-04-15T08:00:00Z",
					Monitored:        false,
					Path:             "/media/books/Neuromancer",
					QualityProfileID: 2,
					Tags:             []int{1},
					Genres:           []string{"Cyberpunk"},
					Author: struct {
						AuthorName string `json:"authorName"`
					}{AuthorName: "William Gibson"},
					Ratings: struct {
						Value float64 `json:"value"`
					}{Value: 7.9},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	items, err := client.GetMediaItems()
	if err != nil {
		t.Fatalf("GetMediaItems should succeed: %v", err)
	}

	// Expect 2 books (Empty Book has SizeOnDisk=0)
	if len(items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(items))
	}

	// First book
	dune := items[0]
	if dune.Type != MediaTypeBook {
		t.Errorf("Expected MediaTypeBook, got %v", dune.Type)
	}
	if dune.Title != "Dune" {
		t.Errorf("Expected 'Dune', got %q", dune.Title)
	}
	if dune.ExternalID != "1" {
		t.Errorf("Expected ExternalID '1', got %q", dune.ExternalID)
	}
	if dune.SizeBytes != 2000000 {
		t.Errorf("Expected SizeBytes 2000000, got %d", dune.SizeBytes)
	}
	if dune.Path != "/media/books/Dune" {
		t.Errorf("Expected path '/media/books/Dune', got %q", dune.Path)
	}
	if !dune.Monitored {
		t.Error("Expected Dune to be monitored")
	}
	if dune.AddedAt == nil {
		t.Error("Expected non-nil AddedAt for Dune")
	}
	if dune.QualityProfile != "eBook" {
		t.Errorf("Expected quality profile 'eBook', got %q", dune.QualityProfile)
	}
	if dune.Rating != 8.5 {
		t.Errorf("Expected rating 8.5, got %f", dune.Rating)
	}
	if len(dune.Tags) != 2 || dune.Tags[0] != "sci-fi" || dune.Tags[1] != "classic" {
		t.Errorf("Expected tags [sci-fi, classic], got %v", dune.Tags)
	}
	if dune.Genre != "Science Fiction" {
		t.Errorf("Expected genre 'Science Fiction', got %q", dune.Genre)
	}

	// Second book
	neuro := items[1]
	if neuro.Title != "Neuromancer" {
		t.Errorf("Expected 'Neuromancer', got %q", neuro.Title)
	}
	if neuro.Monitored {
		t.Error("Expected Neuromancer to be unmonitored")
	}
	if neuro.QualityProfile != "Audiobook" {
		t.Errorf("Expected quality profile 'Audiobook', got %q", neuro.QualityProfile)
	}
	if len(neuro.Tags) != 1 || neuro.Tags[0] != "sci-fi" {
		t.Errorf("Expected tags [sci-fi], got %v", neuro.Tags)
	}
}

func TestReadarrClient_GetMediaItems_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case testLidarrPathQuality:
			_, _ = w.Write([]byte(`[]`))
		case testLidarrPathTag:
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/book":
			_, _ = w.Write([]byte(`{not valid json}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	_, err := client.GetMediaItems()
	if err == nil {
		t.Fatal("Expected error for malformed JSON")
	}
}

func TestReadarrClient_GetMediaItems_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case testLidarrPathQuality:
			_, _ = w.Write([]byte(`[]`))
		case testLidarrPathTag:
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/book":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	items, err := client.GetMediaItems()
	if err != nil {
		t.Fatalf("GetMediaItems should succeed with empty: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("Expected 0 items, got %d", len(items))
	}
}

func TestReadarrClient_GetRootFolders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/rootfolder" {
			resp := []struct {
				Path string `json:"path"`
			}{
				{Path: "/media/books"},
				{Path: "/media/audiobooks"},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	folders, err := client.GetRootFolders()
	if err != nil {
		t.Fatalf("GetRootFolders should succeed: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("Expected 2 folders, got %d", len(folders))
	}
	if folders[0] != "/media/books" {
		t.Errorf("Expected '/media/books', got %q", folders[0])
	}
	if folders[1] != "/media/audiobooks" {
		t.Errorf("Expected '/media/audiobooks', got %q", folders[1])
	}
}

func TestReadarrClient_GetQualityProfiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testLidarrPathQuality {
			resp := []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			}{
				{ID: 1, Name: "eBook"},
				{ID: 2, Name: "Audiobook"},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	profiles, err := client.GetQualityProfiles()
	if err != nil {
		t.Fatalf("GetQualityProfiles should succeed: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("Expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0].Value != "eBook" {
		t.Errorf("Expected 'eBook', got %q", profiles[0].Value)
	}
}

func TestReadarrClient_GetTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testLidarrPathTag {
			resp := []struct {
				ID    int    `json:"id"`
				Label string `json:"label"`
			}{
				{ID: 1, Label: "sci-fi"},
				{ID: 2, Label: "fantasy"},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	tags, err := client.GetTags()
	if err != nil {
		t.Fatalf("GetTags should succeed: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("Expected 2 tags, got %d", len(tags))
	}
	if tags[0].Value != "sci-fi" {
		t.Errorf("Expected 'sci-fi', got %q", tags[0].Value)
	}
}

func TestReadarrClient_GetLanguages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/language" {
			resp := []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			}{
				{ID: 1, Name: "English"},
				{ID: 2, Name: "French"},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL, testTautulliAPIKey)
	langs, err := client.GetLanguages()
	if err != nil {
		t.Fatalf("GetLanguages should succeed: %v", err)
	}
	if len(langs) != 2 {
		t.Fatalf("Expected 2 languages, got %d", len(langs))
	}
	if langs[0].Value != "English" {
		t.Errorf("Expected 'English', got %q", langs[0].Value)
	}
	if langs[1].Value != "French" {
		t.Errorf("Expected 'French', got %q", langs[1].Value)
	}
}

func TestReadarrClient_URLTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/status" {
			t.Errorf("Expected /api/v1/system/status, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewReadarrClient(srv.URL+"/", testTautulliAPIKey)
	if err := client.TestConnection(); err != nil {
		t.Fatalf("TestConnection should succeed: %v", err)
	}
}
