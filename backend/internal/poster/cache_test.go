package poster

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCache_StoreGetDelete(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewCache(dir)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	key := CacheKey(1, 12345, "abc123")
	data := []byte("fake poster data")

	// Store
	if err := cache.Store(key, data); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Has
	if !cache.Has(key) {
		t.Error("Expected Has=true after Store")
	}

	// Get
	got, found, err := cache.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Fatal("Expected found=true after Store")
	}
	if string(got) != string(data) {
		t.Errorf("Expected %q, got %q", data, got)
	}

	// Delete
	if err := cache.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if cache.Has(key) {
		t.Error("Expected Has=false after Delete")
	}
}

func TestCache_GetMissing(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewCache(dir)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	_, found, err := cache.Get("nonexistent.jpg")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if found {
		t.Error("Expected found=false for missing key")
	}
}

func TestCache_DeleteMissing(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewCache(dir)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	// Should not error on missing key
	if err := cache.Delete("nonexistent.jpg"); err != nil {
		t.Fatalf("Delete failed on missing key: %v", err)
	}
}

func TestCache_ListAll(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewCache(dir)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	// Empty
	keys, err := cache.ListAll()
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys, got %d", len(keys))
	}

	// Add two entries
	_ = cache.Store("1_100_abc.jpg", []byte("a"))
	_ = cache.Store("2_200_def.jpg", []byte("b"))

	keys, err = cache.ListAll()
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}

func TestCache_ListAll_IgnoresNonJPG(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewCache(dir)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}

	_ = cache.Store("1_100_abc.jpg", []byte("a"))
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a poster"), 0o600)

	keys, err := cache.ListAll()
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("Expected 1 key (ignoring .txt), got %d", len(keys))
	}
}

func TestCacheKey(t *testing.T) {
	key := CacheKey(5, 67890, "aabbcc")
	if key != "5_67890_aabbcc.jpg" {
		t.Errorf("Expected '5_67890_aabbcc.jpg', got %q", key)
	}
}
