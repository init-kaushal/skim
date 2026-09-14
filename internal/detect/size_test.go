package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kaushal/skim/internal/config"
)

func TestFileSize(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name          string
		content       string
		wantLines     int
		wantBytesFunc func(string) int
	}{
		{"empty", "", 0, func(s string) int { return 0 }},
		{"one line no newline", "abc", 1, func(s string) int { return len(s) }},
		{"one line newline", "abc\n", 1, func(s string) int { return len(s) }},
		{"three lines", "a\nb\nc\n", 3, func(s string) int { return len(s) }},
		{"trailing partial", "a\nb\nc", 3, func(s string) int { return len(s) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name)
			os.WriteFile(p, []byte(tc.content), 0o644)
			lines, bytes, err := FileSize(p)
			if err != nil {
				t.Fatal(err)
			}
			if lines != tc.wantLines {
				t.Errorf("lines = %d, want %d", lines, tc.wantLines)
			}
			if bytes != tc.wantBytesFunc(tc.content) {
				t.Errorf("bytes = %d, want %d", bytes, tc.wantBytesFunc(tc.content))
			}
		})
	}
}

// TestMeasure_PrefixBytes covers the offset the Read hook caps the worker's
// input at. It must land on a line boundary: cutting at a raw byte offset fed
// the worker a half-token final line (observed as `func f13929() { retur`).
func TestMeasure_PrefixBytes(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name        string
		content     string
		prefixLines int
		wantLines   int
		wantPrefix  int
	}{
		// Prefix lands exactly after the 2nd newline, so "a\nb\n" = 4 bytes.
		{"cut mid file", "a\nb\nc\nd\n", 2, 4, 4},
		// Fewer lines than the cap: the prefix is the whole file.
		{"shorter than cap", "a\nb\n", 10, 2, 4},
		// Exactly at the cap: still the whole file.
		{"exactly at cap", "a\nb\n", 2, 2, 4},
		// Final line without a newline counts as a line but has no newline to
		// cut after, so the prefix is everything.
		{"no trailing newline", "a\nb\nc", 10, 3, 5},
		// A cap reached before an unterminated tail cuts after the newline.
		{"cap before ragged tail", "aa\nbb\ncc", 2, 3, 6},
		{"empty", "", 5, 0, 0},
		// prefixLines <= 0 disables the prefix bound entirely.
		{"no prefix requested", "a\nb\nc\n", 0, 3, 6},
		// Multi-byte content: the offset is in bytes, not runes.
		{"utf8", "héllo\nwörld\nx\n", 2, 3, len("héllo\nwörld\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name)
			if err := os.WriteFile(p, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			s, err := Measure(p, tc.prefixLines)
			if err != nil {
				t.Fatal(err)
			}
			if s.Lines != tc.wantLines {
				t.Errorf("Lines = %d, want %d", s.Lines, tc.wantLines)
			}
			if s.Bytes != len(tc.content) {
				t.Errorf("Bytes = %d, want %d", s.Bytes, len(tc.content))
			}
			if s.PrefixBytes != tc.wantPrefix {
				t.Errorf("PrefixBytes = %d, want %d", s.PrefixBytes, tc.wantPrefix)
			}
			if s.PrefixBytes > s.Bytes {
				t.Errorf("PrefixBytes %d exceeds Bytes %d", s.PrefixBytes, s.Bytes)
			}
			// The prefix must end on a line boundary (or at EOF), so that the
			// worker never receives a partial line.
			if s.PrefixBytes > 0 && s.PrefixBytes < s.Bytes &&
				tc.content[s.PrefixBytes-1] != '\n' {
				t.Errorf("PrefixBytes %d does not end on a newline", s.PrefixBytes)
			}
		})
	}
}

// TestMeasure_PrefixSpansReadBuffer guards the boundary arithmetic across the
// 32KB internal read buffer: the prefix offset is accumulated per chunk, so an
// off-by-one there would only show up on a file bigger than one buffer.
func TestMeasure_PrefixSpansReadBuffer(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big")
	// 5000 lines of 20 bytes each = 100KB, well past the 32KB buffer.
	line := "0123456789012345678\n" // 20 bytes incl. newline
	content := ""
	for i := 0; i < 5000; i++ {
		content += line
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Measure(p, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if s.Lines != 5000 {
		t.Errorf("Lines = %d, want 5000", s.Lines)
	}
	if want := 2000 * len(line); s.PrefixBytes != want {
		t.Errorf("PrefixBytes = %d, want %d", s.PrefixBytes, want)
	}
	if content[s.PrefixBytes-1] != '\n' {
		t.Error("prefix does not end on a newline")
	}
}

func TestExceedsThreshold(t *testing.T) {
	c := config.Default() // 300 lines / 60000 bytes
	if ExceedsThreshold(300, 100, c) {
		t.Error("exactly at line limit should not exceed")
	}
	if !ExceedsThreshold(301, 100, c) {
		t.Error("over line limit should exceed")
	}
	if !ExceedsThreshold(10, 60001, c) {
		t.Error("over byte limit should exceed")
	}
}
