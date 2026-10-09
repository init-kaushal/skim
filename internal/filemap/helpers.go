package filemap

import (
	"bufio"
	"strings"
)

// splitLines splits src into individual lines. An empty string returns nil.
func splitLines(src string) []string {
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(src))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}
