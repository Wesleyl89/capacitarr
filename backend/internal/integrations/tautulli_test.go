package integrations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testTautulliAPIKey = "test-key"

func TestTautulliClient_TestConnection_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tautulli uses query params for auth: ?apikey=XXX&cmd=CMD
		if r.URL.Path != "/api/v2" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		apiKey := r.URL.Query().Get("apikey")
		if apiKey != testTautulliAPIKey {
			t.Errorf("Expected apikey 'test-key', got %q", apiKey)
		}
		cmd := r.URL.Query().Get("cmd")
		if cmd != "get_tautulli_info" {
			t.Errorf("Expected cmd 'get_tautulli_info', got %q", cmd)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "success",
				"message": "",
				"data": {"tautulli_version": "2.13.0"}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	if err := client.TestConnection(); err != nil {
		t.Fatalf("TestConnection should succeed: %v", err)
	}
}

func TestTautulliClient_TestConnection_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, "bad-key")
	err := client.TestConnection()
	if err == nil {
		t.Fatal("TestConnection should fail with 401")
	}
}

func TestTautulliClient_TestConnection_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	err := client.TestConnection()
	if err == nil {
		t.Fatal("TestConnection should fail with 500")
	}
}

func TestTautulliClient_TestConnection_ErrorResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "error",
				"message": "Invalid apikey",
				"data": {}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, "bad-key")
	err := client.TestConnection()
	if err == nil {
		t.Fatal("TestConnection should fail when result is 'error'")
	}
	if !strings.Contains(err.Error(), "Invalid apikey") {
		t.Errorf("Expected error message to mention 'Invalid apikey', got: %v", err)
	}
}

func TestTautulliClient_TestConnection_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	err := client.TestConnection()
	if err == nil {
		t.Fatal("TestConnection should fail with malformed JSON")
	}
}

func TestTautulliClient_GetWatchHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cmd := r.URL.Query().Get("cmd")
		if cmd != "get_history" {
			t.Errorf("Expected cmd 'get_history', got %q", cmd)
		}
		ratingKey := r.URL.Query().Get("rating_key")
		if ratingKey != "12345" {
			t.Errorf("Expected rating_key '12345', got %q", ratingKey)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "success",
				"message": "",
				"data": {
					"recordsFiltered": 3,
					"recordsTotal": 3,
					"data": [
						{
							"date": 1703520000,
							"duration": 7200,
							"play_duration": 7100,
							"paused_counter": 120,
							"watched_status": 1,
							"user": "alice",
							"rating_key": "12345",
							"title": "Serenity",
							"media_type": "movie"
						},
						{
							"date": 1703606400,
							"duration": 7200,
							"play_duration": 6800,
							"paused_counter": 60,
							"watched_status": 1,
							"user": "bob",
							"rating_key": "12345",
							"title": "Serenity",
							"media_type": "movie"
						},
						{
							"date": 1703692800,
							"duration": 7200,
							"play_duration": 3500,
							"paused_counter": 0,
							"watched_status": 0,
							"user": "alice",
							"rating_key": "12345",
							"title": "Serenity",
							"media_type": "movie"
						}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	data, err := client.GetWatchHistory("12345")
	if err != nil {
		t.Fatalf("GetWatchHistory should succeed: %v", err)
	}

	if data.PlayCount != 3 {
		t.Errorf("Expected PlayCount 3, got %d", data.PlayCount)
	}
	// Total duration should be sum of play_duration: 7100 + 6800 + 3500 = 17400
	if data.TotalDuration != 17400 {
		t.Errorf("Expected TotalDuration 17400, got %d", data.TotalDuration)
	}
	// Should have 2 unique users: alice and bob
	if len(data.Users) != 2 {
		t.Errorf("Expected 2 unique users, got %d", len(data.Users))
	}
	if data.LastPlayed == nil {
		t.Fatal("Expected non-nil LastPlayed")
	}
	// Latest date is 1703692800
	if data.LastPlayed.Unix() != 1703692800 {
		t.Errorf("Expected LastPlayed Unix 1703692800, got %d", data.LastPlayed.Unix())
	}
}

func TestTautulliClient_GetWatchHistory_EmptyHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "success",
				"message": "",
				"data": {
					"recordsFiltered": 0,
					"recordsTotal": 0,
					"data": []
				}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	data, err := client.GetWatchHistory("99999")
	if err != nil {
		t.Fatalf("GetWatchHistory should succeed with empty: %v", err)
	}
	if data.PlayCount != 0 {
		t.Errorf("Expected PlayCount 0, got %d", data.PlayCount)
	}
	if data.TotalDuration != 0 {
		t.Errorf("Expected TotalDuration 0, got %d", data.TotalDuration)
	}
	if data.LastPlayed != nil {
		t.Error("Expected nil LastPlayed for empty history")
	}
	if len(data.Users) != 0 {
		t.Errorf("Expected 0 users, got %d", len(data.Users))
	}
}

func TestTautulliClient_GetWatchHistory_ErrorResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "error",
				"message": "Unable to retrieve history",
				"data": {}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	_, err := client.GetWatchHistory("12345")
	if err == nil {
		t.Fatal("GetWatchHistory should fail with error result")
	}
}

func TestTautulliClient_GetWatchHistory_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{broken json}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	_, err := client.GetWatchHistory("12345")
	if err == nil {
		t.Fatal("Expected error for malformed JSON")
	}
}

