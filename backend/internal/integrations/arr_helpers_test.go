package integrations

import "testing"

func TestArrExtractPosterURL(t *testing.T) {
	const testBaseURL = "https://radarr.example.com:7878"

	tests := []struct {
		name     string
		images   []arrImage
		baseURL  string
		expected string
	}{
		{
			name:     "empty array returns empty string",
			images:   []arrImage{},
			baseURL:  testBaseURL,
			expected: "",
		},
		{
			name:     "nil array returns empty string",
			images:   nil,
			baseURL:  testBaseURL,
			expected: "",
		},
		{
			name: "no poster type returns empty string",
			images: []arrImage{
				{CoverType: "banner", RemoteURL: "https://example.com/banner.jpg", URL: "/banner.jpg"},
				{CoverType: "fanart", RemoteURL: "https://example.com/fanart.jpg", URL: "/fanart.jpg"},
			},
			baseURL:  testBaseURL,
			expected: "",
		},
		{
			name: "poster with remoteUrl preferred over url",
			images: []arrImage{
				{CoverType: "banner", RemoteURL: "https://example.com/banner.jpg"},
				{CoverType: "poster", RemoteURL: "https://cdn.example.com/poster.jpg", URL: "/api/v3/mediacover/1/poster.jpg"},
			},
			baseURL:  testBaseURL,
			expected: "https://cdn.example.com/poster.jpg",
		},
		{
			name: "poster with only url resolved against baseURL",
			images: []arrImage{
				{CoverType: "poster", RemoteURL: "", URL: "/api/v3/mediacover/1/poster.jpg"},
			},
			baseURL:  testBaseURL,
			expected: "https://radarr.example.com:7878/api/v3/mediacover/1/poster.jpg",
		},
		{
			name: "poster with only url when remoteUrl is missing resolved against baseURL",
			images: []arrImage{
				{CoverType: "poster", URL: "/api/v3/mediacover/1/poster.jpg"},
			},
			baseURL:  testBaseURL,
			expected: "https://radarr.example.com:7878/api/v3/mediacover/1/poster.jpg",
		},
		{
			name: "poster with only url resolved against baseURL with trailing slash",
			images: []arrImage{
				{CoverType: "poster", RemoteURL: "", URL: "/api/v3/mediacover/1/poster.jpg"},
			},
			baseURL:  "https://radarr.example.com:7878/",
			expected: "https://radarr.example.com:7878/api/v3/mediacover/1/poster.jpg",
		},
		{
			name: "cover type fallback for Readarr books",
			images: []arrImage{
				{CoverType: "cover", RemoteURL: "https://cdn.example.com/cover.jpg", URL: "/cover.jpg"},
			},
			baseURL:  testBaseURL,
			expected: "https://cdn.example.com/cover.jpg",
		},
		{
			name: "cover type with only url resolved against baseURL",
			images: []arrImage{
				{CoverType: "cover", RemoteURL: "", URL: "/api/v1/mediacover/1/cover.jpg"},
			},
			baseURL:  testBaseURL,
			expected: "https://radarr.example.com:7878/api/v1/mediacover/1/cover.jpg",
		},
		{
			name: "poster type preferred over cover type",
			images: []arrImage{
				{CoverType: "cover", RemoteURL: "https://cdn.example.com/cover.jpg"},
				{CoverType: "poster", RemoteURL: "https://cdn.example.com/poster.jpg"},
			},
			baseURL:  testBaseURL,
			expected: "https://cdn.example.com/poster.jpg",
		},
		{
			name: "first poster wins if multiple posterURLs exist",
			images: []arrImage{
				{CoverType: "poster", RemoteURL: "https://cdn.example.com/first-poster.jpg"},
				{CoverType: "poster", RemoteURL: "https://cdn.example.com/second-poster.jpg"},
			},
			baseURL:  testBaseURL,
			expected: "https://cdn.example.com/first-poster.jpg",
		},
		{
			name: "empty baseURL leaves relative path as-is",
			images: []arrImage{
				{CoverType: "poster", URL: "/api/v3/mediacover/1/poster.jpg"},
			},
			baseURL:  "",
			expected: "/api/v3/mediacover/1/poster.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := arrExtractPosterURL(tt.images, tt.baseURL)
			if result != tt.expected {
				t.Errorf("arrExtractPosterURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResolveArrImageURL(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		baseURL  string
		expected string
	}{
		{name: "empty path returns empty", path: "", baseURL: "https://arr.local", expected: ""},
		{name: "absolute URL returned as-is", path: "https://cdn.example.com/poster.jpg", baseURL: "https://arr.local", expected: "https://cdn.example.com/poster.jpg"},
		{name: "http URL returned as-is", path: "http://cdn.example.com/poster.jpg", baseURL: "https://arr.local", expected: "http://cdn.example.com/poster.jpg"},
		{name: "relative path resolved", path: "/MediaCover/123/poster.jpg", baseURL: "https://arr.local:7878", expected: "https://arr.local:7878/MediaCover/123/poster.jpg"},
		{name: "trailing slash on baseURL handled", path: "/MediaCover/123/poster.jpg", baseURL: "https://arr.local:7878/", expected: "https://arr.local:7878/MediaCover/123/poster.jpg"},
		{name: "empty baseURL returns path as-is", path: "/MediaCover/123/poster.jpg", baseURL: "", expected: "/MediaCover/123/poster.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveArrImageURL(tt.path, tt.baseURL)
			if result != tt.expected {
				t.Errorf("resolveArrImageURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}
