package poster

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Cache stores original poster images on the filesystem so they can be
// restored after overlay removal. Keyed by "{integrationID}_{tmdbID}_{hash}.jpg".
type Cache struct {
	dir string
}

// NewCache creates a poster cache rooted at the given directory.
// Creates the directory if it does not exist.
func NewCache(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create poster cache directory %s: %w", dir, err)
	}
	return &Cache{dir: dir}, nil
}

// CacheKey generates a filesystem-safe key for a poster.
func CacheKey(integrationID uint, tmdbID int, contentHash string) string {
	return fmt.Sprintf("%d_%d_%s.jpg", integrationID, tmdbID, contentHash)
}

// Store saves poster image data to the cache.
func (c *Cache) Store(key string, data []byte) error {
	path := filepath.Join(c.dir, key)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write poster cache %s: %w", key, err)
	}
	slog.Debug("Cached original poster", "component", "poster", "key", key, "bytes", len(data))
	return nil
}

// Get retrieves poster image data from the cache.
// Returns the data, whether the key was found, and any error.
func (c *Cache) Get(key string) ([]byte, bool, error) {
	path := filepath.Clean(filepath.Join(c.dir, key))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read poster cache %s: %w", key, err)
	}
	return data, true, nil
}

// Delete removes a cached poster.
func (c *Cache) Delete(key string) error {
	path := filepath.Join(c.dir, key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete poster cache %s: %w", key, err)
	}
	return nil
}

// Has checks whether a cache entry exists without reading the data.
func (c *Cache) Has(key string) bool {
	path := filepath.Join(c.dir, key)
	_, err := os.Stat(path)
	return err == nil
}

// ListAll returns all cache entry keys (filenames without directory).
func (c *Cache) ListAll() ([]string, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, fmt.Errorf("list poster cache: %w", err)
	}

	var keys []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jpg") {
			keys = append(keys, entry.Name())
		}
	}
	return keys, nil
}

// Dir returns the cache directory path.
func (c *Cache) Dir() string {
	return c.dir
}
