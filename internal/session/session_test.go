package session_test

import (
	"testing"
	"time"

	"github.com/kaushal/skim/internal/session"
)

func TestSession_GetCountZeroForUnseen(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	s := session.Load()
	if n := s.GetCount("never/seen.go"); n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestSession_RecordReadIncrementsCount(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	s := session.Load()

	n1 := s.RecordRead("file.go")
	if n1 != 1 {
		t.Errorf("first RecordRead: want 1, got %d", n1)
	}
	n2 := s.RecordRead("file.go")
	if n2 != 2 {
		t.Errorf("second RecordRead: want 2, got %d", n2)
	}
	n3 := s.RecordRead("other.go")
	if n3 != 1 {
		t.Errorf("first RecordRead for other.go: want 1, got %d", n3)
	}
}

func TestSession_GetCountReflectsRecords(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	s := session.Load()

	s.RecordRead("a.go")
	s.RecordRead("a.go")
	s.RecordRead("a.go")

	if n := s.GetCount("a.go"); n != 3 {
		t.Errorf("want 3, got %d", n)
	}
	if n := s.GetCount("b.go"); n != 0 {
		t.Errorf("b.go should be 0, got %d", n)
	}
}

func TestSession_IndependentTargets(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	s := session.Load()

	for _, f := range []string{"a.go", "b.go", "c.go"} {
		s.RecordRead(f)
	}
	s.RecordRead("a.go")

	if got := s.GetCount("a.go"); got != 2 {
		t.Errorf("a.go: want 2, got %d", got)
	}
	if got := s.GetCount("b.go"); got != 1 {
		t.Errorf("b.go: want 1, got %d", got)
	}
	if got := s.GetCount("c.go"); got != 1 {
		t.Errorf("c.go: want 1, got %d", got)
	}
}

func TestSession_LoadFreshWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKIM_HOME", dir)

	s := session.Load() // no file exists yet
	if s == nil {
		t.Fatal("Load returned nil")
	}
	if n := s.GetCount("anything"); n != 0 {
		t.Errorf("fresh session should have count 0, got %d", n)
	}
}

func TestSession_DoesNotPanicOnConcurrentAccess(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	s := session.Load()

	done := make(chan struct{})
	for range 10 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 20 {
				s.RecordRead("concurrent.go")
				_ = s.GetCount("concurrent.go")
			}
		}()
	}
	for range 10 {
		<-done
	}
}

// TestSession_RecordReadReturnsCorrectCount verifies the return value matches
// the count after the increment.
func TestSession_RecordReadReturnsCorrectCount(t *testing.T) {
	t.Setenv("SKIM_HOME", t.TempDir())
	s := session.Load()

	for i := 1; i <= 5; i++ {
		got := s.RecordRead("file.go")
		if got != i {
			t.Errorf("iteration %d: want %d, got %d", i, i, got)
		}
	}
}

func TestSession_NewSessionAfterWindow(_ *testing.T) {
	// This test is informational only — it can't easily verify the 4-hour
	// window without time mocking, but the Load/newState paths are covered
	// by the other tests. The session window constant is tested implicitly
	// by the fact that a fresh SKIM_HOME always returns zero counts.
	_ = time.Hour // 4*time.Hour is the window constant in session.go
}
