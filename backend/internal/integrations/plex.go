package integrations

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PlexClient implements Connectable, WatchDataProvider, and WatchlistProvider for Plex Media Server.
// PlexClient intentionally does NOT implement MediaSource — only *arr integrations (which also
// implement MediaDeleter and DiskReporter) should provide media items to the evaluation pool.
//
// Per-cycle caching: getMediaItems() fetches the full Plex library once per client
// instance and caches the result. Since BuildIntegrationRegistry() creates new client
// instances each poll cycle, the cache is naturally cycle-scoped and requires no
// explicit reset. This eliminates ~16 redundant Plex API calls per cycle where
// multiple enrichers (watch data, collections, labels, TMDb mapping) each independently
// fetched the same library data.
type PlexClient struct {
	URL   string
	Token string `json:"-"` // X-Plex-Token

	// Per-cycle library cache. Populated on first call to getMediaItems(),
	// reused by all subsequent callers within the same poll cycle.
	cachedItems    []MediaItem
	cachedItemsErr error
	cacheOnce      sync.Once
}

// NewPlexClient creates a new Plex media server API client.
func NewPlexClient(url, token string) *PlexClient {
	return &PlexClient{
		URL:   strings.TrimRight(url, "/"),
		Token: token,
	}
}

func (p *PlexClient) doRequest(endpoint string) ([]byte, error) {
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	fullURL := p.URL + endpoint + sep + "X-Plex-Token=" + p.Token
	return DoAPIRequest(fullURL, "Accept", "application/json")
}

// TestConnection verifies the Plex server is reachable and the token is valid.
func (p *PlexClient) TestConnection() error {
	_, err := p.doRequest("/identity")
	return err
}

// plexLibraryResponse maps /library/sections response
type plexLibraryResponse struct {
	MediaContainer struct {
		Directory []struct {
			Key   string `json:"key"`
			Title string `json:"title"`
			Type  string `json:"type"` // movie, show, artist
		} `json:"Directory"`
	} `json:"MediaContainer"`
}

