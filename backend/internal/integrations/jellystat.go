package integrations

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// JellystatClient provides access to the Jellystat API for enriched Jellyfin
// watch history. Jellystat is to Jellyfin what Tautulli is to Plex — a
// supplementary analytics service that provides per-user watch history, play
// counts, and activity tracking beyond what Jellyfin's native API exposes.
//
// Authentication uses Jellystat's API key mechanism. The key is stored in the
// standard APIKey field and sent as the `x-api-token` header.
type JellystatClient struct {
	URL    string
	APIKey string `json:"-"` // Jellystat API key (stored in IntegrationConfig.APIKey)
}

// NewJellystatClient creates a new Jellystat API client.
func NewJellystatClient(url, apiKey string) *JellystatClient {
	return &JellystatClient{
		URL:    strings.TrimRight(url, "/"),
		APIKey: apiKey,
	}
}

// doRequest executes a Jellystat API call using API key authentication.
func (j *JellystatClient) doRequest(endpoint string) ([]byte, error) {
	fullURL := j.URL + endpoint
	return DoAPIRequest(fullURL, "x-api-token", j.APIKey)
}

// TestConnection verifies the Jellystat URL and API key are valid by calling
// the libraries endpoint. On 401, returns a descriptive error about the API key.
func (j *JellystatClient) TestConnection() error {
	body, err := j.doRequest("/api/getLibraries")
	if err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			return fmt.Errorf("jellystat auth failed (check your API key — generate one in Jellystat Settings → API Keys)")
		}
		return err
	}

	// Verify we got a valid JSON array response
	var libraries []json.RawMessage
	if err := json.Unmarshal(body, &libraries); err != nil {
		return fmt.Errorf("failed to parse Jellystat response: %w", err)
	}

	return nil
}

// jellystatLibraryItem represents a single item from Jellystat's library items endpoint.
// Jellystat tracks Jellyfin Item IDs, so TMDb resolution requires a lookup map.
type jellystatLibraryItem struct {
	ID             string `json:"Id"`               // Jellyfin Item ID
	Name           string `json:"Name"`             // Title
	TotalPlayCount int    `json:"total_play_count"` // Total plays across all users
	TotalPlayed    string `json:"total_played"`     // Last played timestamp (ISO 8601)
	Users          []struct {
		UserName  string `json:"UserName"`
		PlayCount int    `json:"play_count"`
	} `json:"Users"`
}

// GetBulkWatchStats fetches all library items with watch statistics from Jellystat.
// Since Jellystat tracks items by Jellyfin Item ID, TMDb resolution requires the
// jellyfinIDToTMDbID map (built from JellyfinClient's ProviderIDs during the same
// poll cycle). Returns a map keyed by TMDb ID.
func (j *JellystatClient) GetBulkWatchStats(jellyfinIDToTMDbID map[string]int) (map[int]*WatchData, error) {
	if len(jellyfinIDToTMDbID) == 0 {
		slog.Debug("Jellystat bulk watch stats skipped — no Jellyfin ID→TMDb ID mappings available",
			"component", "jellystat")
		return make(map[int]*WatchData), nil
	}

	body, err := j.doRequest("/api/getItemsWithStats")
	if err != nil {
		return nil, fmt.Errorf("jellystat items: %w", err)
	}

	var items []jellystatLibraryItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("failed to parse Jellystat items: %w", err)
	}

	result := make(map[int]*WatchData)
	skippedNoMapping := 0

	for _, item := range items {
		tmdbID, ok := jellyfinIDToTMDbID[item.ID]
		if !ok || tmdbID == 0 {
			skippedNoMapping++
			continue
		}

		if item.TotalPlayCount == 0 {
			continue // No watch data to enrich with
		}

		wd := &WatchData{
			PlayCount: item.TotalPlayCount,
		}

		// Parse last played timestamp
		if item.TotalPlayed != "" {
			if t, err := time.Parse(time.RFC3339, item.TotalPlayed); err == nil {
				wd.LastPlayed = &t
			}
		}

		// Collect unique users who watched this item
		for _, u := range item.Users {
			if u.PlayCount > 0 && u.UserName != "" {
				wd.Users = append(wd.Users, u.UserName)
			}
		}

		// Merge: keep higher play count if duplicate TMDb ID (shouldn't happen normally)
		if existing, ok := result[tmdbID]; ok {
			if wd.PlayCount > existing.PlayCount {
				result[tmdbID] = wd
			}
		} else {
			result[tmdbID] = wd
		}
	}

	slog.Debug("Jellystat bulk watch stats fetched", "component", "jellystat",
		"totalItems", len(items), "resolved", len(result), "skippedNoMapping", skippedNoMapping)

	return result, nil
}

// Verify JellystatClient satisfies capability interfaces at compile time.
// Note: Jellystat uses bulk watch stats via the JellystatEnricher, not the
// WatchDataProvider interface, because it requires a Jellyfin ID→TMDb ID map
// that must be injected externally.
var _ Connectable = (*JellystatClient)(nil)
