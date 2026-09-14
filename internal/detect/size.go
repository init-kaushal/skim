package detect

import (
	"bufio"
	"io"
	"os"

	"github.com/kaushal/skim/internal/config"
)

func FileSize(path string) (lines, bytes int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	buf := make([]byte, 32*1024)
	var lastByte byte
	for {
		n, rerr := r.Read(buf)
		for _, b := range buf[:n] {
			if b == '\n' {
				lines++
			}
		}
		if n > 0 {
			lastByte = buf[n-1]
		}
		bytes += n
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return 0, 0, rerr
		}
	}
	if bytes > 0 && lastByte != '\n' {
		lines++
	}
	return lines, bytes, nil
}

func ExceedsThreshold(lines, bytes int, cfg config.Config) bool {
	return lines > cfg.ReadMaxLines || bytes > cfg.ReadMaxBytes
}