func TestTautulliClient_GetShowWatchHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cmd := r.URL.Query().Get("cmd")
		if cmd != "get_history" {
			t.Errorf("Expected cmd 'get_history', got %q", cmd)
		}
		gpRatingKey := r.URL.Query().Get("grandparent_rating_key")
		if gpRatingKey != "show42" {
			t.Errorf("Expected grandparent_rating_key 'show42', got %q", gpRatingKey)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "success",
				"message": "",
				"data": {
					"recordsFiltered": 2,
					"recordsTotal": 2,
					"data": [
						{
							"date": 1704067200,
							"play_duration": 2700,
							"user": "carol",
							"grandparent_rating_key": "show42",
							"title": "S01E01",
							"media_type": "episode"
						},
						{
							"date": 1704153600,
							"play_duration": 2800,
							"user": "carol",
							"grandparent_rating_key": "show42",
							"title": "S01E02",
							"media_type": "episode"
						}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	data, err := client.GetShowWatchHistory("show42")
	if err != nil {
		t.Fatalf("GetShowWatchHistory should succeed: %v", err)
	}

	if data.PlayCount != 2 {
		t.Errorf("Expected PlayCount 2, got %d", data.PlayCount)
	}
	// Total duration: 2700 + 2800 = 5500
	if data.TotalDuration != 5500 {
		t.Errorf("Expected TotalDuration 5500, got %d", data.TotalDuration)
	}
	// Only one unique user: carol
	if len(data.Users) != 1 {
		t.Errorf("Expected 1 unique user, got %d", len(data.Users))
	}
	if data.LastPlayed == nil {
		t.Fatal("Expected non-nil LastPlayed")
	}
	if data.LastPlayed.Unix() != 1704153600 {
		t.Errorf("Expected LastPlayed Unix 1704153600, got %d", data.LastPlayed.Unix())
	}
}

func TestTautulliClient_GetShowWatchHistory_ErrorResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "error",
				"message": "Unable to retrieve history",
				"data": {}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	_, err := client.GetShowWatchHistory("show42")
	if err == nil {
		t.Fatal("GetShowWatchHistory should fail with error result")
	}
}

func TestTautulliClient_URLTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2" {
			t.Errorf("Expected /api/v2, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "success",
				"message": "",
				"data": {}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL+"/", testTautulliAPIKey)
	if err := client.TestConnection(); err != nil {
		t.Fatalf("TestConnection should succeed: %v", err)
	}
}

func TestFlexString_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "string value", input: `"12345"`, want: "12345"},
		{name: "integer value", input: `12345`, want: "12345"},
		{name: "zero", input: `0`, want: "0"},
		{name: "empty string", input: `""`, want: ""},
		{name: "large integer", input: `9999999999`, want: "9999999999"},
		{name: "null becomes empty", input: `null`, want: ""},
		{name: "boolean rejects", input: `true`, wantErr: true},
		{name: "object rejects", input: `{}`, wantErr: true},
		{name: "array rejects", input: `[]`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fs flexString
			err := json.Unmarshal([]byte(tt.input), &fs)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %s, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %s: %v", tt.input, err)
			}
			if string(fs) != tt.want {
				t.Errorf("got %q, want %q", string(fs), tt.want)
			}
		})
	}
}

func TestTautulliClient_GetWatchHistory_NumericRatingKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Tautulli returns rating_key fields as bare integers in some versions.
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "success",
				"message": "",
				"data": {
					"recordsFiltered": 1,
					"recordsTotal": 1,
					"data": [
						{
							"date": 1703520000,
							"duration": 7200,
							"play_duration": 7100,
							"paused_counter": 120,
							"watched_status": 1,
							"user": "alice",
							"rating_key": 95295,
							"parent_rating_key": 95200,
							"grandparent_rating_key": 95000,
							"title": "Serenity",
							"media_type": "movie"
						}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	data, err := client.GetWatchHistory("95295")
	if err != nil {
		t.Fatalf("GetWatchHistory should succeed with numeric rating keys: %v", err)
	}

	if data.PlayCount != 1 {
		t.Errorf("Expected PlayCount 1, got %d", data.PlayCount)
	}
	if data.TotalDuration != 7100 {
		t.Errorf("Expected TotalDuration 7100, got %d", data.TotalDuration)
	}
}

func TestTautulliClient_GetShowWatchHistory_NumericRatingKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"result": "success",
				"message": "",
				"data": {
					"recordsFiltered": 1,
					"recordsTotal": 1,
					"data": [
						{
							"date": 1704067200,
							"play_duration": 2700,
							"user": "carol",
							"rating_key": 95296,
							"parent_rating_key": 95200,
							"grandparent_rating_key": 95000,
							"title": "Firefly S01E01",
							"media_type": "episode"
						}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := NewTautulliClient(srv.URL, testTautulliAPIKey)
	data, err := client.GetShowWatchHistory("95000")
	if err != nil {
		t.Fatalf("GetShowWatchHistory should succeed with numeric rating keys: %v", err)
	}

	if data.PlayCount != 1 {
		t.Errorf("Expected PlayCount 1, got %d", data.PlayCount)
	}
	if data.TotalDuration != 2700 {
		t.Errorf("Expected TotalDuration 2700, got %d", data.TotalDuration)
	}
}
