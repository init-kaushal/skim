// Package session tracks per-target read counts within a single session window.
// Because skim is invoked as a separate process for each hook call, state is
// persisted to disk and reloaded on every invocation. A "session" is defined
// as any period within 4 hours of the last recorded activity.
//
// Reuse tracking is used for observability and future cost-prediction models.
// A file read multiple times in a session is a candidate for proactive caching.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaushal/skim/internal/paths"
)

// sessionWindow is how long a session remains active without a new event.
const sessionWindow = 4 * time.Hour

// State holds per-target read counts for the current session.
type State struct {
	mu         sync.Mutex
	LastSeen   time.Time      `json:"last_seen"`
	ReadCounts map[string]int `json:"read_counts"` // target path → times seen
}

// Load reads the session state from disk. Returns a fresh empty state if the
// file does not exist, cannot be parsed, or the session window has expired.
// Load never returns an error — a missing or stale session is not a failure.
func Load() *State {
	b, err := os.ReadFile(stateFile())
	if err != nil {
		return newState()
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return newState()
	}
	if time.Since(s.LastSeen) > sessionWindow {
		return newState()
	}
	if s.ReadCounts == nil {
		s.ReadCounts = make(map[string]int)
	}
	return &s
}

func newState() *State {
	return &State{
		LastSeen:   time.Time{},
		ReadCounts: make(map[string]int),
	}
}

// GetCount returns how many times target has been seen in this session.
// Returns 0 for a target never seen, or if the session expired.
func (s *State) GetCount(target string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ReadCounts[target]
}

// RecordRead increments the count for target, marks the session as active, and
// saves asynchronously. Returns the new count (1 on first call for a target).
// Errors from disk I/O are silently dropped — reuse tracking must never block
// a hook call.
func (s *State) RecordRead(target string) int {
	s.mu.Lock()
	s.LastSeen = time.Now().UTC()
	s.ReadCounts[target]++
	count := s.ReadCounts[target]
	s.mu.Unlock()
	go func() { _ = s.save() }()
	return count
}

func (s *State) save() error {
	s.mu.Lock()
	b, err := json.MarshalIndent(s, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	p := stateFile()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

func stateFile() string {
	return filepath.Join(paths.Home(), "session.json")
}
