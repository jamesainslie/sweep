package main

import (
	"fmt"
	"strings"

	"github.com/jamesainslie/sweep/pkg/sweep/filter"
	"github.com/jamesainslie/sweep/pkg/sweep/types"
	"github.com/spf13/viper"
)

// New flag variables for output and filtering.
var (
	// Output flags
	outputFormat string
	templateStr  string
	columns      string

	// Filter flags
	limit      int
	olderThan  string
	newerThan  string
	fileTypes  string
	extensions string
	include    string
	maxDepth   int
	sortBy     string
	reverse    bool

	// Daemon/cache control
	maxAge      string
	forceDaemon bool
	forceScan   bool
)

// buildFilter creates a filter.Filter from the CLI flags.
func buildFilter() (*filter.Filter, error) {
	var opts []filter.Option

	// Limit
	limitVal := viper.GetInt("limit")
	if limitVal > 0 {
		opts = append(opts, filter.WithLimit(limitVal))
	} else {
		opts = append(opts, filter.WithLimit(0)) // unlimited
	}

	// Min size (from existing flag)
	minSizeStr := viper.GetString("min_size")
	if minSizeStr != "" {
		minSize, err := types.ParseSize(minSizeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid min-size %q: %w", minSizeStr, err)
		}
		opts = append(opts, filter.WithMinSize(minSize))
	}

	// Older than
	olderThanStr := viper.GetString("older_than")
	if olderThanStr != "" {
		d, err := filter.ParseDuration(olderThanStr)
		if err != nil {
			return nil, fmt.Errorf("invalid older-than %q: %w", olderThanStr, err)
		}
		opts = append(opts, filter.WithOlderThan(d))
	}

	// Newer than
	newerThanStr := viper.GetString("newer_than")
	if newerThanStr != "" {
		d, err := filter.ParseDuration(newerThanStr)
		if err != nil {
			return nil, fmt.Errorf("invalid newer-than %q: %w", newerThanStr, err)
		}
		opts = append(opts, filter.WithNewerThan(d))
	}

	// File types (expand to extensions)
	fileTypesStr := viper.GetString("type")
	if fileTypesStr != "" {
		groups := parseCommaSeparated(fileTypesStr)
		opts = append(opts, filter.WithTypeGroups(groups...))
	}

	// Extensions (overrides type groups if both specified)
	extStr := viper.GetString("ext")
	if extStr != "" {
		exts := parseCommaSeparated(extStr)
		// Normalize: ensure each extension starts with a dot
		for i, ext := range exts {
			if !strings.HasPrefix(ext, ".") {
				exts[i] = "." + ext
			}
		}
		opts = append(opts, filter.WithExtensions(exts...))
	}

	// Include patterns
	includeStr := viper.GetString("include")
	if includeStr != "" {
		patterns := parseCommaSeparated(includeStr)
		opts = append(opts, filter.WithInclude(patterns...))
	}

	// Exclude patterns (from existing flag)
	exclude := viper.GetStringSlice("exclude")
	if len(exclude) > 0 {
		opts = append(opts, filter.WithExclude(exclude...))
	}

	// Max depth
	maxDepthVal := viper.GetInt("max_depth")
	if maxDepthVal > 0 {
		opts = append(opts, filter.WithMaxDepth(maxDepthVal))
	}

	// Sort by
	sortByStr := viper.GetString("sort")
	if sortByStr == "" {
		sortByStr = "size"
	}
	sortField, err := filter.ParseSortField(sortByStr)
	if err != nil {
		return nil, fmt.Errorf("invalid sort field %q: %w", sortByStr, err)
	}
	opts = append(opts, filter.WithSortBy(sortField))

	// Sort descending (default true for size, so reverse actually makes it ascending)
	// The --reverse flag reverses the natural order
	reverseVal := viper.GetBool("reverse")
	// For size and age, descending (largest/oldest first) is the natural order
	// For path, ascending (A-Z) is the natural order
	// So: SortDescending = !reverse for size/age, SortDescending = reverse for path
	descending := !reverseVal
	if sortField == filter.SortPath {
		descending = reverseVal
	}
	opts = append(opts, filter.WithSortDescending(descending))

	return filter.New(opts...), nil
}

// parseColumns parses the columns flag into a slice of column names.
func parseColumns(columnsStr string) []string {
	if columnsStr == "" {
		return []string{}
	}
	return parseCommaSeparated(columnsStr)
}

// parseCommaSeparated splits a comma-separated string and trims whitespace.
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