// plexMediaResponse maps /library/sections/{key}/all response
type plexMediaResponse struct {
	MediaContainer struct {
		Metadata []plexMetadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

type plexMetadata struct {
	RatingKey        string     `json:"ratingKey"`
	Title            string     `json:"title"`
	ParentTitle      string     `json:"parentTitle,omitempty"`
	GrandparentTitle string     `json:"grandparentTitle,omitempty"`
	Year             int        `json:"year"`
	Type             string     `json:"type"` // movie, show, season, episode
	AudienceRating   float64    `json:"audienceRating"`
	Rating           float64    `json:"rating"`
	ViewCount        int        `json:"viewCount"`
	LastViewedAt     int64      `json:"lastViewedAt"`
	AddedAt          int64      `json:"addedAt"`
	Duration         int64      `json:"duration"`
	GUID             string     `json:"guid"`           // Primary GUID (e.g. "plex://movie/...")
	GUIDs            []plexGUID `json:"Guid,omitempty"` // Additional GUIDs including TMDb references
	Genre            []struct {
		Tag string `json:"tag"`
	} `json:"Genre"`
	Collection []struct {
		Tag string `json:"tag"`
	} `json:"Collection"`
	Label []struct {
		Tag string `json:"tag"`
	} `json:"Label"`
	Media []struct {
		Part []struct {
			File string `json:"file"`
			Size int64  `json:"size"`
		} `json:"Part"`
	} `json:"Media"`
	Index     int `json:"index,omitempty"`     // season/episode number
	LeafCount int `json:"leafCount,omitempty"` // episode count (for shows/seasons)
}

// plexGUID represents a GUID entry from the Plex API.
type plexGUID struct {
	ID string `json:"id"` // e.g. "tmdb://12345", "imdb://tt1234567", "tvdb://54321"
}

// plexTMDbIDRegex matches TMDb IDs in Plex GUID strings like "tmdb://12345".
var plexTMDbIDRegex = regexp.MustCompile(`^tmdb://(\d+)$`)

// plexExtractTMDbID extracts the TMDb ID from a Plex item's GUIDs array.
// Plex stores GUIDs as "tmdb://12345", "imdb://tt1234567", etc.
// Returns 0 if no TMDb GUID is found.
func plexExtractTMDbID(guids []plexGUID) int {
	for _, g := range guids {
		matches := plexTMDbIDRegex.FindStringSubmatch(g.ID)
		if len(matches) == 2 {
			id, err := strconv.Atoi(matches[1])
			if err == nil {
				return id
			}
		}
	}
	return 0
}

// getMediaItems returns all movies, shows, and seasons from all Plex libraries.
// Results are cached for the lifetime of this PlexClient instance (one poll cycle).
// This method is unexported to prevent PlexClient from satisfying the MediaSource interface.
// Only *arr integrations should implement MediaSource. Internal callers (GetBulkWatchData,
// GetCollectionMemberships, GetLabelMemberships, GetTMDbToRatingKeyMap, GetCollectionNames,
// GetLabelNames) all share the same cached result.
func (p *PlexClient) getMediaItems() ([]MediaItem, error) {
	p.cacheOnce.Do(func() {
		p.cachedItems, p.cachedItemsErr = p.fetchMediaItems()
	})
	return p.cachedItems, p.cachedItemsErr
}

// fetchMediaItems performs the actual HTTP calls to fetch all movies, shows, and
// seasons from all Plex libraries. Called once per cycle via getMediaItems().
func (p *PlexClient) fetchMediaItems() ([]MediaItem, error) {
	// 1. Get all library sections
	body, err := p.doRequest("/library/sections")
	if err != nil {
		return nil, err
	}

	var libs plexLibraryResponse
	if err := json.Unmarshal(body, &libs); err != nil {
		return nil, fmt.Errorf("failed to parse library sections: %w", err)
	}

	var items []MediaItem

	for _, lib := range libs.MediaContainer.Directory {
		// Only process movie and show libraries
		if lib.Type != string(MediaTypeMovie) && lib.Type != string(MediaTypeShow) {
			continue
		}

		// 2. Get all items in this library (includeGuids=1 is required for TMDb ID extraction)
		itemBody, err := p.doRequest(fmt.Sprintf("/library/sections/%s/all?includeGuids=1", lib.Key))
		if err != nil {
			continue // Skip failed libraries
		}

		var media plexMediaResponse
		if err := json.Unmarshal(itemBody, &media); err != nil {
			continue
		}

		for _, m := range media.MediaContainer.Metadata {
			item := plexMetadataToMediaItem(m)
			if item != nil {
				items = append(items, *item)
			}
		}
	}

	return items, nil
}

func plexMetadataToMediaItem(m plexMetadata) *MediaItem {
	// Calculate total file size from all media parts
	var totalSize int64
	var filePath string
	for _, media := range m.Media {
		for _, part := range media.Part {
			totalSize += part.Size
			if filePath == "" {
				filePath = part.File
			}
		}
	}

	// Build genre string
	genres := make([]string, 0, len(m.Genre))
	for _, g := range m.Genre {
		genres = append(genres, g.Tag)
	}

	// Build collections list
	collections := make([]string, 0, len(m.Collection))
	for _, c := range m.Collection {
		collections = append(collections, c.Tag)
	}

	// Build labels list
	labels := make([]string, 0, len(m.Label))
	for _, l := range m.Label {
		labels = append(labels, l.Tag)
	}

	// Pick best rating
	rating := m.AudienceRating
	if rating == 0 {
		rating = m.Rating
	}

	// Convert timestamps
	var lastPlayed *time.Time
	if m.LastViewedAt > 0 {
		t := time.Unix(m.LastViewedAt, 0)
		lastPlayed = &t
	}

	var addedAt *time.Time
	if m.AddedAt > 0 {
		t := time.Unix(m.AddedAt, 0)
		addedAt = &t
	}

	var mediaType MediaType
	switch MediaType(m.Type) { //nolint:exhaustive // Plex only returns movie, show, season, and episode types
	case MediaTypeMovie:
		mediaType = MediaTypeMovie
	case MediaTypeShow:
		mediaType = MediaTypeShow
	case MediaTypeSeason:
		mediaType = MediaTypeSeason
	case MediaTypeEpisode:
		mediaType = MediaTypeEpisode
	default:
		return nil
	}

	item := &MediaItem{
		ExternalID:  m.RatingKey,
		Type:        mediaType,
		Title:       m.Title,
		Year:        m.Year,
		TMDbID:      plexExtractTMDbID(m.GUIDs),
		SizeBytes:   totalSize,
		Path:        filePath,
		Rating:      rating,
		Genre:       strings.Join(genres, ", "),
		PlayCount:   m.ViewCount,
		LastPlayed:  lastPlayed,
		AddedAt:     addedAt,
		Collections: collections,
		Labels:      labels,
	}

	// Show/season specifics
	if m.Type == "season" {
		item.SeasonNumber = m.Index
		item.EpisodeCount = m.LeafCount
		item.ShowTitle = m.ParentTitle
	}

	return item
}

// GetLibrarySections returns the library sections for display purposes
func (p *PlexClient) GetLibrarySections() ([]PlexLibrarySection, error) {
	body, err := p.doRequest("/library/sections")
	if err != nil {
		return nil, err
	}

	var libs plexLibraryResponse
	if err := json.Unmarshal(body, &libs); err != nil {
		return nil, fmt.Errorf("failed to parse library sections: %w", err)
	}

	sections := make([]PlexLibrarySection, len(libs.MediaContainer.Directory))
	for i, d := range libs.MediaContainer.Directory {
		sections[i] = PlexLibrarySection{
			Key:   d.Key,
			Title: d.Title,
			Type:  d.Type,
		}
	}
	return sections, nil
}

// PlexLibrarySection represents a Plex library section
type PlexLibrarySection struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// GetBulkWatchData returns a map of TMDb ID to watch data from the cached Plex
// library. Uses getMediaItems() to avoid redundant library fetches — the same
// cached data is shared with GetCollectionMemberships, GetLabelMemberships,
// GetTMDbToRatingKeyMap, etc.
func (p *PlexClient) GetBulkWatchData() (map[int]*WatchData, error) {
	items, err := p.getMediaItems()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Plex items for watch data: %w", err)
	}

	result := make(map[int]*WatchData)
	for _, item := range items {
		if item.TMDbID == 0 {
			continue
		}

		data := &WatchData{
			PlayCount:  item.PlayCount,
			LastPlayed: item.LastPlayed,
		}
		// Keep the entry with the highest play count if duplicates
		if existing, ok := result[item.TMDbID]; ok {
			if data.PlayCount > existing.PlayCount {
				result[item.TMDbID] = data
			}
		} else {
			result[item.TMDbID] = data
		}
	}
	return result, nil
}

// GetTMDbToRatingKeyMap builds a mapping from TMDb ID to Plex ratingKey by
// scanning all movie and show libraries. This is used by the Tautulli enricher
// to translate TMDb IDs from *arr items into Plex rating keys for per-item
// watch history queries. Built and consumed within a single poll cycle — not cached.
func (p *PlexClient) GetTMDbToRatingKeyMap() (map[int]string, error) {
	items, err := p.getMediaItems()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Plex items for TMDb mapping: %w", err)
	}

	result := make(map[int]string)
	for _, item := range items {
		if item.TMDbID > 0 && item.ExternalID != "" {
			result[item.TMDbID] = item.ExternalID
		}
	}
	return result, nil
}

