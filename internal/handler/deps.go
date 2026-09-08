package handler

import (
	"context"
	"io"
	"time"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
	"github.com/kaushal/skim/internal/worker"
)

// Deps holds the collaborators ReadHook (and future hooks) need. Every field is
// injectable so tests can drive each branch without real I/O. The zero value is
// not usable; callers wire the "usually" implementations noted per field.
type Deps struct {
	Cfg       config.Config
	Env       func(string) string                                           // usually os.Getenv
	Summarize func(ctx context.Context, req worker.Request) ([]byte, error) // usually worker.Run
	CacheKey  func(path, model string) (string, error)                      // usually cache.Key
	CacheGet  func(key string) (digest.FileMap, bool)                       // usually cache.Get
	CachePut  func(key string, fm digest.FileMap) error                     // usually cache.Put
	Record    func(metrics.Entry)                                           // usually func(e){ _ = metrics.Record(e) }
	Logf      func(format string, args ...any)                              // appends to skim.log
	Now       func() time.Time
	Stdout    io.Writer

	// CountMatches reports how many Grep hits gi would produce and returns a
	// capped (~32 KB) rg context dump as sample, used as the worker input.
	// Usually backed by rg; an error (e.g. rg not installed) degrades open.
	CountMatches func(gi hookio.GrepInput) (total int, sample string, err error)
}
