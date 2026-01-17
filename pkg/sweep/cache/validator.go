package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
		// Root changed - need to validate children
		if err := v.validateDir(root, "", cachedRoot, result); err != nil {
			return nil, err
		}
	} else {
		// Root unchanged - collect all cached files
		if err := v.collectCachedFiles(root, "", result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// validateDir recursively validates a directory and its children.
func (v *Validator) validateDir(root, relPath string, cached *CachedEntry, result *ValidationResult) error {
	fullPath := filepath.Join(root, relPath)
	if relPath == "" {
		fullPath = root
	}

	// Check each cached child
	for _, childName := range cached.Children {
		childRelPath := childName
		if relPath != "" {
			childRelPath = filepath.Join(relPath, childName)
		}
		childFullPath := filepath.Join(root, childRelPath)

		// Stat the child
		childInfo, err := os.Stat(childFullPath)
		if os.IsNotExist(err) {
			// Child was deleted
			result.DeletedPaths = append(result.DeletedPaths, childRelPath)
			continue
		}
		if err != nil {
			// Can't stat - treat as deleted
			result.DeletedPaths = append(result.DeletedPaths, childRelPath)
			continue
		}

		// Get cached child entry
		cachedChild, err := v.store.Get(root, childRelPath)
		if errors.Is(err, ErrNotFound) {
			// Not in cache but exists - parent dir is stale
			result.StaleDirs = append(result.StaleDirs, fullPath)
			return nil
		}
		if err != nil {
			return err
		}

		// Check mtime
		mtimeChanged := childInfo.ModTime().UnixNano() != cachedChild.Mtime
		isDir := childInfo.IsDir()

		switch {
		case mtimeChanged && !isDir:
			// File changed - parent dir is stale (need to rescan)
			result.StaleDirs = append(result.StaleDirs, fullPath)
			return nil
		case mtimeChanged && isDir:
			// Directory changed - mark as stale
			result.StaleDirs = append(result.StaleDirs, childFullPath)
		case !mtimeChanged && isDir:
			// Unchanged directory - recursively collect from subtree
			if err := v.collectCachedFiles(root, childRelPath, result); err != nil {
				return err
			}
		default:
			// Unchanged file - add to valid files
			result.ValidFiles = append(result.ValidFiles, types.FileInfo{
				Path:    childFullPath,
				Size:    cachedChild.Size,
				ModTime: childInfo.ModTime(),
			})
		}
	}

	// Check for new entries (in filesystem but not in cache)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", fullPath, err)
	}

	cachedSet := make(map[string]bool)
	for _, name := range cached.Children {
		cachedSet[name] = true
	}

	for _, entry := range entries {
		if !cachedSet[entry.Name()] {
			// New entry found - this dir is stale
			result.StaleDirs = append(result.StaleDirs, fullPath)
			return nil
		}
	}

	return nil
}

// collectCachedFiles recursively collects all files from an unchanged subtree.
// It validates file mtimes and marks directories as stale if files have changed.
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
		// It's a file - check if it's still valid
		info, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			// File was deleted - mark parent as stale for rescan
			parentDir := filepath.Dir(fullPath)
			result.StaleDirs = append(result.StaleDirs, parentDir)
			return nil
		}
		if err != nil {
			// Stat failed for other reason - propagate error
			return fmt.Errorf("stat file %s: %w", fullPath, err)
		}
		// Check if file mtime changed (content may have been modified in place)
		if info.ModTime().UnixNano() != cached.Mtime {
			// File changed - mark parent as stale
			parentDir := filepath.Dir(fullPath)
			result.StaleDirs = append(result.StaleDirs, parentDir)
			return nil
		}
		result.ValidFiles = append(result.ValidFiles, types.FileInfo{
			Path:    fullPath,
			Size:    cached.Size,
			ModTime: info.ModTime(),
		})
		return nil
	}

	// It's a directory - collect children
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
