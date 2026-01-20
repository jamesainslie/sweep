package filter

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		// Days
		{name: "1 day", input: "1d", want: 24 * time.Hour, wantErr: false},
		{name: "30 days", input: "30d", want: 30 * 24 * time.Hour, wantErr: false},
		{name: "days uppercase", input: "7D", want: 7 * 24 * time.Hour, wantErr: false},

		// Weeks
		{name: "1 week", input: "1w", want: 7 * 24 * time.Hour, wantErr: false},
		{name: "2 weeks", input: "2w", want: 14 * 24 * time.Hour, wantErr: false},
		{name: "weeks uppercase", input: "4W", want: 28 * 24 * time.Hour, wantErr: false},

		// Months
		{name: "1 month", input: "1mo", want: 30 * 24 * time.Hour, wantErr: false},
		{name: "3 months", input: "3mo", want: 90 * 24 * time.Hour, wantErr: false},
		{name: "months uppercase", input: "6MO", want: 180 * 24 * time.Hour, wantErr: false},
		{name: "months mixed case", input: "2Mo", want: 60 * 24 * time.Hour, wantErr: false},

		// Years
		{name: "1 year", input: "1y", want: 365 * 24 * time.Hour, wantErr: false},
		{name: "2 years", input: "2y", want: 730 * 24 * time.Hour, wantErr: false},
		{name: "years uppercase", input: "5Y", want: 5 * 365 * 24 * time.Hour, wantErr: false},

		// Standard Go duration (fallback)
		{name: "hours", input: "24h", want: 24 * time.Hour, wantErr: false},
		{name: "minutes", input: "90m", want: 90 * time.Minute, wantErr: false},

		// Decimal values
		{name: "decimal days", input: "1.5d", want: 36 * time.Hour, wantErr: false},
		{name: "decimal weeks", input: "0.5w", want: 84 * time.Hour, wantErr: false},

		// Whitespace handling
		{name: "leading space", input: "  30d", want: 30 * 24 * time.Hour, wantErr: false},
		{name: "trailing space", input: "30d  ", want: 30 * 24 * time.Hour, wantErr: false},

		// Error cases
		{name: "empty", input: "", wantErr: true},
		{name: "invalid", input: "abc", wantErr: true},
		{name: "negative", input: "-1d", wantErr: true},
		{name: "no number", input: "d", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		// Plain bytes
		{name: "bytes", input: "1024", want: 1024, wantErr: false},
		{name: "zero", input: "0", want: 0, wantErr: false},

		// Kilobytes
		{name: "kilobytes K", input: "100K", want: 100 * 1024, wantErr: false},
		{name: "kilobytes KB", input: "100KB", want: 100 * 1024, wantErr: false},
		{name: "kilobytes lowercase", input: "100k", want: 100 * 1024, wantErr: false},
		{name: "kilobytes kb", input: "100kb", want: 100 * 1024, wantErr: false},

		// Megabytes
		{name: "megabytes M", input: "50M", want: 50 * 1024 * 1024, wantErr: false},
		{name: "megabytes MB", input: "50MB", want: 50 * 1024 * 1024, wantErr: false},
		{name: "megabytes lowercase", input: "50m", want: 50 * 1024 * 1024, wantErr: false},
		{name: "megabytes mb", input: "50mb", want: 50 * 1024 * 1024, wantErr: false},

		// Gigabytes
		{name: "gigabytes G", input: "2G", want: 2 * 1024 * 1024 * 1024, wantErr: false},
		{name: "gigabytes GB", input: "2GB", want: 2 * 1024 * 1024 * 1024, wantErr: false},
		{name: "gigabytes lowercase", input: "2g", want: 2 * 1024 * 1024 * 1024, wantErr: false},
		{name: "gigabytes gb", input: "2gb", want: 2 * 1024 * 1024 * 1024, wantErr: false},

		// Decimal values
		{name: "decimal megabytes", input: "1.5M", want: int64(1.5 * 1024 * 1024), wantErr: false},
		{name: "decimal gigabytes", input: "1.5G", want: int64(1.5 * 1024 * 1024 * 1024), wantErr: false},

		// Whitespace handling
		{name: "leading space", input: "  100M", want: 100 * 1024 * 1024, wantErr: false},
		{name: "trailing space", input: "100M  ", want: 100 * 1024 * 1024, wantErr: false},

		// Error cases
		{name: "empty", input: "", wantErr: true},
		{name: "invalid", input: "abc", wantErr: true},
		{name: "negative", input: "-100M", wantErr: true},
		{name: "no number", input: "M", wantErr: true},
		{name: "invalid suffix", input: "100X", wantErr: true},
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
