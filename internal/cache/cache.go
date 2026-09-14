package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/paths"
)

func Key(path, model string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00%s\x00%s",
		abs, st.ModTime().UnixNano(), st.Size(), model, strconv.Itoa(digest.SchemaVersion))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func file(key string) string { return filepath.Join(paths.CacheDir(), key+".json") }

func Get(key string) (digest.FileMap, bool) {
	b, err := os.ReadFile(file(key))
	if err != nil {
		return digest.FileMap{}, false
	}
	var fm digest.FileMap
	if err := json.Unmarshal(b, &fm); err != nil {
		return digest.FileMap{}, false
	}
	return fm, true
}

func Put(key string, fm digest.FileMap) error {
	if err := os.MkdirAll(paths.CacheDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(fm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file(key), b, 0o644)
}

func Sweep(maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(paths.CacheDir())
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if os.Remove(filepath.Join(paths.CacheDir(), e.Name())) == nil {
				removed++
			}
		}
	}
	return removed, nil
}
