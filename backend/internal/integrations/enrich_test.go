package integrations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newPlexMockServer creates a mock Plex server that returns the given movies and shows.
func newPlexMockServer(t *testing.T, movies []plexMockItem, shows []plexMockItem) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`{"MediaContainer":{"machineIdentifier":"test","version":"1.0"}}`))
		case "/library/sections":
			dirs := []map[string]string{}
			if len(movies) > 0 {
				dirs = append(dirs, map[string]string{"key": "1", "title": "Movies", "type": "movie"})
			}
			if len(shows) > 0 {
				dirs = append(dirs, map[string]string{"key": "2", "title": "TV Shows", "type": "show"})
			}
			resp := map[string]any{
				"MediaContainer": map[string]any{
					"Directory": dirs,
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode sections: %v", err)
			}
		case testPlexPathMoviesAll:
			metadata := make([]map[string]any, len(movies))
			for i, m := range movies {
				metadata[i] = map[string]any{
					"ratingKey":    m.RatingKey,
					"title":        m.Title,
					"year":         m.Year,
					"type":         "movie",
					"viewCount":    m.ViewCount,
					"lastViewedAt": m.LastViewedAt,
				}
			}
			resp := map[string]any{
				"MediaContainer": map[string]any{
					"Metadata": metadata,
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode movies: %v", err)
			}
		case "/library/sections/2/all":
			metadata := make([]map[string]any, len(shows))
			for i, s := range shows {
				metadata[i] = map[string]any{
					"ratingKey":    s.RatingKey,
					"title":        s.Title,
					"year":         s.Year,
					"type":         "show",
					"viewCount":    s.ViewCount,
					"lastViewedAt": s.LastViewedAt,
				}
			}
			resp := map[string]any{
				"MediaContainer": map[string]any{
					"Metadata": metadata,
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode shows: %v", err)
			}
		case "/library/onDeck":
			// Return empty on-deck for existing watch-data tests
			resp := map[string]any{
				"MediaContainer": map[string]any{
					"Metadata": []any{},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode on-deck: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

type plexMockItem struct {
	RatingKey    string
	Title        string
	Year         int
	ViewCount    int
	LastViewedAt int64
}

func TestEnrichItems_PlexEnrichment(t *testing.T) {
	srv := newPlexMockServer(t,
		[]plexMockItem{
			{RatingKey: "101", Title: "Serenity", Year: 2010, ViewCount: 5, LastViewedAt: 1700000000},
			{RatingKey: "102", Title: "Serenity 2", Year: 2014, ViewCount: 3, LastViewedAt: 1699000000},
		},
		[]plexMockItem{
			{RatingKey: "200", Title: "Firefly", Year: 2008, ViewCount: 10, LastViewedAt: 1700000000},
		},
	)
	defer srv.Close()

	plexClient := NewPlexClient(srv.URL, "test-token")

	// Simulate *arr items with no watch data
	items := []MediaItem{
		{Title: "Serenity", Type: MediaTypeMovie, ExternalID: "1"},
		{Title: "Serenity 2", Type: MediaTypeMovie, ExternalID: "2"},
		{Title: "Firefly", Type: MediaTypeShow, ExternalID: "3"},
		{Title: "Firefly 2", Type: MediaTypeShow, ExternalID: "4"}, // not in Plex
	}

	ec := EnrichmentClients{Plex: plexClient}
	EnrichItems(items, ec)

	// Serenity should be enriched
	if items[0].PlayCount != 5 {
		t.Errorf("Serenity: expected PlayCount 5, got %d", items[0].PlayCount)
	}
	if items[0].LastPlayed == nil {
		t.Error("Serenity: expected LastPlayed to be set")
	}

	// Serenity 2 should be enriched
	if items[1].PlayCount != 3 {
		t.Errorf("Serenity 2: expected PlayCount 3, got %d", items[1].PlayCount)
	}

	// Firefly should be enriched
	if items[2].PlayCount != 10 {
		t.Errorf("Firefly: expected PlayCount 10, got %d", items[2].PlayCount)
	}

	// Firefly 2 should NOT be enriched (not in Plex)
	if items[3].PlayCount != 0 {
		t.Errorf("Firefly 2: expected PlayCount 0, got %d", items[3].PlayCount)
	}
	if items[3].LastPlayed != nil {
		t.Error("Firefly 2: expected LastPlayed to be nil")
	}
}

func TestEnrichItems_PlexEnrichment_SeasonMatchesByShowTitle(t *testing.T) {
	srv := newPlexMockServer(t,
		nil,
		[]plexMockItem{
			{RatingKey: "200", Title: "Firefly", Year: 2008, ViewCount: 10, LastViewedAt: 1700000000},
		},
	)
	defer srv.Close()

	plexClient := NewPlexClient(srv.URL, "test-token")

	// Simulate a season item from Sonarr — Season should match via ShowTitle
	items := []MediaItem{
		{
			Title:     "Season 2",
			ShowTitle: "Firefly",
			Type:      MediaTypeSeason,
		},
	}

	ec := EnrichmentClients{Plex: plexClient}
	EnrichItems(items, ec)

	if items[0].PlayCount != 10 {
		t.Errorf("Season 2: expected PlayCount 10 via ShowTitle match, got %d", items[0].PlayCount)
	}
}

func TestEnrichItems_TautulliTakesPriorityOverPlex(t *testing.T) {
	srv := newPlexMockServer(t,
		[]plexMockItem{
			{RatingKey: "101", Title: "Serenity", Year: 2010, ViewCount: 5, LastViewedAt: 1700000000},
		},
		nil,
	)
	defer srv.Close()

	plexClient := NewPlexClient(srv.URL, "test-token")

	// Simulate an item already enriched by Tautulli (PlayCount > 0)
	tautulliTime := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	items := []MediaItem{
		{
			Title:      "Serenity",
			Type:       MediaTypeMovie,
			ExternalID: "1",
			PlayCount:  20,            // Already set by Tautulli
			LastPlayed: &tautulliTime, // Already set by Tautulli
		},
	}

	ec := EnrichmentClients{Plex: plexClient}
	EnrichItems(items, ec)

	// Tautulli data should NOT be overwritten by Plex (PlayCount > 0 guard)
	if items[0].PlayCount != 20 {
		t.Errorf("Expected Tautulli PlayCount 20 to be preserved, got %d", items[0].PlayCount)
	}
	if !items[0].LastPlayed.Equal(tautulliTime) {
		t.Errorf("Expected Tautulli LastPlayed to be preserved, got %v", items[0].LastPlayed)
	}
}

func TestEnrichItems_PlexWatchlistEnrichment(t *testing.T) {
	// Mock Plex server: onDeck returns Serenity and Firefly episodes
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`{"MediaContainer":{"machineIdentifier":"test","version":"1.0"}}`))
		case "/library/onDeck":
			resp := map[string]any{
				"MediaContainer": map[string]any{
					"Metadata": []map[string]any{
						{"ratingKey": "101", "title": "Serenity", "type": "movie"},
						{"ratingKey": "301", "title": "The Train Job", "type": "episode", "grandparentTitle": "Firefly"},
					},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode on-deck: %v", err)
			}
		case "/library/sections":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	plexClient := NewPlexClient(srv.URL, "test-token")

	items := []MediaItem{
		{Title: "Serenity", Type: MediaTypeMovie, ExternalID: "1"},
		{Title: "Firefly", Type: MediaTypeShow, ExternalID: "2"},
		{Title: "Firefly 2", Type: MediaTypeShow, ExternalID: "3"}, // not on deck
	}

	ec := EnrichmentClients{Plex: plexClient}
	EnrichItems(items, ec)

	if !items[0].OnWatchlist {
		t.Error("Serenity: expected OnWatchlist=true (on Plex on-deck)")
	}
	if !items[1].OnWatchlist {
		t.Error("Firefly: expected OnWatchlist=true (episode on Plex on-deck)")
	}
	if items[2].OnWatchlist {
		t.Error("Firefly 2: expected OnWatchlist=false (not on deck)")
	}
}

func TestEnrichItems_JellyfinFavoritesEnrichment(t *testing.T) {
	// Mock Jellyfin server: admin user + favorites
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/Users":
			_, _ = w.Write([]byte(`[{"Id":"admin-1","Name":"admin","Policy":{"IsAdministrator":true}}]`))
		case "/Users/admin-1/Items":
			if r.URL.Query().Get("IsFavorite") == "true" {
				_, _ = w.Write([]byte(`{
					"Items": [{"Id":"1","Name":"Serenity","Type":"Movie"},{"Id":"2","Name":"Firefly","Type":"Series"}],
					"TotalRecordCount": 2
				}`))
			} else {
				_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	jellyfinClient := NewJellyfinClient(srv.URL, "test-key")

	items := []MediaItem{
		{Title: "Serenity", Type: MediaTypeMovie, ExternalID: "1"},
		{Title: "Firefly", Type: MediaTypeShow, ExternalID: "2"},
		{Title: "Firefly 2", Type: MediaTypeShow, ExternalID: "3"}, // not favorited
	}

	ec := EnrichmentClients{Jellyfin: jellyfinClient}
	EnrichItems(items, ec)

	if !items[0].OnWatchlist {
		t.Error("Serenity: expected OnWatchlist=true (Jellyfin favorite)")
	}
	if !items[1].OnWatchlist {
		t.Error("Firefly: expected OnWatchlist=true (Jellyfin favorite)")
	}
	if items[2].OnWatchlist {
		t.Error("Firefly 2: expected OnWatchlist=false (not favorited)")
	}
}

func TestEnrichItems_WatchlistPriorityPlexOverJellyfin(t *testing.T) {
	// Mock Plex: only Serenity on deck
	plexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/library/onDeck":
			resp := map[string]any{
				"MediaContainer": map[string]any{
					"Metadata": []map[string]any{
						{"ratingKey": "101", "title": "Serenity", "type": "movie"},
					},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode: %v", err)
			}
		case "/library/sections":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer plexSrv.Close()

	// Mock Jellyfin: Serenity AND Firefly favorited
	jellyfinSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/Users":
			_, _ = w.Write([]byte(`[{"Id":"admin-1","Name":"admin","Policy":{"IsAdministrator":true}}]`))
		case "/Users/admin-1/Items":
			if r.URL.Query().Get("IsFavorite") == "true" {
				_, _ = w.Write([]byte(`{
					"Items": [{"Id":"1","Name":"Serenity","Type":"Movie"},{"Id":"2","Name":"Firefly","Type":"Series"}],
					"TotalRecordCount": 2
				}`))
			} else {
				_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer jellyfinSrv.Close()

	plexClient := NewPlexClient(plexSrv.URL, "test-token")
	jellyfinClient := NewJellyfinClient(jellyfinSrv.URL, "test-key")

	items := []MediaItem{
		{Title: "Serenity", Type: MediaTypeMovie, ExternalID: "1"},
		{Title: "Firefly", Type: MediaTypeShow, ExternalID: "2"},
		{Title: "Firefly 2", Type: MediaTypeShow, ExternalID: "3"}, // neither source
	}

	ec := EnrichmentClients{Plex: plexClient, Jellyfin: jellyfinClient}
	EnrichItems(items, ec)

	// Serenity: on Plex deck AND Jellyfin favorite — should be true
	if !items[0].OnWatchlist {
		t.Error("Serenity: expected OnWatchlist=true")
	}
	// Firefly: only Jellyfin favorite — should still be true (merged sources)
	if !items[1].OnWatchlist {
		t.Error("Firefly: expected OnWatchlist=true (from Jellyfin)")
	}
	// Firefly 2: neither source
	if items[2].OnWatchlist {
		t.Error("Firefly 2: expected OnWatchlist=false")
	}
}

func TestEnrichItems_PlexDoesNotOverwriteExistingData(t *testing.T) {
	srv := newPlexMockServer(t,
		[]plexMockItem{
			{RatingKey: "101", Title: "Serenity", Year: 2010, ViewCount: 5, LastViewedAt: 1700000000},
		},
		nil,
	)
	defer srv.Close()

	plexClient := NewPlexClient(srv.URL, "test-token")

	// Simulate an item with existing play data from another enrichment source
	existingTime := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	items := []MediaItem{
		{
			Title:      "Serenity",
			Type:       MediaTypeMovie,
			ExternalID: "1",
			PlayCount:  3,
			LastPlayed: &existingTime,
		},
	}

	ec := EnrichmentClients{Plex: plexClient}
	EnrichItems(items, ec)

	// Existing data should be preserved (PlayCount != 0 guard)
	if items[0].PlayCount != 3 {
		t.Errorf("Expected existing PlayCount 3 to be preserved, got %d", items[0].PlayCount)
	}
	if !items[0].LastPlayed.Equal(existingTime) {
		t.Errorf("Expected existing LastPlayed to be preserved, got %v", items[0].LastPlayed)
	}
}

func TestEnrichItems_OverseerrEnrichment(t *testing.T) {
	// Mock Overseerr server: returns two requests keyed by TMDb ID
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/request":
			resp := map[string]any{
				"pageInfo": map[string]any{"pages": 1, "page": 1, "results": 2},
				"results": []map[string]any{
					{
						"id":     1,
						"status": 2,
						"type":   "movie",
						"media": map[string]any{
							"tmdbId":    16320,
							"mediaType": "movie",
						},
						"requestedBy": map[string]any{
							"displayName": "RJ",
							"username":    "rj",
						},
					},
					{
						"id":     2,
						"status": 4,
						"type":   "tv",
						"media": map[string]any{
							"tmdbId":    1437,
							"mediaType": "tv",
						},
						"requestedBy": map[string]any{
							"displayName": "Mal",
							"username":    "mal",
						},
					},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode overseerr response: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	overseerrClient := NewOverseerrClient(srv.URL, "test-key")

	// Items with TMDb IDs set (as would happen after the fix)
	items := []MediaItem{
		{Title: "Serenity", Type: MediaTypeMovie, ExternalID: "1", TMDbID: 16320},
		{Title: "Firefly - Season 1", ShowTitle: "Firefly", Type: MediaTypeSeason, ExternalID: "2-s1", TMDbID: 1437},
		{Title: "Firefly", Type: MediaTypeShow, ExternalID: "2", TMDbID: 1437},
		{Title: "Firefly 2", Type: MediaTypeShow, ExternalID: "3", TMDbID: 0}, // No TMDb ID — should NOT be enriched
	}

	ec := EnrichmentClients{Overseerr: overseerrClient}
	EnrichItems(items, ec)

	// Serenity: TMDb 16320 matches → IsRequested=true, RequestedBy="RJ"
	if !items[0].IsRequested {
		t.Error("Serenity: expected IsRequested=true")
	}
	if items[0].RequestedBy != "RJ" {
		t.Errorf("Serenity: expected RequestedBy 'RJ', got %q", items[0].RequestedBy)
	}
	if items[0].RequestCount != 1 {
		t.Errorf("Serenity: expected RequestCount 1, got %d", items[0].RequestCount)
	}

	// Firefly Season 1: TMDb 1437 matches → IsRequested=true, RequestedBy="Mal"
	if !items[1].IsRequested {
		t.Error("Firefly Season 1: expected IsRequested=true")
	}
	if items[1].RequestedBy != "Mal" {
		t.Errorf("Firefly Season 1: expected RequestedBy 'Mal', got %q", items[1].RequestedBy)
	}

	// Firefly show-level: same TMDb 1437 → also matched
	if !items[2].IsRequested {
		t.Error("Firefly: expected IsRequested=true")
	}
	if items[2].RequestedBy != "Mal" {
		t.Errorf("Firefly: expected RequestedBy 'Mal', got %q", items[2].RequestedBy)
	}

	// Firefly 2: TMDb 0 → should NOT be enriched
	if items[3].IsRequested {
		t.Error("Firefly 2: expected IsRequested=false (TMDbID is 0)")
	}
	if items[3].RequestedBy != "" {
		t.Errorf("Firefly 2: expected empty RequestedBy, got %q", items[3].RequestedBy)
	}
}

func TestEnrichItems_OverseerrWithWatchedByRequestor(t *testing.T) {
	// Mock Overseerr: "RJ" requested Serenity (TMDb 16320)
	overseerrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/request" {
			resp := map[string]any{
				"pageInfo": map[string]any{"pages": 1, "page": 1, "results": 1},
				"results": []map[string]any{
					{
						"id":     1,
						"status": 4,
						"type":   "movie",
						"media": map[string]any{
							"tmdbId":    16320,
							"mediaType": "movie",
						},
						"requestedBy": map[string]any{
							"displayName": "RJ",
							"username":    "rj",
						},
					},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("Failed to encode: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer overseerrSrv.Close()

	overseerrClient := NewOverseerrClient(overseerrSrv.URL, "test-key")

	// Item has TMDbID AND watch data from Tautulli where "RJ" watched it
	items := []MediaItem{
		{
			Title:          "Serenity",
			Type:           MediaTypeMovie,
			ExternalID:     "1",
			TMDbID:         16320,
			WatchedByUsers: []string{"RJ"},
		},
	}

	ec := EnrichmentClients{Overseerr: overseerrClient}
	EnrichItems(items, ec)

	// RequestedBy should be "RJ"
	if items[0].RequestedBy != "RJ" {
		t.Errorf("Expected RequestedBy 'RJ', got %q", items[0].RequestedBy)
	}
	// WatchedByRequestor cross-ref should fire because "RJ" is in WatchedByUsers
	if !items[0].WatchedByRequestor {
		t.Error("Expected WatchedByRequestor=true (RJ requested and watched)")
	}
}
