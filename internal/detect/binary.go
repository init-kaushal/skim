package detect

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// binaryExts are extensions skim never digests. Shipping these bytes to a
// summariser burns an API call to describe noise, and — worse — denying the
// Read means the model can never view the image or PDF at all while skim is
// installed. Passing them through is the only useful behaviour.
var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".ico": true, ".svg": true, ".pdf": true, ".ipynb": true,
	".zip": true, ".tar": true, ".gz": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true,
	".mp3": true, ".mp4": true, ".mov": true, ".wav": true,
}

// sniffBytes is how much of a file's head LooksBinary inspects. 512 bytes is
// the conventional content-sniffing window and is plenty to find a NUL.
const sniffBytes = 512

// LooksBinary reports whether path is likely non-text, by extension first and
// then by sniffing the first sniffBytes for a NUL byte — the standard
// binary-detection heuristic, since well-formed text never contains one.
//
// An error means "could not tell". Callers must NOT treat that as a positive
// detection: only a definite yes should divert a file away from interception.
func LooksBinary(path string) (bool, error) {
	if binaryExts[strings.ToLower(filepath.Ext(path))] {
		return true, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, sniffBytes)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}
