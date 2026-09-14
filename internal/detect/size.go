package detect

import (
	"bufio"
	"io"
	"os"

	"github.com/kaushal/skim/internal/config"
)

// Size is a file's extent, plus where a line-bounded prefix of it ends.
type Size struct {
	Lines int
	Bytes int
	// PrefixBytes is the byte length of the file's first prefixLines lines,
	// including their terminating newlines. It equals Bytes when the file has
	// no more lines than that. Callers cap what they ship to the worker at this
	// offset, so the digest describes a whole number of lines rather than a
	// prefix cut mid-token.
	PrefixBytes int
}

// Measure walks path once, returning its line and byte count along with the
// byte offset at which its first prefixLines lines end. A prefixLines of zero
// or less leaves PrefixBytes equal to Bytes.
func Measure(path string, prefixLines int) (Size, error) {
	f, err := os.Open(path)
	if err != nil {
		return Size{}, err
	}
	defer f.Close()

	var s Size
	r := bufio.NewReader(f)
	buf := make([]byte, 32*1024)
	var lastByte byte
	foundPrefix := false
	for {
		n, rerr := r.Read(buf)
		for i, b := range buf[:n] {
			if b != '\n' {
				continue
			}
			s.Lines++
			if !foundPrefix && prefixLines > 0 && s.Lines == prefixLines {
				// Offset just past this newline: a whole number of lines.
				s.PrefixBytes = s.Bytes + i + 1
				foundPrefix = true
			}
		}
		if n > 0 {
			lastByte = buf[n-1]
		}
		s.Bytes += n
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return Size{}, rerr
		}
	}
	// A final line with no trailing newline still counts as a line.
	if s.Bytes > 0 && lastByte != '\n' {
		s.Lines++
	}
	// Either no cap was requested, or the file has fewer lines than the cap.
	// Both mean the prefix is the whole file.
	if !foundPrefix {
		s.PrefixBytes = s.Bytes
	}
	return s, nil
}

// FileSize reports a file's line and byte count. It is Measure without the
// prefix bookkeeping, kept for callers that only need the extent.
func FileSize(path string) (lines, bytes int, err error) {
	s, err := Measure(path, 0)
	return s.Lines, s.Bytes, err
}

func ExceedsThreshold(lines, bytes int, cfg config.Config) bool {
	return lines > cfg.ReadMaxLines || bytes > cfg.ReadMaxBytes
}