// GetOnDeckItems returns a set of TMDb IDs for items on the Plex "On Deck" list.
// On-deck items are those a user has started watching or that are next in a
// series they are watching — a strong signal of active interest.
// The returned map is keyed by TMDb ID for matching against *arr items.
func (p *PlexClient) GetOnDeckItems() (map[int]bool, error) {
	body, err := p.doRequest("/library/onDeck")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Plex on-deck items: %w", err)
	}

	var resp plexMediaResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse Plex on-deck response: %w", err)
	}

	result := make(map[int]bool)
	for _, m := range resp.MediaContainer.Metadata {
		tmdbID := plexExtractTMDbID(m.GUIDs)
		if tmdbID > 0 {
			result[tmdbID] = true
		}
	}
	return result, nil
}

// GetCollectionNames returns a sorted, deduplicated list of collection names
// from all Plex libraries. This is used by FetchCollectionValues() to provide
// autocomplete options for collection-based rules without exposing GetMediaItems
// (which would make PlexClient satisfy MediaSource).
func (p *PlexClient) GetCollectionNames() ([]string, error) {
	items, err := p.getMediaItems()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Plex items for collections: %w", err)
	}

	seen := make(map[string]bool)
	for _, item := range items {
		for _, col := range item.Collections {
			name := strings.TrimSpace(col)
			if name != "" {
				seen[name] = true
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// GetWatchlistItems implements WatchlistProvider by returning Plex on-deck items
// keyed by TMDb ID.
func (p *PlexClient) GetWatchlistItems() (map[int]bool, error) {
	return p.GetOnDeckItems()
}

// GetCollectionMemberships implements CollectionDataProvider by scanning all
// Plex libraries and building a TMDb ID → collection names map from metadata.
// This bridges Plex collection data onto *arr items via the CollectionEnricher.
func (p *PlexClient) GetCollectionMemberships() (map[int][]string, error) {
	items, err := p.getMediaItems()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Plex items for collection memberships: %w", err)
	}

	result := make(map[int][]string)
	for _, item := range items {
		if item.TMDbID == 0 || len(item.Collections) == 0 {
			continue
		}
		result[item.TMDbID] = item.Collections
	}
	return result, nil
}

// GetLabelMemberships implements LabelDataProvider by scanning all Plex
// libraries and building a TMDb ID → label names map from metadata.
// This bridges Plex label data onto *arr items via the LabelEnricher.
func (p *PlexClient) GetLabelMemberships() (map[int][]string, error) {
	items, err := p.getMediaItems()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Plex items for label memberships: %w", err)
	}

	result := make(map[int][]string)
	for _, item := range items {
		if item.TMDbID == 0 || len(item.Labels) == 0 {
			continue
		}
		result[item.TMDbID] = item.Labels
	}
	return result, nil
}

// GetLabelNames returns a sorted, deduplicated list of label names from all
// Plex libraries. Used by FetchLabelValues() for rule value autocomplete.
func (p *PlexClient) GetLabelNames() ([]string, error) {
	items, err := p.getMediaItems()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Plex items for labels: %w", err)
	}

	seen := make(map[string]bool)
	for _, item := range items {
		for _, lbl := range item.Labels {
			name := strings.TrimSpace(lbl)
			if name != "" {
				seen[name] = true
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// doRequestWithMethod creates an HTTP request with a custom method (PUT, POST)
// and appends the Plex token as a query parameter. Used for label management.
func (p *PlexClient) doRequestWithMethod(method, endpoint string) error {
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	fullURL := p.URL + endpoint + sep + "X-Plex-Token=" + p.Token
	return DoAPIRequestWithBody(method, fullURL, nil, "Accept", "application/json")
}

// AddLabel applies a label to a Plex item identified by ratingKey.
// Uses the Plex metadata endpoint: PUT /library/metadata/{ratingKey}?label[0].tag.tag={label}&label.locked=1
func (p *PlexClient) AddLabel(itemID string, label string) error {
	endpoint := fmt.Sprintf("/library/metadata/%s?label[0].tag.tag=%s&label.locked=1",
		url.PathEscape(itemID), url.QueryEscape(label))
	return p.doRequestWithMethod("PUT", endpoint)
}

// RemoveLabel removes a label from a Plex item identified by ratingKey.
// Uses the Plex metadata endpoint: PUT /library/metadata/{ratingKey}?label[].tag.tag-={label}&label.locked=1
func (p *PlexClient) RemoveLabel(itemID string, label string) error {
	endpoint := fmt.Sprintf("/library/metadata/%s?label[].tag.tag-=%s&label.locked=1",
		url.PathEscape(itemID), url.QueryEscape(label))
	return p.doRequestWithMethod("PUT", endpoint)
}

// GetPosterImage downloads the current primary poster for a Plex item.
// Uses /library/metadata/{ratingKey}/thumb to fetch the poster image.
func (p *PlexClient) GetPosterImage(itemID string) ([]byte, string, error) {
	endpoint := fmt.Sprintf("/library/metadata/%s/thumb", url.PathEscape(itemID))
	data, err := p.doRequest(endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("fetch poster: %w", err)
	}
	return data, "image/jpeg", nil
}

// UploadPosterImage uploads a new primary poster to a Plex item.
// Plex requires multipart/form-data with a "file" field to both upload AND select
// the poster as active. A raw body POST only adds the poster to the selection list
// without making it the active poster.
func (p *PlexClient) UploadPosterImage(itemID string, imageData []byte, _ string) error {
	sep := "?"
	endpoint := fmt.Sprintf("/library/metadata/%s/posters", url.PathEscape(itemID))
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	fullURL := p.URL + endpoint + sep + "X-Plex-Token=" + p.Token
	return DoMultipartUpload(fullURL, imageData, "file", "poster.jpg", nil)
}

// RestorePosterImage removes any custom poster from a Plex item, reverting to
// the default agent-sourced poster. Achieved by unlocking the poster field.
func (p *PlexClient) RestorePosterImage(itemID string) error {
	endpoint := fmt.Sprintf("/library/metadata/%s?thumb=&poster.locked=0",
		url.PathEscape(itemID))
	return p.doRequestWithMethod("PUT", endpoint)
}

// SearchByTMDbID searches Plex for an item matching the given TMDb ID.
// Uses title to narrow the search space via /hubs/search, then verifies
// the TMDb ID in the Guid array of each result. Returns the ratingKey
// of the matched item.
func (p *PlexClient) SearchByTMDbID(title string, tmdbID int) (string, error) {
	if title == "" || tmdbID <= 0 {
		return "", fmt.Errorf("title and tmdbID are required for Plex search")
	}

	endpoint := fmt.Sprintf("/hubs/search?query=%s&includeGuids=1&limit=25", url.QueryEscape(title))
	body, err := p.doRequest(endpoint)
	if err != nil {
		return "", fmt.Errorf("plex search: %w", err)
	}

	// Plex search returns hubs containing metadata arrays
	var resp struct {
		MediaContainer struct {
			Hub []struct {
				Metadata []plexMetadata `json:"Metadata"`
			} `json:"Hub"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("plex search unmarshal: %w", err)
	}

	for _, hub := range resp.MediaContainer.Hub {
		for _, m := range hub.Metadata {
			if plexExtractTMDbID(m.GUIDs) == tmdbID {
				return m.RatingKey, nil
			}
		}
	}

	return "", fmt.Errorf("plex search: no item found with TMDb ID %d", tmdbID)
}

// Verify PlexClient satisfies capability interfaces at compile time.
// Note: PlexClient intentionally does NOT implement MediaSource — only *arr integrations should.
var _ Connectable = (*PlexClient)(nil)
var _ WatchDataProvider = (*PlexClient)(nil)
var _ WatchlistProvider = (*PlexClient)(nil)
var _ CollectionDataProvider = (*PlexClient)(nil)
var _ LabelDataProvider = (*PlexClient)(nil)
var _ LabelManager = (*PlexClient)(nil)
var _ PosterManager = (*PlexClient)(nil)
var _ NativeIDSearcher = (*PlexClient)(nil)
