package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/kaushal/skim/internal/digest"
	"github.com/kaushal/skim/internal/filemap"
)

// Map generates and prints the deterministic file map for the named file.
// It is used to inspect what the AST-based compressor would produce before
// committing to an interception strategy.
func Map(w io.Writer, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	fm, ok := filemap.Generate(string(src), path)
	if !ok {
		fmt.Fprintf(w, "no deterministic map available for %s\n", path)
		fmt.Fprintln(w, "(file type not supported or content could not be parsed)")
		return nil
	}

	// Show the rendered output — exactly what would be injected as the deny
	// reason in the hook. Uses a dummy self-path since we are running on PATH.
	selfPath, _ := os.Executable()
	cov := digest.Coverage{
		SeenBytes: len(src), TotalBytes: len(src),
	}
	fmt.Fprint(w, digest.RenderFileMap(fm, path, selfPath, cov))
	return nil
}
