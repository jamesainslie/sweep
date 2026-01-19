package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/types"
)

// ValidationResult contains the results of cache validation.
type ValidationResult struct {
	// ValidFiles are files from cache that are still valid (unchanged).
	ValidFiles []types.FileInfo

	// StaleDirs are directories that need to be rescanned.
	StaleDirs []string

	// DeletedPaths are paths that no longer exist and should be removed from cache.
	DeletedPaths []string
}

// Validator validates cached entries against the filesystem.
type Validator struct {
	store *Store
}

// NewValidator creates a new cache validator.
func NewValidator(store *Store) *Validator {
	return &Validator{store: store}
}

// Validate checks the cache against the filesystem and returns what's stale.
// For performance, this only checks the root directory mtime.
// If unchanged, all cached files are considered valid.
// If changed, the entire tree is marked as stale for a full rescan.
func (v *Validator) Validate(root string) (*ValidationResult, error) {
	result := &ValidationResult{}

	// Get cached root entry
	cachedRoot, err := v.store.Get(root, "")
	if errors.Is(err, ErrNotFound) {
		// No cache for this root - entire tree is stale
		result.StaleDirs = []string{root}
		return result, nil
	}
	if err != nil {
		return nil, err
	}

	// Stat the actual root
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat root %s: %w", root, err)
	}

	// Check if root mtime changed
	if rootInfo.ModTime().UnixNano() != cachedRoot.Mtime {
		// Root changed - do a full rescan
		// This is a conservative approach: any change in the tree triggers full rescan
		result.StaleDirs = []string{root}
		return result, nil
	}

	// Root unchanged - collect all cached files without additional stat calls
	if err := v.collectCachedFiles(root, "", result); err != nil {
		return nil, err
	}

	return result, nil
}

// collectCachedFiles recursively collects all files from a cached subtree.
// This is a fast O(cached entries) operation that does NO stat calls.
// It trusts the cache completely since root mtime was already verified.
func (v *Validator) collectCachedFiles(root, relPath string, result *ValidationResult) error {
	cached, err := v.store.Get(root, relPath)
	if err != nil {
		return err
	}

	fullPath := root
	if relPath != "" {
		fullPath = filepath.Join(root, relPath)
	}

	if !cached.IsDir {
		// It's a file - trust cached data completely (no stat)
		result.ValidFiles = append(result.ValidFiles, types.FileInfo{
			Path:    fullPath,
			Size:    cached.Size,
			ModTime: time.Unix(0, cached.Mtime),
		})
		return nil
	}

	// It's a directory - recurse into children (no stat)
	for _, childName := range cached.Children {
		childRelPath := childName
		if relPath != "" {
			childRelPath = filepath.Join(relPath, childName)
		}
		if err := v.collectCachedFiles(root, childRelPath, result); err != nil {
			return err
		}
	}

	return nil
}
