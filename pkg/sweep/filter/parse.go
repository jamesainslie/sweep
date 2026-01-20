package filter

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Size constants for binary (IEC) units.
const (
	KiB int64 = 1024
	MiB int64 = 1024 * KiB
	GiB int64 = 1024 * MiB
)

// Duration constants.
const (
	Day   = 24 * time.Hour
	Week  = 7 * Day
	Month = 30 * Day  // Approximate
	Year  = 365 * Day // Approximate
)

// ErrInvalidDuration indicates that the duration string could not be parsed.
var ErrInvalidDuration = errors.New("invalid duration format")

// ErrInvalidSize indicates that the size string could not be parsed.
var ErrInvalidSize = errors.New("invalid size format")

// ErrNegativeValue indicates that a negative value was provided.
var ErrNegativeValue = errors.New("value cannot be negative")

// durationPattern matches duration strings like "30d", "2w", "1mo", "1y".
var durationPattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*(d|w|mo|y|h|m|s|ms|us|ns)\s*$`)

// sizePattern matches size strings like "100M", "2G", "500K", "1.5GB".
var sizePattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*([KMGT]?B?)\s*$`)

// ParseDuration parses a human-readable duration string.
// It supports the following formats:
//   - Days: "1d", "30d"
//   - Weeks: "1w", "2w"
//   - Months: "1mo", "3mo" (30 days per month)
//   - Years: "1y", "2y" (365 days per year)
//   - Standard Go duration: "24h", "90m", "1h30m"
//
// Returns ErrInvalidDuration if the format is not recognized.
// Returns ErrNegativeValue if the value is negative.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty string", ErrInvalidDuration)
	}

	// Check for negative values
	if strings.HasPrefix(s, "-") {
		return 0, ErrNegativeValue
	}

	matches := durationPattern.FindStringSubmatch(s)
	if matches == nil {
		// Try standard Go duration as fallback
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("%w: %q", ErrInvalidDuration, s)
		}
		return d, nil
	}

	// Parse the numeric value
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidDuration, s)
	}

	// Determine the multiplier based on the suffix
	suffix := strings.ToLower(matches[2])
	var multiplier time.Duration
	switch suffix {
	case "d":
		multiplier = Day
	case "w":
		multiplier = Week
	case "mo":
		multiplier = Month
	case "y":
		multiplier = Year
	case "h":
		multiplier = time.Hour
	case "m":
		multiplier = time.Minute
	case "s":
		multiplier = time.Second
	case "ms":
		multiplier = time.Millisecond
	case "us":
		multiplier = time.Microsecond
	case "ns":
		multiplier = time.Nanosecond
	default:
		return 0, fmt.Errorf("%w: unknown suffix %q", ErrInvalidDuration, suffix)
	}

	return time.Duration(value * float64(multiplier)), nil
}

// ParseSize parses a human-readable size string and returns the size in bytes.
// It supports the following formats:
//   - Plain bytes: "1024", "0"
//   - Kilobytes: "100K", "100KB", "100k", "100kb"
//   - Megabytes: "50M", "50MB", "50m", "50mb"
//   - Gigabytes: "2G", "2GB", "2g", "2gb"
//   - Terabytes: "1T", "1TB", "1t", "1tb"
//
// Decimal values are supported and truncated to the nearest byte.
// Leading and trailing whitespace is ignored.
//
// Returns ErrInvalidSize if the format is not recognized.
// Returns ErrNegativeValue if the value is negative.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty string", ErrInvalidSize)
	}

	// Check for negative values
	if strings.HasPrefix(s, "-") {
		return 0, ErrNegativeValue
	}

	matches := sizePattern.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidSize, s)
	}

	// Parse the numeric value
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidSize, s)
	}

	// Determine the multiplier based on the suffix
	suffix := strings.ToUpper(matches[2])
	// Remove 'B' suffix to get just the unit letter
	suffix = strings.TrimSuffix(suffix, "B")

	var multiplier int64
	switch suffix {
	case "":
		multiplier = 1
	case "K":
		multiplier = KiB
	case "M":
		multiplier = MiB
	case "G":
		multiplier = GiB
	case "T":
		multiplier = GiB * 1024
	default:
		return 0, fmt.Errorf("%w: unknown suffix %q", ErrInvalidSize, suffix)
	}

	return int64(value * float64(multiplier)), nil
}
