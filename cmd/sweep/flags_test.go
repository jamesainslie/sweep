package main

import (
	"testing"
	"time"

	"github.com/jamesainslie/sweep/pkg/sweep/filter"
	"github.com/spf13/viper"
)

func TestBuildFilter(t *testing.T) {
	// Reset viper for each test
	resetViperForTest := func() {
		viper.Reset()
		// Set defaults
		viper.SetDefault("limit", 50)
		viper.SetDefault("sort", "size")
		viper.SetDefault("reverse", false)
	}

	tests := []struct {
		name           string
		setup          func()
		wantLimit      int
		wantMinSize    int64
		wantSortBy     filter.SortField
		wantDescending bool // SortDescending field value
		wantErr        bool
	}{
		{
			name: "default values",
			setup: func() {
				resetViperForTest()
			},
			wantLimit:      50,
			wantMinSize:    0,
			wantSortBy:     filter.SortSize,
			wantDescending: true, // size: largest first by default
			wantErr:        false,
		},
		{
			name: "custom limit",
			setup: func() {
				resetViperForTest()
				viper.Set("limit", 100)
			},
			wantLimit:      100,
			wantSortBy:     filter.SortSize,
			wantDescending: true, // size: largest first by default
			wantErr:        false,
		},
		{
			name: "sort by age",
			setup: func() {
				resetViperForTest()
				viper.Set("sort", "age")
			},
			wantLimit:      50,
			wantSortBy:     filter.SortAge,
			wantDescending: true, // age: oldest first by default
			wantErr:        false,
		},
		{
			name: "sort by path",
			setup: func() {
				resetViperForTest()
				viper.Set("sort", "path")
			},
			wantLimit:      50,
			wantSortBy:     filter.SortPath,
			wantDescending: false, // path: A-Z by default
			wantErr:        false,
		},
		{
			name: "reverse sort on size",
			setup: func() {
				resetViperForTest()
				viper.Set("reverse", true)
			},
			wantLimit:      50,
			wantSortBy:     filter.SortSize,
			wantDescending: false, // reversed: smallest first
			wantErr:        false,
		},
		{
			name: "reverse sort on path",
			setup: func() {
				resetViperForTest()
				viper.Set("sort", "path")
				viper.Set("reverse", true)
			},
			wantLimit:      50,
			wantSortBy:     filter.SortPath,
			wantDescending: true, // reversed: Z-A
			wantErr:        false,
		},
		{
			name: "invalid sort field",
			setup: func() {
				resetViperForTest()
				viper.Set("sort", "invalid")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			f, err := buildFilter()
			if (err != nil) != tt.wantErr {
				t.Errorf("buildFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if f.Limit != tt.wantLimit {
				t.Errorf("buildFilter() Limit = %d, want %d", f.Limit, tt.wantLimit)
			}
			if f.SortBy != tt.wantSortBy {
				t.Errorf("buildFilter() SortBy = %v, want %v", f.SortBy, tt.wantSortBy)
			}
			if f.SortDescending != tt.wantDescending {
				t.Errorf("buildFilter() SortDescending = %v, want %v", f.SortDescending, tt.wantDescending)
			}
		})
	}
}

func TestBuildFilterWithDurations(t *testing.T) {
	resetViperForTest := func() {
		viper.Reset()
		viper.SetDefault("limit", 50)
		viper.SetDefault("sort", "size")
		viper.SetDefault("reverse", false)
	}

	tests := []struct {
		name          string
		olderThan     string
		newerThan     string
		wantOlderThan time.Duration
		wantNewerThan time.Duration
		wantErr       bool
	}{
		{
			name:          "older than 30 days",
			olderThan:     "30d",
			wantOlderThan: 30 * 24 * time.Hour,
			wantErr:       false,
		},
		{
			name:          "newer than 1 week",
			newerThan:     "1w",
			wantNewerThan: 7 * 24 * time.Hour,
			wantErr:       false,
		},
		{
			name:      "invalid older than",
			olderThan: "invalid",
			wantErr:   true,
		},
		{
			name:      "invalid newer than",
			newerThan: "invalid",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViperForTest()
			if tt.olderThan != "" {
				viper.Set("older_than", tt.olderThan)
			}
			if tt.newerThan != "" {
				viper.Set("newer_than", tt.newerThan)
			}

			f, err := buildFilter()
			if (err != nil) != tt.wantErr {
				t.Errorf("buildFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if f.OlderThan != tt.wantOlderThan {
				t.Errorf("buildFilter() OlderThan = %v, want %v", f.OlderThan, tt.wantOlderThan)
			}
			if f.NewerThan != tt.wantNewerThan {
				t.Errorf("buildFilter() NewerThan = %v, want %v", f.NewerThan, tt.wantNewerThan)
			}
		})
	}
}

func TestBuildFilterWithTypeGroups(t *testing.T) {
	resetViperForTest := func() {
		viper.Reset()
		viper.SetDefault("limit", 50)
		viper.SetDefault("sort", "size")
		viper.SetDefault("reverse", false)
	}

	tests := []struct {
		name           string
		fileTypes      string
		extensions     string
		wantExtensions []string
	}{
		{
			name:           "video type group",
			fileTypes:      "video",
			wantExtensions: filter.TypeGroups["video"],
		},
		{
			name:           "multiple type groups",
			fileTypes:      "video,audio",
			wantExtensions: append(filter.TypeGroups["video"], filter.TypeGroups["audio"]...),
		},
		{
			name:           "custom extensions",
			extensions:     ".mp4,.mkv",
			wantExtensions: []string{".mp4", ".mkv"},
		},
		{
			name:           "extensions without dots",
			extensions:     "mp4,mkv",
			wantExtensions: []string{".mp4", ".mkv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetViperForTest()
			if tt.fileTypes != "" {
				viper.Set("type", tt.fileTypes)
			}
			if tt.extensions != "" {
				viper.Set("ext", tt.extensions)
			}

			f, err := buildFilter()
			if err != nil {
				t.Fatalf("buildFilter() error = %v", err)
			}

			if len(f.Extensions) != len(tt.wantExtensions) {
				t.Errorf("buildFilter() Extensions count = %d, want %d", len(f.Extensions), len(tt.wantExtensions))
			}
		})
	}
}

func TestParseColumns(t *testing.T) {
	tests := []struct {
		name    string
		columns string
		want    []string
	}{
		{
			name:    "default columns",
			columns: "size,path",
			want:    []string{"size", "path"},
		},
		{
			name:    "multiple columns",
			columns: "size,path,age,ext",
			want:    []string{"size", "path", "age", "ext"},
		},
		{
			name:    "single column",
			columns: "path",
			want:    []string{"path"},
		},
		{
			name:    "with spaces",
			columns: "size, path, age",
			want:    []string{"size", "path", "age"},
		},
		{
			name:    "empty",
			columns: "",
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseColumns(tt.columns)
			if len(got) != len(tt.want) {
				t.Errorf("parseColumns() = %v, want %v", got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseColumns()[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}
