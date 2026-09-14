package metrics

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kaushal/skim/internal/paths"
)

type Entry struct {
	TS              string `json:"ts"`
	Tool            string `json:"tool"`
	OrigTokensEst   int    `json:"orig_tokens_est"`
	DigestTokensEst int    `json:"digest_tokens_est"`
	WorkerTokens    int    `json:"worker_tokens"`
	CacheHit        bool   `json:"cache_hit"`
	SavedEst        int    `json:"saved_est"`
}

func Path() string { return paths.Metrics() }

func EstimateTokens(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 3) / 4
}

func Record(e Entry) error {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(Path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

type Stats struct {
	Interceptions map[string]int
	CacheHits     int
	CacheMisses   int
	TotalSavedEst int
	// TotalWorkerTokens is what the nested Haiku calls cost. Without it
	// TotalSavedEst is only half the ledger and can't answer whether the trade
	// nets out positive.
	TotalWorkerTokens int
}

func Aggregate(r io.Reader) (Stats, error) {
	s := Stats{Interceptions: map[string]int{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		if e.Tool != "" {
			s.Interceptions[e.Tool]++
		}
		if e.CacheHit {
			s.CacheHits++
		} else {
			s.CacheMisses++
		}
		s.TotalSavedEst += e.SavedEst
		s.TotalWorkerTokens += e.WorkerTokens
	}
	return s, sc.Err()
}
