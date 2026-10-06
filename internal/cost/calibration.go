package cost

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaushal/skim/internal/paths"
)

// DefaultOutputRatio is the conservative initial estimate for output_tokens /
// input_tokens from a worker call. 0.30 means the digest is ~30% of the input
// token count. This is deliberately conservative (real results are usually
// better) so we under-predict savings rather than over-predict them.
//
// Why 0.30: observed skim FileMap digests for ~5–10 KB Go source files run
// ~300–600 tokens from ~1500–2500 input tokens (18–25%). A 30% ceiling is
// safe enough to use before calibration data accumulates.
const DefaultOutputRatio = 0.30

// minSamplesForCalibration is the minimum number of observations before
// we prefer historical data over the conservative default.
const minSamplesForCalibration = 10

// ToolStats tracks compression performance for one tool/compressor kind using
// Welford's online algorithm for numerically stable mean and variance.
type ToolStats struct {
	Kind        string    `json:"kind"`
	Count       int       `json:"count"`
	MeanRatio   float64   `json:"mean_ratio"`
	M2          float64   `json:"m2"` // sum of squared deviations (Welford)
	LastUpdated time.Time `json:"last_updated"`
}

// StdDev returns the standard deviation of observed output ratios.
func (s *ToolStats) StdDev() float64 {
	if s.Count < 2 {
		return 0.10 // default uncertainty before we have data
	}
	return math.Sqrt(s.M2 / float64(s.Count-1))
}

// ConservativeRatio returns mean + 1.5σ, bounding the ratio from above.
// A higher ratio means less compression, so being conservative means we
// under-estimate savings. Caps at 1.0 (no compression at all).
func (s *ToolStats) ConservativeRatio() float64 {
	if s.Count < minSamplesForCalibration {
		return DefaultOutputRatio
	}
	r := s.MeanRatio + 1.5*s.StdDev()
	if r > 1.0 {
		return 1.0
	}
	if r < 0.01 {
		return 0.01
	}
	return r
}

// update adds one observation using Welford's online algorithm.
func (s *ToolStats) update(ratio float64) {
	s.Count++
	delta := ratio - s.MeanRatio
	s.MeanRatio += delta / float64(s.Count)
	delta2 := ratio - s.MeanRatio
	s.M2 += delta * delta2
	s.LastUpdated = time.Now().UTC()
}

// Calibration holds compression-performance statistics per tool kind.
// It implements the telemetry feedback loop:
//
//	prediction → execution → actual usage → outcome → improved prediction
//
// It is safe for concurrent use.
type Calibration struct {
	mu    sync.Mutex
	Tools map[string]*ToolStats `json:"tools"`
}

// NewCalibration returns a Calibration with no history. All estimates use
// DefaultOutputRatio until enough observations accumulate.
func NewCalibration() *Calibration {
	return &Calibration{Tools: make(map[string]*ToolStats)}
}

// LoadCalibration reads the calibration file. If it does not exist or cannot
// be read, a fresh calibration is returned — not an error, because missing
// calibration data means "use defaults", not failure.
func LoadCalibration() *Calibration {
	c := NewCalibration()
	b, err := os.ReadFile(calibrationPath())
	if err != nil {
		return c
	}
	if err := json.Unmarshal(b, c); err != nil {
		return NewCalibration()
	}
	if c.Tools == nil {
		c.Tools = make(map[string]*ToolStats)
	}
	return c
}

// EstimatedOutputRatio returns the estimated output_tokens/input_tokens ratio
// for a given compressor kind, using historical data when sufficient.
// kind="" means a generic LLM worker call with no specific compressor.
func (c *Calibration) EstimatedOutputRatio(kind string) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := kind
	if k == "" {
		k = "_worker"
	}
	if s, ok := c.Tools[k]; ok && s.Count >= minSamplesForCalibration {
		return s.ConservativeRatio()
	}
	return DefaultOutputRatio
}

// RecordActual updates the calibration with one observed compression result.
// inputTokens is what was sent to the worker (not including system overhead);
// outputTokens is what the worker produced.
// This should be called after every successful worker invocation.
func (c *Calibration) RecordActual(kind string, inputTokens, outputTokens int) {
	if inputTokens <= 0 {
		return
	}
	ratio := float64(outputTokens) / float64(inputTokens)
	if ratio <= 0 || ratio > 5 { // sanity guard
		return
	}

	k := kind
	if k == "" {
		k = "_worker"
	}

	c.mu.Lock()
	s, ok := c.Tools[k]
	if !ok {
		s = &ToolStats{Kind: k}
		c.Tools[k] = s
	}
	s.update(ratio)
	c.mu.Unlock()

	// Save asynchronously so the hook path isn't delayed by disk I/O.
	go func() { _ = c.save() }()
}

// Stats returns a snapshot of calibration statistics for display.
func (c *Calibration) Stats() map[string]ToolStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]ToolStats, len(c.Tools))
	for k, v := range c.Tools {
		out[k] = *v
	}
	return out
}

func (c *Calibration) save() error {
	c.mu.Lock()
	b, err := json.MarshalIndent(c, "", "  ")
	c.mu.Unlock()
	if err != nil {
		return err
	}
	p := calibrationPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

func calibrationPath() string {
	return filepath.Join(paths.Home(), "calibration.json")
}
