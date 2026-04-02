package integrations

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// flexString is a string type that can unmarshal from both JSON strings and
// JSON numbers. Tautulli's API returns rating_key fields as integers in some
// versions and strings in others; this type handles both transparently.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	// Try string first (the common/expected case).
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = flexString(s)
		return nil
	}

	// Fall back to number (Tautulli returns rating keys as integers).
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*f = flexString(n.String())
		return nil
	}

	return fmt.Errorf("flexString: cannot unmarshal %s", strconv.Quote(string(data)))
}

// flexInt64 is an int64 type that can unmarshal from JSON integers, floats,
// and string-encoded numbers. APIs with loose typing (Tautulli/Python,
// Jellystat/Node.js, Tracearr) may return numeric fields as any of these
// representations; this type handles all of them by truncating to int64.
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(data []byte) error {
	// Use json.Number to handle integers, floats, and scientific notation.
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		// Try integer first (the common case).
		if i, err := n.Int64(); err == nil {
			*f = flexInt64(i)
			return nil
		}
		// Fall back to float (e.g., 1703520000.0) and truncate.
		if fl, err := n.Float64(); err == nil {
			*f = flexInt64(int64(fl))
			return nil
		}
	}

	// Fall back to string-encoded number (e.g., "1703520000").
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		i, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			*f = flexInt64(i)
			return nil
		}
		// Try parsing as float string (e.g., "1703520000.0").
		fl, err := strconv.ParseFloat(s, 64)
		if err == nil {
			*f = flexInt64(int64(fl))
			return nil
		}
	}

	return fmt.Errorf("flexInt64: cannot unmarshal %s", strconv.Quote(string(data)))
}

// TautulliClient provides access to the Tautulli API for enriched watch history.
// Tautulli supplements Plex's binary watched/unwatched signal with detailed
// play counts, last played timestamps, watch durations, and per-user history.
type TautulliClient struct {
	URL    string
	APIKey string `json:"-"`
}

// NewTautulliClient creates a new Tautulli API client.
func NewTautulliClient(url, apiKey string) *TautulliClient {
	return &TautulliClient{
		URL:    strings.TrimRight(url, "/"),
		APIKey: apiKey,
	}
}

// TautulliWatchData contains enriched watch history from Tautulli.
type TautulliWatchData struct {
	PlayCount     int        `json:"playCount"`
	LastPlayed    *time.Time `json:"lastPlayed,omitempty"`
	TotalDuration int64      `json:"totalDuration"` // total seconds watched
	Users         []string   `json:"users"`         // which users watched
}

// doRequest executes a Tautulli API call using the ?apikey=XXX&cmd=CMD pattern.
func (t *TautulliClient) doRequest(cmd string, extraParams string) ([]byte, error) {
	fullURL := fmt.Sprintf("%s/api/v2?apikey=%s&cmd=%s", t.URL, t.APIKey, cmd)
	if extraParams != "" {
		fullURL += "&" + extraParams
	}
	return DoAPIRequest(fullURL, "", "")
}

// tautulliResponse wraps the standard Tautulli API response envelope.
type tautulliResponse struct {
	Response struct {
		Result  string          `json:"result"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"response"`
}

// tautulliHistoryData maps the get_history response data.
type tautulliHistoryData struct {
	RecordsFiltered int                    `json:"recordsFiltered"`
	RecordsTotal    int                    `json:"recordsTotal"`
	Data            []tautulliHistoryEntry `json:"data"`
}

// tautulliHistoryEntry represents one play record from Tautulli history.
type tautulliHistoryEntry struct {
	Date                 flexInt64  `json:"date"`           // Unix epoch of play start
	Duration             flexInt64  `json:"duration"`       // Duration of item (seconds)
	PlayDuration         flexInt64  `json:"play_duration"`  // Actual time played (seconds)
	PausedCounter        flexInt64  `json:"paused_counter"` // Time spent paused (seconds)
	WatchedStatus        float64    `json:"watched_status"` // 0=unwatched, 0.5=partial, 1=watched
	User                 string     `json:"user"`           // Username
	RatingKey            flexString `json:"rating_key"`     // Plex rating key
	ParentRatingKey      flexString `json:"parent_rating_key"`
	GrandparentRatingKey flexString `json:"grandparent_rating_key"`
	Title                string     `json:"title"`
	MediaType            string     `json:"media_type"` // movie, episode, track
}

// TestConnection verifies the Tautulli URL and API key are valid
// by calling the get_tautulli_info command.
func (t *TautulliClient) TestConnection() error {
	body, err := t.doRequest("get_tautulli_info", "")
	if err != nil {
		return err
	}

	var resp tautulliResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("failed to parse Tautulli response: %w", err)
	}

	if resp.Response.Result != "success" {
		return fmt.Errorf("tautulli error: %s", resp.Response.Message)
	}

	return nil
}

