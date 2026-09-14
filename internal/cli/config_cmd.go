package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/paths"
)

// Config implements the `skim config` subcommand. With no args it prints the
// current config as pretty JSON. Otherwise the first arg selects an action:
// `set <key> <value>`, `--clear-cache`, `off`, or `on`.
func Config(w io.Writer, args []string) error {
	if len(args) == 0 {
		c, err := config.Load()
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(c, "", "  ")
		fmt.Fprintln(w, string(b))
		return nil
	}
	switch args[0] {
	case "--clear-cache":
		n, err := clearCache()
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "removed %d cache entries\n", n)
		return nil
	case "off":
		return setField(w, "disabled", "true")
	case "on":
		return setField(w, "disabled", "false")
	case "set":
		if len(args) != 3 {
			return fmt.Errorf("usage: skim config set <key> <value>")
		}
		return setField(w, args[1], args[2])
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func setField(w io.Writer, key, val string) error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	switch key {
	case "disabled":
		c.Disabled = val == "true"
	case "read_max_lines":
		c.ReadMaxLines, err = strconv.Atoi(val)
	case "read_max_bytes":
		c.ReadMaxBytes, err = strconv.Atoi(val)
	case "grep_max_matches":
		c.GrepMaxMatches, err = strconv.Atoi(val)
	case "worker_timeout_sec":
		c.WorkerTimeoutSec, err = strconv.Atoi(val)
	case "model":
		c.Model = val
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	if err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if err := config.Save(c); err != nil {
		return err
	}
	fmt.Fprintf(w, "set %s = %s\n", key, val)
	return nil
}

func clearCache() (int, error) {
	entries, err := os.ReadDir(paths.CacheDir())
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			if os.Remove(filepath.Join(paths.CacheDir(), e.Name())) == nil {
				n++
			}
		}
	}
	return n, nil
}
