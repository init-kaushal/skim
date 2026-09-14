// Package cli implements skim's user-facing subcommands: doctor, stats,
// config, and cat. Each command is a single function writing to an io.Writer.
package cli

import (
	"io"
	"os"
)

// Cat streams the whole file at path to w with no size checks and no
// interception — the deliberate escape hatch when the full contents are needed.
func Cat(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}
