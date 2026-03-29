package integrations

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestJellyfinAddLabel verifies the Jellyfin read-modify-write label flow
// against a mocked HTTP server.
func TestJellyfinAddLabel(t *testing.T) {
	var postedBody []byte

	// Mock server that returns an item on GET and accepts POST
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Id": "admin-id", "Name": "admin", "Policy": map[string]interface{}{"IsAdministrator": true}},
			})
		case r.Method == "GET" && r.URL.Path == "/Users/admin-id/Items/item-123":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Id":   "item-123",
				"Name": "Firefly",
				"Type": "Series",
				"Tags": []string{"existing-tag"},
			})
		case r.Method == "POST" && r.URL.Path == "/Items/item-123":
			postedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewJellyfinClient(srv.URL, "test-api-key")
	if err := client.AddLabel("item-123", "capacitarr-sunset"); err != nil {
		t.Fatalf("AddLabel failed: %v", err)
	}

	// Verify the POST body includes the new tag
	if len(postedBody) == 0 {
		t.Fatal("Expected POST body, got empty")
	}
	var posted map[string]interface{}
	if err := json.Unmarshal(postedBody, &posted); err != nil {
		t.Fatalf("Failed to parse posted body: %v", err)
	}
	tags, ok := posted["Tags"].([]interface{})
	if !ok {
		t.Fatalf("Expected Tags array in posted body, got %T", posted["Tags"])
	}
	if len(tags) != 2 {
		t.Fatalf("Expected 2 tags, got %d: %v", len(tags), tags)
	}
	if tags[0] != "existing-tag" || tags[1] != "capacitarr-sunset" {
		t.Errorf("Expected tags [existing-tag, capacitarr-sunset], got %v", tags)
	}
}

// TestJellyfinRemoveLabel verifies label removal via the read-modify-write pattern.
func TestJellyfinRemoveLabel(t *testing.T) {
	var postedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Id": "admin-id", "Name": "admin", "Policy": map[string]interface{}{"IsAdministrator": true}},
			})
		case r.Method == "GET" && r.URL.Path == "/Users/admin-id/Items/item-123":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Id":   "item-123",
				"Name": "Firefly",
				"Type": "Series",
				"Tags": []string{"existing-tag", "capacitarr-sunset"},
			})
		case r.Method == "POST" && r.URL.Path == "/Items/item-123":
			postedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewJellyfinClient(srv.URL, "test-api-key")
	if err := client.RemoveLabel("item-123", "capacitarr-sunset"); err != nil {
		t.Fatalf("RemoveLabel failed: %v", err)
	}

	var posted map[string]interface{}
	if err := json.Unmarshal(postedBody, &posted); err != nil {
		t.Fatalf("Failed to parse posted body: %v", err)
	}
	tags, ok := posted["Tags"].([]interface{})
	if !ok {
		t.Fatalf("Expected Tags array in posted body, got %T", posted["Tags"])
	}
	if len(tags) != 1 {
		t.Fatalf("Expected 1 tag after removal, got %d: %v", len(tags), tags)
	}
	if tags[0] != "existing-tag" {
		t.Errorf("Expected remaining tag 'existing-tag', got %v", tags[0])
	}
}

// TestEmbyAddLabel verifies the Emby read-modify-write label flow.
func TestEmbyAddLabel(t *testing.T) {
	var postedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Id": "admin-id", "Name": "admin", "Policy": map[string]interface{}{"IsAdministrator": true}},
			})
		case r.Method == "GET" && r.URL.Path == "/Users/admin-id/Items/item-456":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Id": "item-456", "Name": "Serenity", "Type": "Movie", "Tags": []string{"existing"},
			})
		case r.Method == "POST" && r.URL.Path == "/Items/item-456":
			postedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewEmbyClient(srv.URL, "test-api-key")
	if err := client.AddLabel("item-456", "capacitarr-sunset"); err != nil {
		t.Fatalf("AddLabel failed: %v", err)
	}

	var posted map[string]interface{}
	if err := json.Unmarshal(postedBody, &posted); err != nil {
		t.Fatalf("Failed to parse posted body: %v", err)
	}
	tags, ok := posted["Tags"].([]interface{})
	if !ok || len(tags) != 2 {
		t.Fatalf("Expected 2 tags, got %v", posted["Tags"])
	}
}

