package integrations

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LidarrClient implements Connectable, MediaSource, DiskReporter, MediaDeleter, and RuleValueFetcher for Lidarr v1 API (Servarr framework).
// Lidarr manages music libraries and follows the same API patterns as Sonarr/Radarr.
// Shared *arr methods are provided by the embedded arrBaseClient.
type LidarrClient struct {
	arrBaseClient
}

// NewLidarrClient creates a new Lidarr music management API client.
func NewLidarrClient(url, apiKey string) *LidarrClient {
	return &LidarrClient{
		arrBaseClient: newArrBaseClient(url, apiKey, "/api/v1"),
	}
}

// GetLanguages returns nil because Lidarr does not support language lookup.
// This overrides the arrBaseClient default which delegates to arrFetchLanguages.
func (l *LidarrClient) GetLanguages() ([]NameValue, error) {
	return nil, nil
}

// lidarrArtist maps the Lidarr artist API response (relevant fields)
type lidarrArtist struct {
	ID         int    `json:"id"`
	ArtistName string `json:"artistName"`
	Path       string `json:"path"`
	Monitored  bool   `json:"monitored"`
	Ratings    struct {
		Value float64 `json:"value"`
	} `json:"ratings"`
	Genres           []string   `json:"genres"`
	Tags             []int      `json:"tags"`
	QualityProfileID int        `json:"qualityProfileId"`
	Added            string     `json:"added"`
	Images           []arrImage `json:"images"`
	Statistics       struct {
		SizeOnDisk int64 `json:"sizeOnDisk"`
		AlbumCount int   `json:"albumCount"`
		TrackCount int   `json:"trackCount"`
	} `json:"statistics"`
}

// GetMediaItems fetches all artists and their albums from Lidarr.
func (l *LidarrClient) GetMediaItems() ([]MediaItem, error) {
	// Fetch quality profiles for name lookup
	profileMap, err := arrFetchQualityProfileMap(l.doRequest, "/api/v1")
	if err != nil {
		return nil, err
	}

	// Fetch tags for name lookup
	tagMap, err := arrFetchTagMap(l.doRequest, "/api/v1")
	if err != nil {
		return nil, err
	}

	// Fetch all artists
	body, err := l.doRequest("/api/v1/artist")
	if err != nil {
		return nil, err
	}

	var artists []lidarrArtist
	if err := json.Unmarshal(body, &artists); err != nil {
		return nil, fmt.Errorf("failed to parse artists: %w", err)
	}

	items := make([]MediaItem, 0, len(artists))
	for _, a := range artists {
		if a.Statistics.SizeOnDisk == 0 {
			continue // Skip artists without files on disk
		}

		// Lidarr ratings.value is already on a 0–10 scale (MusicBrainz).
		// Pass through directly — the scoring engine normalizes 0–10 at score time.
		rating := a.Ratings.Value

		tagNames := arrResolveTagNames(a.Tags, tagMap)

		var addedAt *time.Time
		if a.Added != "" {
			if t, err := time.Parse(time.RFC3339, a.Added); err == nil {
				addedAt = &t
			}
		}

		items = append(items, MediaItem{
			ExternalID:     strconv.Itoa(a.ID),
			Type:           MediaTypeArtist,
			Title:          a.ArtistName,
			SizeBytes:      a.Statistics.SizeOnDisk,
			Path:           a.Path,
			PosterURL:      arrExtractPosterURL(a.Images, l.URL),
			QualityProfile: profileMap[a.QualityProfileID],
			Rating:         rating,
			Genre:          strings.Join(a.Genres, ", "),
			Monitored:      a.Monitored,
			Tags:           tagNames,
			AddedAt:        addedAt,
		})
	}

	return items, nil
}

// GetQualityProfiles, GetTags are provided by arrBaseClient.
// GetLanguages is overridden above (Lidarr has no language endpoint).

// DeleteMediaItem removes an artist and its files from disk via the Lidarr API.
// When opts.AddImportExclusion is true, the artist is added to Lidarr's import
// list exclusion to prevent automatic re-addition by import lists.
func (l *LidarrClient) DeleteMediaItem(item MediaItem, opts DeleteOptions) error {
	endpoint := fmt.Sprintf("/api/v1/artist/%s?deleteFiles=true&addImportListExclusion=%t", item.ExternalID, opts.AddImportExclusion)
	return arrSimpleDelete(l.URL, l.APIKey, endpoint)
}
