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
