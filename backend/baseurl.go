package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"regexp"
	"strings"
)

// htmlCache holds pre-rewritten HTML entry points for subdirectory deployments.
// When BASE_URL is "/", this is nil and files are served directly from the
// embedded FS. When BASE_URL is e.g. "/capacitarr/", the HTML is rewritten
// once at startup and served from this cache on every request.
type htmlCache struct {
	index []byte // rewritten index.html
	spa   []byte // rewritten 200.html (SPA fallback), may be nil
}

// apiBaseURLPattern matches the Nuxt runtime config apiBaseUrl value.
// Nuxt minifies config keys without quotes, so we match both quoted and
// unquoted key forms: apiBaseUrl:"..." or "apiBaseUrl":"..."
var apiBaseURLPattern = regexp.MustCompile(`"?apiBaseUrl"?:"[^"]*"`)

// buildHTMLCache reads the HTML entry points from the embedded FS, rewrites
// them for the given baseURL, and returns a cache. Returns nil if baseURL is "/".
func buildHTMLCache(fsys fs.FS, baseURL string) *htmlCache {
	if baseURL == "/" {
		return nil
	}

	cache := &htmlCache{}

	// Read and rewrite index.html
	indexData, err := readFSFile(fsys, "index.html")
	if err != nil {
		slog.Error("Failed to read index.html for base URL rewriting",
			"component", "baseurl", "error", err)
		return nil
	}
	cache.index = rewriteHTML(indexData, baseURL)

	// Read and rewrite 200.html (SPA fallback) if it exists
	spaData, err := readFSFile(fsys, "200.html")
	if err == nil {
		cache.spa = rewriteHTML(spaData, baseURL)
	}

	slog.Info("HTML entry points rewritten for subdirectory deployment",
		"component", "baseurl",
		"baseURL", baseURL,
		"indexSize", len(cache.index),
		"spaSize", len(cache.spa),
	)

	return cache
}

// readFSFile reads a file from an fs.FS and returns its contents.
func readFSFile(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return data, nil
}

// rewriteHTML rewrites a Nuxt-generated HTML file for subdirectory deployment.
// It performs the following replacements:
//
//  1. Asset paths: "/_assets/" → "{baseURL}_assets/" in href and src attributes
//  2. Nuxt app.baseURL: "baseURL":"/" → "baseURL":"{baseURL}"
//  3. Nuxt buildAssetsDir: "buildAssetsDir":"/_assets/" → "buildAssetsDir":"{baseURL}_assets/"
//  4. Nuxt apiBaseUrl: "apiBaseUrl":"..." → "apiBaseUrl":"{baseURL without trailing slash}"
//
// The baseURL parameter must start and end with "/" (e.g. "/capacitarr/").
func rewriteHTML(html []byte, baseURL string) []byte {
	// Defensive: ensure baseURL has correct format
	if !strings.HasPrefix(baseURL, "/") {
		baseURL = "/" + baseURL
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	// apiBaseUrl should not have a trailing slash (it's a URL prefix for fetch)
	apiBase := strings.TrimRight(baseURL, "/")

	result := html

	// 1. Rewrite asset paths in HTML attributes
	//    href="/_assets/..." → href="{baseURL}_assets/..."
	//    src="/_assets/..."  → src="{baseURL}_assets/..."
	result = bytes.ReplaceAll(result,
		[]byte(`"/_assets/`),
		[]byte(fmt.Sprintf(`"%s_assets/`, baseURL)))

	// 2. Rewrite Nuxt app.baseURL in the __NUXT__ config script
	//    Nuxt minifies config keys without quotes in production builds, so we
	//    must handle both forms: baseURL:"/" and "baseURL":"/"
	result = bytes.ReplaceAll(result,
		[]byte(`baseURL:"/"`),
		[]byte(fmt.Sprintf(`baseURL:"%s"`, baseURL)))

	// 3. Restore buildAssetsDir to its original value. Step 1's blanket replacement
	//    also caught buildAssetsDir:"/_assets/" inside the __NUXT__ config. Nuxt
	//    treats buildAssetsDir as relative to baseURL and will prepend the new
	//    baseURL automatically. If we left it rewritten, we'd get a double prefix
	//    (e.g. /capacitarr/capacitarr/_assets/ for dynamic asset loads).
	result = bytes.ReplaceAll(result,
		[]byte(fmt.Sprintf(`buildAssetsDir:"%s_assets/"`, baseURL)),
		[]byte(`buildAssetsDir:"/_assets/"`))

	// 4. Rewrite Nuxt apiBaseUrl in the __NUXT__ config script
	//    apiBaseUrl:"<anything>" → apiBaseUrl:"{apiBase}"
	//    This ensures useApi() and useEventStream() construct correct API paths.
	result = apiBaseURLPattern.ReplaceAll(result,
		[]byte(fmt.Sprintf(`apiBaseUrl:"%s"`, apiBase)))

	return result
}
