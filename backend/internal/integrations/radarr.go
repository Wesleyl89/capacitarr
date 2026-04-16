package integrations

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RadarrClient implements Connectable, MediaSource, DiskReporter, MediaDeleter, and RuleValueFetcher for Radarr v3 API.
// Shared *arr methods are provided by the embedded arrBaseClient.
//
// Per-cycle caching: The raw movie list from /api/v3/movie is cached on first
// fetch and reused by ResolveCollectionMembers(). Since BuildIntegrationRegistry()
// creates new client instances each cycle, the cache is naturally cycle-scoped.
// This eliminates 3 redundant API calls per collection resolution (movie list +
// quality profiles + tags).
type RadarrClient struct {
	arrBaseClient

	// Per-cycle movie list cache. Populated on first call to getCachedMovies().
	cachedMovies    []radarrMovie
	cachedMoviesErr error
	moviesOnce      sync.Once
}

// NewRadarrClient creates a new Radarr movie management API client.
func NewRadarrClient(url, apiKey string) *RadarrClient {
	return &RadarrClient{
		arrBaseClient: newArrBaseClient(url, apiKey, "/api/v3"),
	}
}

// radarrMovie maps the Radarr movie API response (relevant fields)
type radarrMovie struct {
	ID               int         `json:"id"`
	Title            string      `json:"title"`
	Year             int         `json:"year"`
	TmdbID           int         `json:"tmdbId"`
	Path             string      `json:"path"`
	Monitored        bool        `json:"monitored"`
	HasFile          bool        `json:"hasFile"`
	SizeOnDisk       int64       `json:"sizeOnDisk"`
	OriginalLanguage arrLanguage `json:"originalLanguage"`
	Ratings          struct {
		IMDB struct {
			Value float64 `json:"value"`
		} `json:"imdb"`
		TMDB struct {
			Value float64 `json:"value"`
		} `json:"tmdb"`
	} `json:"ratings"`
	Genres           []string          `json:"genres"`
	Tags             []int             `json:"tags"`
	QualityProfileID int               `json:"qualityProfileId"`
	Added            string            `json:"added"`
	MovieFile        *radarrMovieFile  `json:"movieFile,omitempty"` // Inline file metadata (dateAdded = actual import time)
	Images           []arrImage        `json:"images"`
	Collection       *radarrCollection `json:"collection,omitempty"` // TMDb collection (e.g., "Sonic the Hedgehog Collection")
}

// radarrMovieFile maps the inline movieFile object from the Radarr movie API response.
// The dateAdded field represents when the file was actually imported/downloaded,
// which is more accurate than movie.added for "time in library" calculations.
type radarrMovieFile struct {
	DateAdded string `json:"dateAdded"`
}

// radarrCollection maps the Radarr API collection object.
type radarrCollection struct {
	Name   string `json:"name"`
	TmdbID int    `json:"tmdbId"`
}

// radarrResolveAddedAt determines the best available "added" timestamp for a movie.
// It prefers movieFile.dateAdded (actual file import time) over movie.added (entry creation time).
// Falls back to movie.added if movieFile is nil or its dateAdded is empty/unparseable.
func radarrResolveAddedAt(movieFile *radarrMovieFile, added string) *time.Time {
	// Prefer file-level dateAdded — this is when the file was actually imported
	if movieFile != nil && movieFile.DateAdded != "" {
		if t, err := time.Parse(time.RFC3339, movieFile.DateAdded); err == nil {
			return &t
		}
	}
	// Fall back to entry-level added date
	if added != "" {
		if t, err := time.Parse(time.RFC3339, added); err == nil {
			return &t
		}
	}
	return nil
}

// getCachedMovies returns the raw Radarr movie list, caching it for the lifetime
// of this client instance (one poll cycle). Used by both GetMediaItems() and
// ResolveCollectionMembers() to avoid redundant /api/v3/movie fetches.
func (r *RadarrClient) getCachedMovies() ([]radarrMovie, error) {
	r.moviesOnce.Do(func() {
		r.cachedMovies, r.cachedMoviesErr = r.fetchMovies()
	})
	return r.cachedMovies, r.cachedMoviesErr
}

// fetchMovies performs the actual HTTP call to fetch all movies from Radarr.
func (r *RadarrClient) fetchMovies() ([]radarrMovie, error) {
	body, err := r.doRequest("/api/v3/movie")
	if err != nil {
		return nil, err
	}
	var movies []radarrMovie
	if err := json.Unmarshal(body, &movies); err != nil {
		return nil, fmt.Errorf("failed to parse movies: %w", err)
	}
	return movies, nil
}

