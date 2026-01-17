package types

import (
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		// Basic byte values
		{name: "plain bytes", input: "1024", want: 1024, wantErr: false},
		{name: "zero bytes", input: "0", want: 0, wantErr: false},
		{name: "bytes with B suffix", input: "512B", want: 512, wantErr: false},
		{name: "bytes with lowercase b", input: "512b", want: 512, wantErr: false},

		// Kilobytes
		{name: "kilobytes uppercase", input: "100K", want: 100 * 1024, wantErr: false},
		{name: "kilobytes lowercase", input: "100k", want: 100 * 1024, wantErr: false},
		{name: "kilobytes with B", input: "100KB", want: 100 * 1024, wantErr: false},
		{name: "kilobytes with iB", input: "100KiB", want: 100 * 1024, wantErr: false},

		// Megabytes
		{name: "megabytes uppercase", input: "50M", want: 50 * 1024 * 1024, wantErr: false},
		{name: "megabytes lowercase", input: "50m", want: 50 * 1024 * 1024, wantErr: false},
		{name: "megabytes with B", input: "50MB", want: 50 * 1024 * 1024, wantErr: false},
		{name: "megabytes with iB", input: "50MiB", want: 50 * 1024 * 1024, wantErr: false},

		// Gigabytes
		{name: "gigabytes uppercase", input: "2G", want: 2 * 1024 * 1024 * 1024, wantErr: false},
		{name: "gigabytes lowercase", input: "2g", want: 2 * 1024 * 1024 * 1024, wantErr: false},
		{name: "gigabytes with B", input: "2GB", want: 2 * 1024 * 1024 * 1024, wantErr: false},
		{name: "gigabytes with iB", input: "2GiB", want: 2 * 1024 * 1024 * 1024, wantErr: false},

		// Terabytes
		{name: "terabytes uppercase", input: "1T", want: 1024 * 1024 * 1024 * 1024, wantErr: false},
		{name: "terabytes lowercase", input: "1t", want: 1024 * 1024 * 1024 * 1024, wantErr: false},
		{name: "terabytes with B", input: "1TB", want: 1024 * 1024 * 1024 * 1024, wantErr: false},
		{name: "terabytes with iB", input: "1TiB", want: 1024 * 1024 * 1024 * 1024, wantErr: false},

		// Whitespace handling
		{name: "leading whitespace", input: "  100M", want: 100 * 1024 * 1024, wantErr: false},
		{name: "trailing whitespace", input: "100M  ", want: 100 * 1024 * 1024, wantErr: false},
		{name: "both whitespace", input: "  100M  ", want: 100 * 1024 * 1024, wantErr: false},

		// Edge cases
		{name: "large value", input: "10T", want: 10 * 1024 * 1024 * 1024 * 1024, wantErr: false},
		{name: "decimal values truncated", input: "1.5G", want: 1610612736, wantErr: false},

		// Error cases
		{name: "empty string", input: "", wantErr: true},
		{name: "only whitespace", input: "   ", wantErr: true},
		{name: "invalid suffix", input: "100X", wantErr: true},
		{name: "negative value", input: "-100M", wantErr: true},
		{name: "letters only", input: "abc", wantErr: true},
		{name: "suffix only", input: "M", wantErr: true},
		{name: "invalid format", input: "100M100", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero", bytes: 0, want: "0 B"},
		{name: "bytes", bytes: 500, want: "500 B"},
		{name: "kilobytes", bytes: 1024, want: "1.0 KiB"},
		{name: "megabytes", bytes: 1024 * 1024, want: "1.0 MiB"},
		{name: "gigabytes", bytes: 1024 * 1024 * 1024, want: "1.0 GiB"},
		{name: "terabytes", bytes: 1024 * 1024 * 1024 * 1024, want: "1.0 TiB"},
		{name: "mixed size", bytes: 1536 * 1024, want: "1.5 MiB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFileInfo_HumanSize(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want string
	}{
		{name: "zero", size: 0, want: "0 B"},
		{name: "small file", size: 512, want: "512 B"},
		{name: "kilobyte file", size: 2048, want: "2.0 KiB"},
		{name: "megabyte file", size: 5 * 1024 * 1024, want: "5.0 MiB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &FileInfo{Size: tt.size}
			got := f.HumanSize()
			if got != tt.want {
				t.Errorf("FileInfo.HumanSize() = %q, want %q", got, tt.want)
			}
		})
	}
}
