package paths

import (
	"os"
	"path/filepath"
)

func Home() string {
	if h := os.Getenv("SKIM_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "skim")
	}
	return filepath.Join(home, ".claude", "skim")
}

func Config() string   { return filepath.Join(Home(), "config.json") }
func CacheDir() string { return filepath.Join(Home(), "cache") }
func RunsDir() string  { return filepath.Join(Home(), "runs") }
func Metrics() string  { return filepath.Join(Home(), "metrics.jsonl") }
func Log() string      { return filepath.Join(Home(), "skim.log") }
