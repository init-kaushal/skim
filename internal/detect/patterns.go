package detect

import (
	"path/filepath"
	"regexp"
	"strings"
)

func MatchNoisy(cmd string, patterns []string) (bool, string) {
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		if re.MatchString(cmd) {
			return true, p
		}
	}
	return false, ""
}

func MatchPassthrough(path string, globs []string) bool {
	base := filepath.Base(path)
	for _, g := range globs {
		re := globToRegexp(g)
		if re.MatchString(path) || re.MatchString(base) {
			return true
		}
	}
	return false
}

func globToRegexp(g string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString(`(^|/)`)
	for i := 0; i < len(g); i++ {
		switch g[i] {
		case '*':
			if i+1 < len(g) && g[i+1] == '*' {
				b.WriteString(`.*`)
				i++
				if i+1 < len(g) && g[i+1] == '/' {
					i++
				}
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(g[i])
		default:
			b.WriteByte(g[i])
		}
	}
	b.WriteString(`$`)
	return regexp.MustCompile(b.String())
}