// GetMediaItems fetches all movies from Radarr with quality and tag metadata.
func (r *RadarrClient) GetMediaItems() ([]MediaItem, error) {
	// Fetch quality profiles for name lookup
	profileMap, err := arrFetchQualityProfileMap(r.doRequest, "/api/v3")
	if err != nil {
		return nil, err
	}

	// Fetch tags for name lookup
	tagMap, err := arrFetchTagMap(r.doRequest, "/api/v3")
	if err != nil {
		return nil, err
	}

	// Fetch all movies (cached for reuse by ResolveCollectionMembers)
	movies, err := r.getCachedMovies()
	if err != nil {
		return nil, err
	}

	items := make([]MediaItem, 0, len(movies))
	for _, m := range movies {
		if !m.HasFile {
			continue // Skip movies without files
		}

		// Pick best available rating
		rating := m.Ratings.IMDB.Value
		if rating == 0 {
			rating = m.Ratings.TMDB.Value
		}

		tagNames := arrResolveTagNames(m.Tags, tagMap)

		addedAt := radarrResolveAddedAt(m.MovieFile, m.Added)

		// Extract collection name if present
		var collections []string
		if m.Collection != nil && m.Collection.Name != "" {
			collections = []string{m.Collection.Name}
		}

		items = append(items, MediaItem{
			ExternalID:     strconv.Itoa(m.ID),
			Type:           MediaTypeMovie,
			Title:          m.Title,
			Year:           m.Year,
			TMDbID:         m.TmdbID,
			SizeBytes:      m.SizeOnDisk,
			Path:           m.Path,
			PosterURL:      arrExtractPosterURL(m.Images, r.URL),
			QualityProfile: profileMap[m.QualityProfileID],
			Rating:         rating,
			Genre:          strings.Join(m.Genres, ", "),
			Monitored:      m.Monitored,
			Language:       m.OriginalLanguage.Name,
			Tags:           tagNames,
			AddedAt:        addedAt,
			Collections:    collections,
		})
	}

	return items, nil
}

// GetQualityProfiles, GetTags, GetLanguages are provided by arrBaseClient.

// DeleteMediaItem removes a movie and its files from disk via the Radarr API.
func (r *RadarrClient) DeleteMediaItem(item MediaItem) error {
	endpoint := fmt.Sprintf("/api/v3/movie/%s?deleteFiles=true", item.ExternalID)
	return arrSimpleDelete(r.URL, r.APIKey, endpoint)
}

// --- CollectionResolver implementation ---

// ResolveCollectionMembers returns all movies in the same TMDb collection as the
// given item. Uses the cached movie list from GetMediaItems() to avoid redundant
// API calls. Returns nil if the item has no collection membership.
func (r *RadarrClient) ResolveCollectionMembers(item MediaItem) ([]MediaItem, error) {
	if len(item.Collections) == 0 {
		return nil, nil
	}

	// Use cached movie list (shared with GetMediaItems)
	movies, err := r.getCachedMovies()
	if err != nil {
		return nil, fmt.Errorf("failed to get movies for collection resolution: %w", err)
	}

	// Find the target collection TMDb ID by matching the item's collection name
	targetCollectionName := item.Collections[0]
	var targetCollectionTmdbID int
	for _, m := range movies {
		if m.Collection != nil && m.Collection.Name == targetCollectionName {
			targetCollectionTmdbID = m.Collection.TmdbID
			break
		}
	}
	if targetCollectionTmdbID == 0 {
		return nil, nil // Collection not found in Radarr library
	}

	// Fetch quality profiles and tags for full MediaItem construction
	profileMap, err := arrFetchQualityProfileMap(r.doRequest, "/api/v3")
	if err != nil {
		profileMap = make(map[int]string)
	}
	tagMap, err := arrFetchTagMap(r.doRequest, "/api/v3")
	if err != nil {
		tagMap = make(map[int]string)
	}

	// Collect all movies in the same collection
	var members []MediaItem
	for _, m := range movies {
		if !m.HasFile || m.Collection == nil || m.Collection.TmdbID != targetCollectionTmdbID {
			continue
		}

		rating := m.Ratings.IMDB.Value
		if rating == 0 {
			rating = m.Ratings.TMDB.Value
		}
		tagNames := arrResolveTagNames(m.Tags, tagMap)

		addedAt := radarrResolveAddedAt(m.MovieFile, m.Added)

		members = append(members, MediaItem{
			ExternalID:        strconv.Itoa(m.ID),
			IntegrationID:     item.IntegrationID,
			Type:              MediaTypeMovie,
			Title:             m.Title,
			Year:              m.Year,
			TMDbID:            m.TmdbID,
			SizeBytes:         m.SizeOnDisk,
			Path:              m.Path,
			PosterURL:         arrExtractPosterURL(m.Images, r.URL),
			QualityProfile:    profileMap[m.QualityProfileID],
			Rating:            rating,
			Genre:             strings.Join(m.Genres, ", "),
			Monitored:         m.Monitored,
			Language:          m.OriginalLanguage.Name,
			Tags:              tagNames,
			AddedAt:           addedAt,
			Collections:       []string{m.Collection.Name},
			CollectionSources: map[string]uint{m.Collection.Name: item.IntegrationID},
		})
	}

	if len(members) <= 1 {
		return nil, nil // Only the trigger item itself — no expansion needed
	}

	return members, nil
}

// Verify RadarrClient satisfies CollectionResolver at compile time.
var _ CollectionResolver = (*RadarrClient)(nil)
