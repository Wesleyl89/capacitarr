package services

import (
	"testing"
	"time"

	"capacitarr/internal/db"
	"capacitarr/internal/integrations"
)

// ── Mock implementations ────────────────────────────────────────────────────
// NOTE: mockPreviewSource and mockRulesSource are defined in analytics_test.go
// (same package). We reuse them here to avoid redeclaration.

// watchAnalyticsDiskGroupMock implements DiskGroupLister for watch analytics tests.
type watchAnalyticsDiskGroupMock struct {
	groups map[uint]*db.DiskGroup
}

func (m *watchAnalyticsDiskGroupMock) List() ([]db.DiskGroup, error) {
	result := make([]db.DiskGroup, 0, len(m.groups))
	for _, g := range m.groups {
		result = append(result, *g)
	}
	return result, nil
}

func (m *watchAnalyticsDiskGroupMock) GetByID(id uint) (*db.DiskGroup, error) {
	g, ok := m.groups[id]
	if !ok {
		return nil, ErrNotFound
	}
	return g, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func timePtr(t time.Time) *time.Time {
	return &t
}

func uintPtr(v uint) *uint {
	return &v
}

// ── Tests: GetDeadContent ───────────────────────────────────────────────────

func TestGetDeadContent_ReturnsNeverWatchedItems(t *testing.T) {
	now := time.Now()
	addedLongAgo := now.Add(-120 * 24 * time.Hour) // 120 days ago

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title:       "Serenity",
				Type:        integrations.MediaTypeMovie,
				SizeBytes:   5_000_000_000,
				PlayCount:   0,
				AddedAt:     timePtr(addedLongAgo),
				IsRequested: true, // Enrichment signal: item was requested but never watched
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetDeadContent(90, nil)

	if report.TotalCount != 1 {
		t.Errorf("Expected 1 dead item, got %d", report.TotalCount)
	}
	if report.TotalSize != 5_000_000_000 {
		t.Errorf("Expected total size 5000000000, got %d", report.TotalSize)
	}
	if len(report.Items) != 1 || report.Items[0].Title != "Serenity" {
		t.Errorf("Expected dead item title 'Serenity', got %v", report.Items)
	}
}

func TestGetDeadContent_ExcludesWatchedItems(t *testing.T) {
	now := time.Now()
	addedLongAgo := now.Add(-120 * 24 * time.Hour)

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title:      "Firefly",
				Type:       integrations.MediaTypeShow,
				SizeBytes:  10_000_000_000,
				PlayCount:  3,
				AddedAt:    timePtr(addedLongAgo),
				LastPlayed: timePtr(now.Add(-30 * 24 * time.Hour)),
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetDeadContent(90, nil)

	if report.TotalCount != 0 {
		t.Errorf("Expected 0 dead items (item was watched), got %d", report.TotalCount)
	}
}

func TestGetDeadContent_ExcludesWatchlistedItems(t *testing.T) {
	now := time.Now()
	addedLongAgo := now.Add(-120 * 24 * time.Hour)

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title:       "Serenity",
				Type:        integrations.MediaTypeMovie,
				SizeBytes:   2_000_000_000,
				PlayCount:   0,
				OnWatchlist: true,
				AddedAt:     timePtr(addedLongAgo),
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetDeadContent(90, nil)

	if report.TotalCount != 0 {
		t.Errorf("Expected 0 dead items (item is on watchlist), got %d", report.TotalCount)
	}
}

func TestGetDeadContent_ExcludesRecentItems(t *testing.T) {
	now := time.Now()
	addedRecently := now.Add(-30 * 24 * time.Hour) // 30 days ago (< 90 day threshold)

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title:     "Serenity",
				Type:      integrations.MediaTypeMovie,
				SizeBytes: 2_000_000_000,
				PlayCount: 0,
				AddedAt:   timePtr(addedRecently),
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetDeadContent(90, nil)

	if report.TotalCount != 0 {
		t.Errorf("Expected 0 dead items (item added too recently), got %d", report.TotalCount)
	}
}

func TestGetDeadContent_ExcludesItemsWithoutEnrichment(t *testing.T) {
	now := time.Now()
	addedLongAgo := now.Add(-120 * 24 * time.Hour)

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title:     "Serenity",
				Type:      integrations.MediaTypeMovie,
				SizeBytes: 2_000_000_000,
				PlayCount: 0,
				AddedAt:   timePtr(addedLongAgo),
				// No enrichment data: PlayCount=0, LastPlayed=nil, OnWatchlist=false, IsRequested=false, WatchedByUsers=nil
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetDeadContent(90, nil)

	// hasEnrichmentData requires at least one enrichment signal
	if report.TotalCount != 0 {
		t.Errorf("Expected 0 dead items (no enrichment data), got %d", report.TotalCount)
	}
}

func TestGetDeadContent_SortsBySize(t *testing.T) {
	now := time.Now()
	addedLongAgo := now.Add(-120 * 24 * time.Hour)

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title: "Serenity", Type: integrations.MediaTypeMovie,
				SizeBytes: 2_000_000_000, PlayCount: 0,
				AddedAt: timePtr(addedLongAgo), OnWatchlist: false, IsRequested: true,
			},
			{
				Title: "Firefly", Type: integrations.MediaTypeShow,
				SizeBytes: 10_000_000_000, PlayCount: 0,
				AddedAt: timePtr(addedLongAgo), OnWatchlist: false, IsRequested: true,
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetDeadContent(90, nil)

	if report.TotalCount != 2 {
		t.Fatalf("Expected 2 dead items, got %d", report.TotalCount)
	}
	// Firefly (10GB) should sort before Serenity (2GB) — descending by size
	if report.Items[0].Title != "Firefly" {
		t.Errorf("Expected items sorted by size desc, first item was %q", report.Items[0].Title)
	}
}

func TestGetDeadContent_CountsProtectedItems(t *testing.T) {
	now := time.Now()
	addedLongAgo := now.Add(-120 * 24 * time.Hour)
	integrationID := uint(1)

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title: "Serenity", Type: integrations.MediaTypeMovie,
				SizeBytes: 2_000_000_000, PlayCount: 0,
				AddedAt: timePtr(addedLongAgo), IsRequested: true,
				IntegrationID: integrationID,
			},
		},
	}

	rules := &mockRulesSource{
		rules: []db.CustomRule{
			{
				Enabled:       true,
				Effect:        "always_keep",
				Field:         "title",
				Operator:      "contains",
				Value:         "Serenity",
				IntegrationID: &integrationID,
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	svc.SetRulesSource(rules)
	report := svc.GetDeadContent(90, nil)

	if report.TotalCount != 0 {
		t.Errorf("Expected 0 dead items (protected by rule), got %d", report.TotalCount)
	}
	if report.ProtectedCount != 1 {
		t.Errorf("Expected 1 protected item, got %d", report.ProtectedCount)
	}
}

// ── Tests: GetStaleContent ──────────────────────────────────────────────────

func TestGetStaleContent_ReturnsLongUnwatchedItems(t *testing.T) {
	now := time.Now()
	lastPlayed := now.Add(-200 * 24 * time.Hour) // 200 days ago

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title:      "Firefly",
				Type:       integrations.MediaTypeShow,
				SizeBytes:  10_000_000_000,
				PlayCount:  5,
				LastPlayed: timePtr(lastPlayed),
				AddedAt:    timePtr(now.Add(-365 * 24 * time.Hour)),
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetStaleContent(180, nil)

	if report.TotalCount != 1 {
		t.Errorf("Expected 1 stale item, got %d", report.TotalCount)
	}
	if report.TotalSize != 10_000_000_000 {
		t.Errorf("Expected total size 10000000000, got %d", report.TotalSize)
	}
	if len(report.Items) != 1 || report.Items[0].Title != "Firefly" {
		t.Errorf("Expected stale item title 'Firefly', got %v", report.Items)
	}
	if report.Items[0].PlayCount != 5 {
		t.Errorf("Expected play count 5, got %d", report.Items[0].PlayCount)
	}
}

func TestGetStaleContent_ExcludesNeverWatchedItems(t *testing.T) {
	now := time.Now()

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title:     "Serenity",
				Type:      integrations.MediaTypeMovie,
				SizeBytes: 2_000_000_000,
				PlayCount: 0,
				AddedAt:   timePtr(now.Add(-365 * 24 * time.Hour)),
				// PlayCount=0 and LastPlayed=nil → not stale (it's dead, not stale)
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetStaleContent(180, nil)

	if report.TotalCount != 0 {
		t.Errorf("Expected 0 stale items (never watched), got %d", report.TotalCount)
	}
}

func TestGetStaleContent_ExcludesRecentlyWatchedItems(t *testing.T) {
	now := time.Now()
	lastPlayed := now.Add(-30 * 24 * time.Hour) // 30 days ago (< 180 threshold)

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title:      "Firefly",
				Type:       integrations.MediaTypeShow,
				SizeBytes:  10_000_000_000,
				PlayCount:  3,
				LastPlayed: timePtr(lastPlayed),
				AddedAt:    timePtr(now.Add(-365 * 24 * time.Hour)),
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetStaleContent(180, nil)

	if report.TotalCount != 0 {
		t.Errorf("Expected 0 stale items (watched recently), got %d", report.TotalCount)
	}
}

func TestGetStaleContent_StalenessScoreHigherForEndedSeries(t *testing.T) {
	now := time.Now()
	lastPlayed := now.Add(-365 * 24 * time.Hour) // 365 days ago

	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{
				Title: "Firefly", Type: integrations.MediaTypeShow,
				SizeBytes: 10_000_000_000, PlayCount: 5,
				LastPlayed: timePtr(lastPlayed), SeriesStatus: "ended",
				AddedAt: timePtr(now.Add(-730 * 24 * time.Hour)),
			},
			{
				Title: "Serenity", Type: integrations.MediaTypeMovie,
				SizeBytes: 2_000_000_000, PlayCount: 1,
				LastPlayed: timePtr(lastPlayed), SeriesStatus: "",
				AddedAt: timePtr(now.Add(-730 * 24 * time.Hour)),
			},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	report := svc.GetStaleContent(180, nil)

	if report.TotalCount != 2 {
		t.Fatalf("Expected 2 stale items, got %d", report.TotalCount)
	}

	// "Firefly" (ended) should have higher staleness score than "Serenity" (not ended)
	// and should sort first (descending by staleness score)
	if report.Items[0].Title != "Firefly" {
		t.Errorf("Expected ended series 'Firefly' to rank first by staleness score, got %q", report.Items[0].Title)
	}
	if report.Items[0].StalenessScore <= report.Items[1].StalenessScore {
		t.Errorf("Expected ended series to have higher staleness score: %f vs %f",
			report.Items[0].StalenessScore, report.Items[1].StalenessScore)
	}
}

// ── Tests: filterItemsByDiskGroup ───────────────────────────────────────────

func TestFilterItemsByDiskGroup_NilDiskGroupIDReturnsAll(t *testing.T) {
	now := time.Now()
	preview := &mockPreviewSource{
		items: []integrations.MediaItem{
			{Title: "Firefly", Path: "/media/shows/Firefly", AddedAt: timePtr(now), PlayCount: 1, LastPlayed: timePtr(now)},
			{Title: "Serenity", Path: "/other/movies/Serenity", AddedAt: timePtr(now), PlayCount: 1, LastPlayed: timePtr(now)},
		},
	}

	svc := NewWatchAnalyticsService(preview)
	// GetStaleContent with staleDays=0 is irrelevant here — we just need the service
	// instance to test the filter method below.
	_ = svc.GetStaleContent(0, nil)

	// Directly test the filter method to verify nil diskGroupID passthrough
	items := svc.filterItemsByDiskGroup(preview.items, nil)
	if len(items) != 2 {
		t.Errorf("Expected 2 items with nil diskGroupID, got %d", len(items))
	}
}

func TestFilterItemsByDiskGroup_FiltersByMountPath(t *testing.T) {
	now := time.Now()
	items := []integrations.MediaItem{
		{Title: "Firefly", Path: "/media/shows/Firefly", AddedAt: timePtr(now)},
		{Title: "Serenity", Path: "/other/movies/Serenity", AddedAt: timePtr(now)},
		{Title: "Firefly S1", Path: "/media/shows/Firefly/Season 1", AddedAt: timePtr(now)},
	}

	dg := &watchAnalyticsDiskGroupMock{
		groups: map[uint]*db.DiskGroup{
			1: {MountPath: "/media"},
		},
	}
	dg.groups[1].ID = 1

	svc := NewWatchAnalyticsService(&mockPreviewSource{items: items})
	svc.SetDiskGroupLister(dg)

	dgID := uintPtr(1)
	filtered := svc.filterItemsByDiskGroup(items, dgID)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 items on /media mount, got %d", len(filtered))
	}
	for _, item := range filtered {
		if item.Title == "Serenity" {
			t.Errorf("Serenity (/other/movies/) should not be in /media filtered results")
		}
	}
}

func TestFilterItemsByDiskGroup_UnknownGroupReturnsAll(t *testing.T) {
	items := []integrations.MediaItem{
		{Title: "Firefly", Path: "/media/shows/Firefly"},
	}

	dg := &watchAnalyticsDiskGroupMock{groups: map[uint]*db.DiskGroup{}}

	svc := NewWatchAnalyticsService(&mockPreviewSource{items: items})
	svc.SetDiskGroupLister(dg)

	// Request unknown disk group ID — should return all items
	dgID := uintPtr(999)
	filtered := svc.filterItemsByDiskGroup(items, dgID)

	if len(filtered) != 1 {
		t.Errorf("Expected 1 item (unknown group returns all), got %d", len(filtered))
	}
}

// ── Tests: hasEnrichmentData ────────────────────────────────────────────────

func TestHasEnrichmentData(t *testing.T) {
	tests := []struct {
		name     string
		item     integrations.MediaItem
		expected bool
	}{
		{
			name:     "no enrichment data",
			item:     integrations.MediaItem{Title: "Serenity"},
			expected: false,
		},
		{
			name:     "has play count",
			item:     integrations.MediaItem{Title: "Serenity", PlayCount: 1},
			expected: true,
		},
		{
			name:     "on watchlist",
			item:     integrations.MediaItem{Title: "Serenity", OnWatchlist: true},
			expected: true,
		},
		{
			name:     "is requested",
			item:     integrations.MediaItem{Title: "Serenity", IsRequested: true},
			expected: true,
		},
		{
			name:     "has watched by users",
			item:     integrations.MediaItem{Title: "Serenity", WatchedByUsers: []string{"user1"}},
			expected: true,
		},
		{
			name: "has last played",
			item: integrations.MediaItem{
				Title:      "Serenity",
				LastPlayed: timePtr(time.Now()),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasEnrichmentData(tt.item)
			if result != tt.expected {
				t.Errorf("hasEnrichmentData(%q) = %v, expected %v", tt.item.Title, result, tt.expected)
			}
		})
	}
}

// ── Tests: stalenessScore ───────────────────────────────────────────────────

func TestStalenessScore(t *testing.T) {
	tests := []struct {
		name       string
		item       integrations.MediaItem
		daysSince  int
		wantHigher bool // true if score should be > 1.0
	}{
		{
			name:       "365 days, continuing series, not on watchlist",
			item:       integrations.MediaItem{SeriesStatus: "continuing", OnWatchlist: false},
			daysSince:  365,
			wantHigher: true, // 1.0 * 1.0 * 1.2 = 1.2
		},
		{
			name:       "365 days, ended series, not on watchlist",
			item:       integrations.MediaItem{SeriesStatus: "ended", OnWatchlist: false},
			daysSince:  365,
			wantHigher: true, // 1.0 * 1.5 * 1.2 = 1.8
		},
		{
			name:       "365 days, ended series, on watchlist",
			item:       integrations.MediaItem{SeriesStatus: "ended", OnWatchlist: true},
			daysSince:  365,
			wantHigher: false, // 1.0 * 1.5 * 0.5 = 0.75
		},
		{
			name:       "180 days, continuing series, not on watchlist",
			item:       integrations.MediaItem{SeriesStatus: "continuing", OnWatchlist: false},
			daysSince:  180,
			wantHigher: false, // ~0.49 * 1.0 * 1.2 = ~0.59
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := stalenessScore(tt.item, tt.daysSince)
			if tt.wantHigher && score <= 1.0 {
				t.Errorf("Expected score > 1.0, got %f", score)
			}
			if !tt.wantHigher && score > 1.0 {
				t.Errorf("Expected score <= 1.0, got %f", score)
			}
			if score < 0 {
				t.Errorf("Score should never be negative, got %f", score)
			}
		})
	}

	// Verify ended series has higher score than continuing
	endedScore := stalenessScore(integrations.MediaItem{SeriesStatus: "ended"}, 365)
	continuingScore := stalenessScore(integrations.MediaItem{SeriesStatus: "continuing"}, 365)
	if endedScore <= continuingScore {
		t.Errorf("Ended series should have higher staleness: ended=%f, continuing=%f", endedScore, continuingScore)
	}

	// Verify watchlisted items have lower score
	notWatchlisted := stalenessScore(integrations.MediaItem{OnWatchlist: false}, 365)
	watchlisted := stalenessScore(integrations.MediaItem{OnWatchlist: true}, 365)
	if watchlisted >= notWatchlisted {
		t.Errorf("Watchlisted should have lower staleness: watchlisted=%f, not=%f", watchlisted, notWatchlisted)
	}
}