// GetWatchHistory fetches aggregated watch data for a specific Plex rating key.
// It queries Tautulli's get_history endpoint filtered by the rating key, then
// aggregates play count, last played time, total duration, and unique users.
func (t *TautulliClient) GetWatchHistory(ratingKey string) (*TautulliWatchData, error) {
	params := fmt.Sprintf("rating_key=%s&length=100", ratingKey)
	body, err := t.doRequest("get_history", params)
	if err != nil {
		return nil, err
	}

	var resp tautulliResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse Tautulli response: %w", err)
	}

	if resp.Response.Result != "success" {
		return nil, fmt.Errorf("tautulli error: %s", resp.Response.Message)
	}

	var history tautulliHistoryData
	if err := json.Unmarshal(resp.Response.Data, &history); err != nil {
		return nil, fmt.Errorf("failed to parse history data: %w", err)
	}

	data := &TautulliWatchData{}
	data.PlayCount = len(history.Data)

	userSet := make(map[string]bool)
	var latestPlay int64

	for _, entry := range history.Data {
		// Track total actual watch duration
		data.TotalDuration += int64(entry.PlayDuration)

		// Track unique users
		if entry.User != "" {
			userSet[entry.User] = true
		}

		// Track most recent play
		if int64(entry.Date) > latestPlay {
			latestPlay = int64(entry.Date)
		}
	}

	if latestPlay > 0 {
		t := time.Unix(latestPlay, 0)
		data.LastPlayed = &t
	}

	for user := range userSet {
		data.Users = append(data.Users, user)
	}

	return data, nil
}

// GetShowWatchHistory fetches watch data for a show by querying with grandparent_rating_key.
// This aggregates across all episodes of the show for a holistic view.
func (t *TautulliClient) GetShowWatchHistory(ratingKey string) (*TautulliWatchData, error) {
	params := fmt.Sprintf("grandparent_rating_key=%s&length=500", ratingKey)
	body, err := t.doRequest("get_history", params)
	if err != nil {
		return nil, err
	}

	var resp tautulliResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse Tautulli response: %w", err)
	}

	if resp.Response.Result != "success" {
		return nil, fmt.Errorf("tautulli error: %s", resp.Response.Message)
	}

	var history tautulliHistoryData
	if err := json.Unmarshal(resp.Response.Data, &history); err != nil {
		return nil, fmt.Errorf("failed to parse history data: %w", err)
	}

	data := &TautulliWatchData{}
	data.PlayCount = len(history.Data)

	userSet := make(map[string]bool)
	var latestPlay int64

	for _, entry := range history.Data {
		data.TotalDuration += int64(entry.PlayDuration)

		if entry.User != "" {
			userSet[entry.User] = true
		}

		if int64(entry.Date) > latestPlay {
			latestPlay = int64(entry.Date)
		}
	}

	if latestPlay > 0 {
		t := time.Unix(latestPlay, 0)
		data.LastPlayed = &t
	}

	for user := range userSet {
		data.Users = append(data.Users, user)
	}

	return data, nil
}

// tautulliHistoryPageSize is the number of history entries fetched per API call
// in GetAllHistory(). 1000 is a good balance — small enough to keep individual
// response sizes manageable (~500KB), large enough to minimize round trips
// (a typical server has 1,000–20,000 total history entries = 1–20 API calls).
const tautulliHistoryPageSize = 1000

// getAllHistory fetches the complete watch history from Tautulli in paginated
// chunks. This replaces the previous per-item GetWatchHistory() pattern which
// made N individual API calls (one per media item with a TMDb→RatingKey mapping).
// With 500+ items that was 500+ sequential HTTP requests; this bulk approach
// reduces it to ceil(totalRecords / 1000) calls — typically 1–20.
//
// Returns all history entries for in-memory aggregation by the TautulliEnricher.
func (t *TautulliClient) getAllHistory() ([]tautulliHistoryEntry, error) {
	var allEntries []tautulliHistoryEntry
	start := 0

	for {
		params := fmt.Sprintf("length=%d&start=%d", tautulliHistoryPageSize, start)
		body, err := t.doRequest("get_history", params)
		if err != nil {
			return allEntries, fmt.Errorf("tautulli bulk history page %d: %w", start, err)
		}

		var resp tautulliResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return allEntries, fmt.Errorf("tautulli bulk history parse: %w", err)
		}

		if resp.Response.Result != "success" {
			return allEntries, fmt.Errorf("tautulli error: %s", resp.Response.Message)
		}

		var history tautulliHistoryData
		if err := json.Unmarshal(resp.Response.Data, &history); err != nil {
			return allEntries, fmt.Errorf("tautulli bulk history data: %w", err)
		}

		allEntries = append(allEntries, history.Data...)

		// Check if we've fetched all records
		if len(allEntries) >= history.RecordsFiltered || len(history.Data) == 0 {
			break
		}
		start += len(history.Data)
	}

	return allEntries, nil
}

// Verify TautulliClient satisfies capability interfaces at compile time.
var _ Connectable = (*TautulliClient)(nil)
