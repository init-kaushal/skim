package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DemoFileName is what Demo writes. Deliberately obvious, and obviously
// disposable.
const DemoFileName = "skim-demo-inventory.go"

// Demo writes a file large enough to trip interception into dir, then prints
// what to do next.
//
// It writes into the user's working directory rather than shipping a sample
// inside the plugin, and that is not a stylistic choice: an installed plugin
// lives under ~/.claude/plugins/, which matches the default passthrough glob
// `.claude/**`. A bundled sample would be passed through untouched and the demo
// would silently demonstrate nothing — the most misleading possible outcome for
// the one command whose job is to show that skim works.
func Demo(w io.Writer, dir string) error {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("skim demo: %w", err)
		}
		dir = cwd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("skim demo: %w", err)
	}
	if strings.Contains(filepath.ToSlash(abs), "/.claude/") {
		return fmt.Errorf("skim demo: %s is under .claude/, which skim passes through by "+
			"default (passthrough_globs) — the demo would not be intercepted. "+
			"Run it from a normal project directory", abs)
	}

	path := filepath.Join(abs, DemoFileName)
	body := demoFile()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("skim demo: write %s: %w", path, err)
	}

	lines := strings.Count(body, "\n")
	fmt.Fprintf(w, "skim demo: wrote %s\n", path)
	fmt.Fprintf(w, "           %d lines, %d bytes — over the default 300-line threshold.\n\n", lines, len(body))
	fmt.Fprintf(w, "Now read it. With skim active you will get a ~15-line structural digest\n")
	fmt.Fprintf(w, "instead of %d lines of source, and you can still ask questions about it:\n\n", lines)
	fmt.Fprintf(w, "    Read %s\n\n", DemoFileName)
	fmt.Fprintf(w, "Then compare:\n")
	fmt.Fprintf(w, "    /skim stats                 what it saved, and what the worker cost\n")
	fmt.Fprintf(w, "    /skim off                   turn interception off\n")
	fmt.Fprintf(w, "    Read %s   (again — now you get the full file)\n", DemoFileName)
	fmt.Fprintf(w, "    /skim on\n\n")
	fmt.Fprintf(w, "Delete the demo file when you are done: rm %s\n", DemoFileName)
	return nil
}

// demoFile builds the sample. It is generated rather than embedded so the
// repository does not carry 30KB of filler, and it is deliberately given
// several *distinct* sections — types, validation, HTTP handlers, storage,
// helpers — because a digest of one repeated construct shows nothing. The point
// is that the line-range map comes back describing real structure.
func demoFile() string {
	var b strings.Builder

	b.WriteString(`// Package inventory is a generated sample used by ` + "`skim demo`" + `.
//
// It exists only to be large enough that skim intercepts a Read of it. Nothing
// here is meant to be run, and the file is safe to delete.
package inventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

`)

	kinds := []string{"Widget", "Gadget", "Sprocket", "Flange", "Bracket", "Coupling"}
	for _, k := range kinds {
		fmt.Fprintf(&b, `// %[1]s is a catalogue item.
type %[1]s struct {
	ID        int       `+"`json:\"id\"`"+`
	Name      string    `+"`json:\"name\"`"+`
	Tags      []string  `+"`json:\"tags\"`"+`
	Quantity  int       `+"`json:\"quantity\"`"+`
	UpdatedAt time.Time `+"`json:\"updated_at\"`"+`
}

// Key returns the storage key for a %[1]s.
func (x *%[1]s) Key() string { return fmt.Sprintf("%[2]s/%%d", x.ID) }

`, k, strings.ToLower(k))
	}

	b.WriteString(`// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

var (
	ErrNoID        = errors.New("inventory: id must be positive")
	ErrNoName      = errors.New("inventory: name must not be empty")
	ErrNegativeQty = errors.New("inventory: quantity must not be negative")
)

`)
	for _, k := range kinds {
		fmt.Fprintf(&b, `// Validate checks %[1]s invariants.
func (x *%[1]s) Validate() error {
	if x.ID <= 0 {
		return ErrNoID
	}
	if strings.TrimSpace(x.Name) == "" {
		return ErrNoName
	}
	if x.Quantity < 0 {
		return ErrNegativeQty
	}
	for _, t := range x.Tags {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("inventory: %%s has an empty tag", x.Key())
		}
	}
	return nil
}

`, k)
	}

	b.WriteString(`// ---------------------------------------------------------------------------
// In-memory storage
// ---------------------------------------------------------------------------

// Store is a concurrency-safe catalogue.
type Store struct {
	mu    sync.RWMutex
	items map[string]any
}

// NewStore returns an empty Store.
func NewStore() *Store { return &Store{items: map[string]any{}} }

`)
	for _, k := range kinds {
		fmt.Fprintf(&b, `// Put%[1]s stores a %[1]s after validating it.
func (s *Store) Put%[1]s(x *%[1]s) error {
	if err := x.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	x.UpdatedAt = time.Now().UTC()
	s.items[x.Key()] = x
	return nil
}

// Get%[1]s retrieves a %[1]s by id.
func (s *Store) Get%[1]s(id int) (*%[1]s, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[fmt.Sprintf("%[2]s/%%d", id)]
	if !ok {
		return nil, false
	}
	x, ok := v.(*%[1]s)
	return x, ok
}

// Delete%[1]s removes a %[1]s by id.
func (s *Store) Delete%[1]s(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%[2]s/%%d", id)
	_, ok := s.items[key]
	delete(s.items, key)
	return ok
}

`, k, strings.ToLower(k))
	}

	b.WriteString(`// Keys returns every stored key, sorted.
func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.items))
	for k := range s.items {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

`)
	for _, k := range kinds {
		fmt.Fprintf(&b, `// Handle%[1]s serves read and write requests for %[1]s items.
func (s *Store) Handle%[1]s(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var x %[1]s
		if err := json.NewDecoder(r.Body).Decode(&x); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		got, ok := s.Get%[1]s(x.ID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, got)
	case http.MethodPut:
		var x %[1]s
		if err := json.NewDecoder(r.Body).Decode(&x); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Put%[1]s(&x); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeJSON(w, map[string]string{"status": "stored"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

`, k)
	}

	b.WriteString(`// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeJSON encodes v as the response body.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// normaliseTags trims, lowercases and de-duplicates tags.
func normaliseTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// summarise renders a one-line description of the catalogue.
func (s *Store) summarise() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byKind := map[string]int{}
	for k := range s.items {
		byKind[strings.SplitN(k, "/", 2)[0]]++
	}
	parts := make([]string, 0, len(byKind))
	for kind, n := range byKind {
		parts = append(parts, fmt.Sprintf("%s=%d", kind, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}
`)
	return b.String()
}