// TestEmbyRemoveLabel verifies Emby label removal.
func TestEmbyRemoveLabel(t *testing.T) {
	var postedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Id": "admin-id", "Name": "admin", "Policy": map[string]interface{}{"IsAdministrator": true}},
			})
		case r.Method == "GET" && r.URL.Path == "/Users/admin-id/Items/item-456":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Id": "item-456", "Name": "Serenity", "Tags": []string{"existing", "capacitarr-sunset"},
			})
		case r.Method == "POST" && r.URL.Path == "/Items/item-456":
			postedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewEmbyClient(srv.URL, "test-api-key")
	if err := client.RemoveLabel("item-456", "capacitarr-sunset"); err != nil {
		t.Fatalf("RemoveLabel failed: %v", err)
	}

	var posted map[string]interface{}
	if err := json.Unmarshal(postedBody, &posted); err != nil {
		t.Fatalf("Failed to parse posted body: %v", err)
	}
	tags := posted["Tags"].([]interface{})
	if len(tags) != 1 || tags[0] != "existing" {
		t.Errorf("Expected [existing], got %v", tags)
	}
}

// TestPlexAddLabel verifies the Plex PUT-based label API.
func TestPlexAddLabel(t *testing.T) {
	var lastRequest *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastRequest = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewPlexClient(srv.URL, "test-token")
	if err := client.AddLabel("12345", "capacitarr-sunset"); err != nil {
		t.Fatalf("AddLabel failed: %v", err)
	}

	if lastRequest == nil {
		t.Fatal("Expected a request to be made")
	}
	if lastRequest.Method != "PUT" {
		t.Errorf("Expected PUT, got %s", lastRequest.Method)
	}
	if !strings.Contains(lastRequest.URL.RawQuery, "label") {
		t.Errorf("Expected label in query, got %s", lastRequest.URL.RawQuery)
	}
}

// TestPlexRemoveLabel verifies the Plex PUT-based label removal.
func TestPlexRemoveLabel(t *testing.T) {
	var lastRequest *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastRequest = r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewPlexClient(srv.URL, "test-token")
	if err := client.RemoveLabel("12345", "capacitarr-sunset"); err != nil {
		t.Fatalf("RemoveLabel failed: %v", err)
	}

	if lastRequest == nil {
		t.Fatal("Expected a request to be made")
	}
	if lastRequest.Method != "PUT" {
		t.Errorf("Expected PUT, got %s", lastRequest.Method)
	}
}

// TestJellyfinAddLabel_AlreadyPresent verifies idempotency — adding a label
// that already exists is a no-op (no POST issued).
func TestJellyfinAddLabel_AlreadyPresent(t *testing.T) {
	postCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Id": "admin-id", "Name": "admin", "Policy": map[string]interface{}{"IsAdministrator": true}},
			})
		case r.Method == "GET" && r.URL.Path == "/Users/admin-id/Items/item-123":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Id":   "item-123",
				"Name": "Firefly",
				"Tags": []string{"capacitarr-sunset"},
			})
		case r.Method == "POST":
			postCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewJellyfinClient(srv.URL, "test-api-key")
	if err := client.AddLabel("item-123", "capacitarr-sunset"); err != nil {
		t.Fatalf("AddLabel failed: %v", err)
	}
	if postCalled {
		t.Error("Expected no POST when label already exists")
	}
}

// TestJellyfinRemoveLabel_NotPresent verifies no-op when removing a label
// that doesn't exist on the item.
func TestJellyfinRemoveLabel_NotPresent(t *testing.T) {
	postCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/Users":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Id": "admin-id", "Name": "admin", "Policy": map[string]interface{}{"IsAdministrator": true}},
			})
		case r.Method == "GET" && r.URL.Path == "/Users/admin-id/Items/item-123":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Id":   "item-123",
				"Name": "Firefly",
				"Tags": []string{"other-tag"},
			})
		case r.Method == "POST":
			postCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewJellyfinClient(srv.URL, "test-api-key")
	if err := client.RemoveLabel("item-123", "capacitarr-sunset"); err != nil {
		t.Fatalf("RemoveLabel failed: %v", err)
	}
	if postCalled {
		t.Error("Expected no POST when label doesn't exist")
	}
}
