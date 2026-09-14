package detect

import "testing"

func TestMatchNoisy(t *testing.T) {
	pats := []string{`\bcat\s`, `go\s+test`, `(`} // last is uncompilable, must be skipped
	m, p := MatchNoisy("cat foo.txt", pats)
	if !m || p != `\bcat\s` {
		t.Fatalf("got (%v,%q)", m, p)
	}
	if m, _ := MatchNoisy("go test ./...", pats); !m {
		t.Fatal("go test should match")
	}
	if m, _ := MatchNoisy("echo hi", pats); m {
		t.Fatal("echo should not match")
	}
	if m, _ := MatchNoisy("concatenate", pats); m {
		t.Fatal(`\bcat\s should not match 'concatenate'`)
	}
}

func TestMatchPassthrough(t *testing.T) {
	globs := []string{"**/*.md", "**/go.mod", ".claude/**"}
	cases := map[string]bool{
		"/home/u/proj/README.md":       true,
		"/home/u/proj/docs/x/y.md":     true,
		"/home/u/proj/go.mod":          true,
		"/home/u/proj/.claude/x.json":  true,
		"/home/u/proj/main.go":         false,
		"/home/u/proj/notes.md.backup": false,
	}
	for path, want := range cases {
		if got := MatchPassthrough(path, globs); got != want {
			t.Errorf("MatchPassthrough(%q) = %v, want %v", path, got, want)
		}
	}
}
