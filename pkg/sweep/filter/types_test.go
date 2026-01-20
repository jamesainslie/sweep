package filter

import (
	"os"
	"testing"
	"time"
)

func TestSortField_String(t *testing.T) {
	tests := []struct {
		name  string
		field SortField
		want  string
	}{
		{name: "SortSize", field: SortSize, want: "size"},
		{name: "SortAge", field: SortAge, want: "age"},
		{name: "SortPath", field: SortPath, want: "path"},
		{name: "unknown", field: SortField(99), want: "size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.field.String()
			if got != tt.want {
				t.Errorf("SortField.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSortField(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    SortField
		wantErr bool
	}{
		{name: "size lowercase", input: "size", want: SortSize, wantErr: false},
		{name: "SIZE uppercase", input: "SIZE", want: SortSize, wantErr: false},
		{name: "Size mixed", input: "Size", want: SortSize, wantErr: false},
		{name: "age", input: "age", want: SortAge, wantErr: false},
		{name: "path", input: "path", want: SortPath, wantErr: false},
		{name: "invalid", input: "invalid", want: SortSize, wantErr: true},
		{name: "empty", input: "", want: SortSize, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSortField(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSortField(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseSortField(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTypeGroups(t *testing.T) {
	// Verify type groups exist and have expected extensions
	tests := []struct {
		group      string
		wantExt    string // An extension we expect to find
		wantExists bool
	}{
		{group: "video", wantExt: ".mp4", wantExists: true},
		{group: "video", wantExt: ".mkv", wantExists: true},
		{group: "audio", wantExt: ".mp3", wantExists: true},
		{group: "audio", wantExt: ".flac", wantExists: true},
		{group: "image", wantExt: ".jpg", wantExists: true},
		{group: "image", wantExt: ".png", wantExists: true},
		{group: "archive", wantExt: ".zip", wantExists: true},
		{group: "archive", wantExt: ".tar", wantExists: true},
		{group: "document", wantExt: ".pdf", wantExists: true},
		{group: "document", wantExt: ".docx", wantExists: true},
		{group: "code", wantExt: ".go", wantExists: true},
		{group: "code", wantExt: ".py", wantExists: true},
		{group: "log", wantExt: ".log", wantExists: true},
		{group: "nonexistent", wantExt: ".xyz", wantExists: false},
	}

	for _, tt := range tests {
		t.Run(tt.group+"_"+tt.wantExt, func(t *testing.T) {
			exts, exists := TypeGroups[tt.group]
			if !tt.wantExists {
				if exists {
					t.Errorf("TypeGroups[%q] should not exist", tt.group)
				}
				return
			}
			if !exists {
				t.Errorf("TypeGroups[%q] should exist", tt.group)
				return
			}
			found := false
			for _, ext := range exts {
				if ext == tt.wantExt {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("TypeGroups[%q] should contain %q", tt.group, tt.wantExt)
			}
		})
	}
}

func TestFileInfo(t *testing.T) {
	now := time.Now()
	fi := FileInfo{
		Path:    "/home/user/file.txt",
		Name:    "file.txt",
		Dir:     "/home/user",
		Ext:     ".txt",
		Size:    1024,
		ModTime: now,
		Mode:    os.FileMode(0644),
		Owner:   "user",
		Depth:   2,
	}

	// Verify all fields are set correctly
	if fi.Path != "/home/user/file.txt" {
		t.Errorf("Path = %q, want %q", fi.Path, "/home/user/file.txt")
	}
	if fi.Name != "file.txt" {
		t.Errorf("Name = %q, want %q", fi.Name, "file.txt")
	}
	if fi.Dir != "/home/user" {
		t.Errorf("Dir = %q, want %q", fi.Dir, "/home/user")
	}
	if fi.Ext != ".txt" {
		t.Errorf("Ext = %q, want %q", fi.Ext, ".txt")
	}
	if fi.Size != 1024 {
		t.Errorf("Size = %d, want %d", fi.Size, 1024)
	}
	if !fi.ModTime.Equal(now) {
		t.Errorf("ModTime = %v, want %v", fi.ModTime, now)
	}
	if fi.Mode != os.FileMode(0644) {
		t.Errorf("Mode = %v, want %v", fi.Mode, os.FileMode(0644))
	}
	if fi.Owner != "user" {
		t.Errorf("Owner = %q, want %q", fi.Owner, "user")
	}
	if fi.Depth != 2 {
		t.Errorf("Depth = %d, want %d", fi.Depth, 2)
	}
}
