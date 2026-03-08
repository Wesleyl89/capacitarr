package main

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

// sampleHTML mirrors the structure of a real Nuxt-generated index.html.
// It contains all the patterns that rewriteHTML must handle.
const sampleHTML = `<!DOCTYPE html><html><head>` +
	`<link rel="stylesheet" href="/_assets/entry.DeJAGcQG.css" crossorigin>` +
	`<link rel="modulepreload" as="script" crossorigin href="/_assets/BZLeI64h.js">` +
	`<script type="module" src="/_assets/BZLeI64h.js" crossorigin></script>` +
	`</head><body>` +
	`<div id="__nuxt"></div>` +
	`<script>window.__NUXT__={};window.__NUXT__.config={public:{apiBaseUrl:""},app:{baseURL:"/",buildId:"test-build",buildAssetsDir:"/_assets/",cdnURL:""}}</script>` +
	`</body></html>`

func TestRewriteHTML_SubdirectoryPath(t *testing.T) {
	result := string(rewriteHTML([]byte(sampleHTML), "/capacitarr/"))

	// Asset paths should be rewritten
	assertContains(t, result, `href="/capacitarr/_assets/entry.DeJAGcQG.css"`)
	assertContains(t, result, `href="/capacitarr/_assets/BZLeI64h.js"`)
	assertContains(t, result, `src="/capacitarr/_assets/BZLeI64h.js"`)

	// Nuxt config should be rewritten (keys are unquoted in minified Nuxt output)
	assertContains(t, result, `baseURL:"/capacitarr/"`)
	assertContains(t, result, `apiBaseUrl:"/capacitarr"`)

	// buildAssetsDir should NOT be rewritten — Nuxt treats it as relative to baseURL
	assertContains(t, result, `buildAssetsDir:"/_assets/"`)

	// Original root paths should NOT be present
	assertNotContains(t, result, `"/_assets/entry.DeJAGcQG.css"`)
	assertNotContains(t, result, `baseURL:"/"`)
}

func TestRewriteHTML_NestedSubdirectory(t *testing.T) {
	result := string(rewriteHTML([]byte(sampleHTML), "/apps/media/capacitarr/"))

	assertContains(t, result, `href="/apps/media/capacitarr/_assets/entry.DeJAGcQG.css"`)
	assertContains(t, result, `baseURL:"/apps/media/capacitarr/"`)
	assertContains(t, result, `apiBaseUrl:"/apps/media/capacitarr"`)

	// buildAssetsDir should NOT be rewritten
	assertContains(t, result, `buildAssetsDir:"/_assets/"`)
}

func TestRewriteHTML_RootPath_NoOp(t *testing.T) {
	// When baseURL is "/", rewriteHTML should still work (no-op for most replacements)
	// but the apiBaseUrl should be set to ""
	result := string(rewriteHTML([]byte(sampleHTML), "/"))

	// Asset paths should remain unchanged (replacing "/_assets/" with "/_assets/" is identity)
	assertContains(t, result, `href="/_assets/entry.DeJAGcQG.css"`)

	// baseURL should remain "/"
	assertContains(t, result, `baseURL:"/"`)

	// buildAssetsDir should remain "/_assets/"
	assertContains(t, result, `buildAssetsDir:"/_assets/"`)

	// apiBaseUrl should be empty string (no prefix needed)
	assertContains(t, result, `apiBaseUrl:""`)
}

func TestRewriteHTML_NormalizesBaseURL(t *testing.T) {
	// Missing leading slash
	result := string(rewriteHTML([]byte(sampleHTML), "capacitarr/"))
	assertContains(t, result, `baseURL:"/capacitarr/"`)

	// Missing trailing slash
	result = string(rewriteHTML([]byte(sampleHTML), "/capacitarr"))
	assertContains(t, result, `baseURL:"/capacitarr/"`)

	// Missing both
	result = string(rewriteHTML([]byte(sampleHTML), "capacitarr"))
	assertContains(t, result, `baseURL:"/capacitarr/"`)
}

func TestRewriteHTML_WithExistingApiBaseUrl(t *testing.T) {
	// When apiBaseUrl has a non-empty value (e.g. from dev build), it should be replaced
	htmlWithAPIBase := `<script>window.__NUXT__={};window.__NUXT__.config={public:{apiBaseUrl:"http://localhost:8080"},app:{baseURL:"/",buildAssetsDir:"/_assets/"}}</script>`

	result := string(rewriteHTML([]byte(htmlWithAPIBase), "/capacitarr/"))
	assertContains(t, result, `apiBaseUrl:"/capacitarr"`)
	assertNotContains(t, result, `apiBaseUrl:"http://localhost:8080"`)
}

func TestBuildHTMLCache_RootPath_ReturnsNil(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(sampleHTML)},
	}

	cache := buildHTMLCache(fsys, "/")
	if cache != nil {
		t.Error("expected nil cache for root baseURL, got non-nil")
	}
}

func TestBuildHTMLCache_SubdirectoryPath(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(sampleHTML)},
	}

	cache := buildHTMLCache(fsys, "/capacitarr/")
	if cache == nil {
		t.Fatal("expected non-nil cache for subdirectory baseURL")
	}
	if cache.index == nil {
		t.Error("expected non-nil index in cache")
	}
	if cache.spa != nil {
		t.Error("expected nil spa in cache (no 200.html in test FS)")
	}

	// Verify the cached HTML is rewritten
	assertContains(t, string(cache.index), `baseURL:"/capacitarr/"`)
}

func TestBuildHTMLCache_WithSPAFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(sampleHTML)},
		"200.html":   &fstest.MapFile{Data: []byte(sampleHTML)},
	}

	cache := buildHTMLCache(fsys, "/capacitarr/")
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}
	if cache.index == nil {
		t.Error("expected non-nil index in cache")
	}
	if cache.spa == nil {
		t.Error("expected non-nil spa in cache")
	}

	// Both should be rewritten
	assertContains(t, string(cache.index), `baseURL:"/capacitarr/"`)
	assertContains(t, string(cache.spa), `baseURL:"/capacitarr/"`)
}

func TestBuildHTMLCache_MissingIndexHTML(t *testing.T) {
	fsys := fstest.MapFS{} // empty filesystem

	cache := buildHTMLCache(fsys, "/capacitarr/")
	if cache != nil {
		t.Error("expected nil cache when index.html is missing")
	}
}

func TestReadFSFile(t *testing.T) {
	content := []byte("hello world")
	fsys := fstest.MapFS{
		"test.txt": &fstest.MapFile{Data: content},
	}

	data, err := readFSFile(fsys, "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestReadFSFile_NotFound(t *testing.T) {
	fsys := fstest.MapFS{}

	_, err := readFSFile(fsys, "missing.txt")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// assertContains checks that s contains substr.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !contains(s, substr) {
		t.Errorf("expected string to contain %q, but it did not.\nFull string:\n%s", substr, s)
	}
}

// assertNotContains checks that s does NOT contain substr.
func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if contains(s, substr) {
		t.Errorf("expected string to NOT contain %q, but it did.\nFull string:\n%s", substr, s)
	}
}

func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && containsString(s, substr)
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure the test file uses the fs package (for interface compliance).
var _ fs.FS = fstest.MapFS{}
