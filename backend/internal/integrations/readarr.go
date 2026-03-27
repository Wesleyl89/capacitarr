package integrations

import (
	"encoding/json"
	"fmt"
	"time"
)

// ReadarrClient implements Connectable, MediaSource, DiskReporter, MediaDeleter, and RuleValueFetcher for Readarr v1 API (books/audiobooks).
// Follows the same API pattern as Sonarr/Radarr/Lidarr.
// Shared *arr methods are provided by the embedded arrBaseClient.
type ReadarrClient struct {
	arrBaseClient
}

// NewReadarrClient creates a new Readarr book management API client.
func NewReadarrClient(url, apiKey string) *ReadarrClient {
	return &ReadarrClient{
		arrBaseClient: newArrBaseClient(url, apiKey, "/api/v1"),
	}
}

// readarrBook maps a Readarr book API response (relevant fields)
type readarrBook struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	AuthorID int    `json:"authorId"`
	Author   struct {
		AuthorName string `json:"authorName"`
	} `json:"author"`
	SizeOnDisk  int64      `json:"sizeOnDisk"`
	ReleaseDate string     `json:"releaseDate"`
	Added       string     `json:"added"`
	Monitored   bool       `json:"monitored"`
	Path        string     `json:"path"`
	Images      []arrImage `json:"images"`
	Ratings     struct {
		Value float64 `json:"value"`
	} `json:"ratings"`
	Genres           []string `json:"genres"`
	Tags             []int    `json:"tags"`
	QualityProfileID int      `json:"qualityProfileId"`
}

// GetMediaItems fetches all books from Readarr with quality, tag, and rating metadata.
func (r *ReadarrClient) GetMediaItems() ([]MediaItem, error) {
	// Fetch quality profiles for name lookup
	profileMap, err := arrFetchQualityProfileMap(r.doRequest, "/api/v1")
	if err != nil {
		return nil, err
	}

	// Fetch tags for name lookup
	tagMap, err := arrFetchTagMap(r.doRequest, "/api/v1")
	if err != nil {
		return nil, err
	}

	// Fetch all books
	body, err := r.doRequest("/api/v1/book")
	if err != nil {
		return nil, err
	}
	var books []readarrBook
	if err := json.Unmarshal(body, &books); err != nil {
		return nil, fmt.Errorf("failed to parse Readarr books: %w", err)
	}

	items := make([]MediaItem, 0, len(books))
	for _, b := range books {
		if b.SizeOnDisk == 0 {
			continue
		}

		var addedAt *time.Time
		if b.Added != "" {
			t, err := time.Parse(time.RFC3339, b.Added)
			if err == nil {
				addedAt = &t
			}
		}

		// Extract publication year from releaseDate
		var year int
		if b.ReleaseDate != "" {
			if t, err := time.Parse(time.RFC3339, b.ReleaseDate); err == nil {
				year = t.Year()
			}
		}

		tagNames := arrResolveTagNames(b.Tags, tagMap)

		// Pick genre string from first genre if available
		genre := ""
		if len(b.Genres) > 0 {
			genre = b.Genres[0]
		}

		// Readarr ratings.value is GoodReads scale (0–5).
		// Normalize to 0–10 so the scoring engine handles it consistently.
		rating := b.Ratings.Value * 2.0

		items = append(items, MediaItem{
			ExternalID:     fmt.Sprintf("%d", b.ID),
			Title:          b.Title,
			Type:           MediaTypeBook,
			Year:           year,
			SizeBytes:      b.SizeOnDisk,
			AddedAt:        addedAt,
			Monitored:      b.Monitored,
			Path:           b.Path,
			PosterURL:      arrExtractPosterURL(b.Images, r.URL),
			Rating:         rating,
			Genre:          genre,
			Tags:           tagNames,
			QualityProfile: profileMap[b.QualityProfileID],
		})
	}
	return items, nil
}

// GetQualityProfiles, GetTags, GetLanguages are provided by arrBaseClient.

// DeleteMediaItem removes a book from Readarr and optionally deletes files
func (r *ReadarrClient) DeleteMediaItem(item MediaItem) error {
	endpoint := fmt.Sprintf("/api/v1/book/%s?deleteFiles=true&addImportExclusion=false", item.ExternalID)
	return arrSimpleDelete(r.URL, r.APIKey, endpoint)
}
