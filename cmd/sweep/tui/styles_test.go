package tui

import (
	"strings"
	"testing"
)

func TestRepeatChar(t *testing.T) {
	tests := []struct {
		char     rune
		n        int
		expected string
	}{
		{'a', 0, ""},
		{'a', -1, ""},
		{'a', 1, "a"},
		{'a', 5, "aaaaa"},
		{'─', 3, "───"},
		{' ', 4, "    "},
	}

	for _, tt := range tests {
		result := repeatChar(tt.char, tt.n)
		if result != tt.expected {
			t.Errorf("repeatChar(%q, %d) = %q, want %q", tt.char, tt.n, result, tt.expected)
		}
	}
}

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		path     string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exact_len", 9, "exact_len"},
		{"/very/long/path/to/file.txt", 20, ".../path/to/file.txt"},
		{"/very/long/path/to/file.txt", 10, "...ile.txt"},
		{"/a/b", 10, "/a/b"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
		{"abcdef", 4, "...f"},
	}

	for _, tt := range tests {
		result := truncatePath(tt.path, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncatePath(%q, %d) = %q, want %q", tt.path, tt.maxLen, result, tt.expected)
		}
		if len(result) > tt.maxLen {
			t.Errorf("truncatePath(%q, %d) result length %d exceeds maxLen", tt.path, tt.maxLen, len(result))
		}
	}
}

func TestPadLeft(t *testing.T) {
	tests := []struct {
		s        string
		width    int
		expected string
	}{
		{"abc", 5, "  abc"},
		{"abc", 3, "abc"},
		{"abc", 2, "abc"},
		{"", 3, "   "},
		{"hello", 10, "     hello"},
	}

	for _, tt := range tests {
		result := padLeft(tt.s, tt.width)
		if result != tt.expected {
			t.Errorf("padLeft(%q, %d) = %q, want %q", tt.s, tt.width, result, tt.expected)
		}
	}
}

func TestCenter(t *testing.T) {
	tests := []struct {
		s        string
		width    int
		expected string
	}{
		{"abc", 7, "  abc  "},
		{"abc", 6, " abc  "},
		{"abc", 3, "abc"},
		{"abc", 2, "abc"},
		{"", 4, "    "},
		{"x", 5, "  x  "},
	}

	for _, tt := range tests {
		result := center(tt.s, tt.width)
		if result != tt.expected {
			t.Errorf("center(%q, %d) = %q, want %q", tt.s, tt.width, result, tt.expected)
		}
	}
}

func TestRenderDivider(t *testing.T) {
	tests := []struct {
		width int
	}{
		{10},
		{20},
		{80},
	}

	for _, tt := range tests {
		result := renderDivider(tt.width)
		// The divider should contain the horizontal line character
		if !strings.Contains(result, "─") {
			t.Errorf("renderDivider(%d) should contain '─' character", tt.width)
		}
	}
}
