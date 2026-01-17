package cache

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// Cache provides high-level caching operations for sweep.
type Cache struct {
	store     *Store
	validator *Validator
}

// Open opens or creates a cache at the given path.
func Open(path string) (*Cache, error) {
	store, err := OpenStore(path)
	if err != nil {
		return nil, err
	}

	return &Cache{
		store:     store,
		validator: NewValidator(store),
	}, nil
}

// Close closes the cache.
func (c *Cache) Close() error {
	return c.store.Close()
}

// ValidateAndGetStale validates the cache and returns valid files and stale directories.
// If the cache is fully valid, staleDirs will be empty.
// If the cache is empty or stale, staleDirs will contain directories to rescan.
func (c *Cache) ValidateAndGetStale(root string) ([]types.FileInfo, []string, error) {
	result, err := c.validator.Validate(root)
	if err != nil {
		return nil, nil, err
	}

	return result.ValidFiles, result.StaleDirs, nil
}

// Update updates the cache with new entries after a scan.
func (c *Cache) Update(root string, entries map[string]*CachedEntry) error {
	return c.store.PutBatch(root, entries)
}

// GetLargeFiles returns all cached files >= minSize for a given root.
func (c *Cache) GetLargeFiles(root string, minSize int64) ([]types.FileInfo, error) {
	var files []types.FileInfo

	// Walk the cached tree and collect large files
	if err := c.walkCachedTree(root, "", minSize, &files); err != nil {
		return nil, err
	}

	return files, nil
}

// walkCachedTree recursively walks the cached tree collecting large files.
func (c *Cache) walkCachedTree(root, relPath string, minSize int64, files *[]types.FileInfo) error {
	entry, err := c.store.Get(root, relPath)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	fullPath := root
	if relPath != "" {
		fullPath = filepath.Join(root, relPath)
	}

	if !entry.IsDir {
		// It's a file - check size
		if entry.Size >= minSize {
			// Get current file info for ModTime
			modTime := time.Time{}
			if info, statErr := os.Stat(fullPath); statErr == nil {
				modTime = info.ModTime()
			}

			*files = append(*files, types.FileInfo{
				Path:    fullPath,
				Size:    entry.Size,
				ModTime: modTime,
			})
		}
		return nil
	}

	// It's a directory - recurse into children
	for _, childName := range entry.Children {
		childRelPath := childName
		if relPath != "" {
			childRelPath = filepath.Join(relPath, childName)
		}
		if err := c.walkCachedTree(root, childRelPath, minSize, files); err != nil {
			return err
		}
	}

	return nil
}

// Clear removes all cached entries for a root.
func (c *Cache) Clear(root string) error {
	return c.store.DeletePrefix(root)
}

// ClearAll removes all cached entries.
func (c *Cache) ClearAll() error {
	// Delete with empty prefix to clear everything
	return c.store.DeletePrefix("")
}
