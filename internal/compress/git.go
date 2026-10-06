package compress

import (
	"fmt"
	"strings"
)

// compressGitStatus compacts `git status` output.
// Produces a 3-5 line summary: branch, counts, and up to 10 key paths.
func compressGitStatus(input string) (string, bool, bool) {
	if !strings.Contains(input, "On branch") && !strings.Contains(input, "HEAD detached") {
		return "", false, false
	}
	// Require at least some status markers
	if !containsAny(input, "modified:", "new file:", "deleted:", "untracked:", "Untracked files:") {
		return "", false, false
	}

	lines := strings.Split(input, "\n")
	var branch string
	var header []string // first few lines (branch, tracking info)
	var changed, staged, untracked []string

	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "On branch") || strings.HasPrefix(t, "HEAD detached") {
			branch = t
			header = append(header, t)
			continue
		}
		if strings.HasPrefix(t, "Your branch") || strings.HasPrefix(t, "nothing to commit") {
			header = append(header, t)
			continue
		}
		if strings.HasPrefix(t, "modified:") || strings.HasPrefix(t, "deleted:") ||
			strings.HasPrefix(t, "renamed:") || strings.HasPrefix(t, "both modified:") {
			changed = append(changed, "\t"+t)
		} else if strings.HasPrefix(t, "new file:") {
			staged = append(staged, "\t"+t)
		} else if t != "" && !strings.HasPrefix(t, "#") &&
			!strings.HasPrefix(t, "(use") && !strings.HasPrefix(t, "Changes") &&
			!strings.HasPrefix(t, "Untracked") && !strings.HasPrefix(t, "no changes") {
			// Could be an untracked file (indented in older git)
			if strings.HasPrefix(line, "\t") {
				untracked = append(untracked, line)
			}
		}
	}
	_ = branch

	const maxPaths = 10
	var out []string
	out = append(out, header...)

	if len(changed) > 0 {
		shown := changed
		omitted := 0
		if len(shown) > maxPaths {
			omitted = len(shown) - maxPaths
			shown = shown[:maxPaths]
		}
		out = append(out, fmt.Sprintf("Changes (%d):", len(changed)))
		out = append(out, shown...)
		if omitted > 0 {
			out = append(out, fmt.Sprintf("\t[+%d more]", omitted))
		}
	}
	if len(staged) > 0 {
		out = append(out, fmt.Sprintf("Staged (%d):", len(staged)))
		shown := staged
		if len(shown) > maxPaths {
			shown = shown[:maxPaths]
		}
		out = append(out, shown...)
	}
	if len(untracked) > 0 {
		out = append(out, fmt.Sprintf("Untracked (%d):", len(untracked)))
		shown := untracked
		if len(shown) > maxPaths {
			shown = shown[:maxPaths]
		}
		out = append(out, shown...)
	}

	result := strings.Join(out, "\n")
	if len(result) >= len(input) {
		return input, true, true
	}
	return result, true, true
}

// compressGitDiff compacts `git diff` output.
// Produces a file-level summary table; keeps hunks for small diffs.
// Returns complete=false because the file summary benefits from LLM context.
func compressGitDiff(input string) (string, bool, bool) {
	if !strings.Contains(input, "diff --git") {
		return "", false, false
	}

	type fileDiff struct {
		path     string
		added    int
		removed  int
		sections []string // kept hunks (errors, security tokens, etc.)
	}

	var files []fileDiff
	var cur *fileDiff
	var hunk []string

	flushHunk := func() {
		if cur != nil && len(hunk) > 0 {
			// Keep hunks that touch error/security-sensitive lines
			keep := false
			for _, h := range hunk {
				if containsAny(h, "password", "secret", "token", "key",
					"Error", "error", "panic", "TODO", "FIXME", "HACK") {
					keep = true
					break
				}
			}
			if keep {
				cur.sections = append(cur.sections, strings.Join(hunk, "\n"))
			}
			hunk = nil
		}
	}

	for _, line := range strings.Split(input, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			flushHunk()
			if cur != nil {
				files = append(files, *cur)
			}
			// Extract path from "diff --git a/foo b/foo"
			parts := strings.Fields(line)
			path := ""
			if len(parts) >= 4 {
				path = strings.TrimPrefix(parts[3], "b/")
			}
			cur = &fileDiff{path: path}
			continue
		}
		if strings.HasPrefix(line, "@@") {
			flushHunk()
			hunk = []string{line}
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			if cur != nil {
				cur.added++
			}
			if hunk != nil {
				hunk = append(hunk, line)
			}
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			if cur != nil {
				cur.removed++
			}
			if hunk != nil {
				hunk = append(hunk, line)
			}
		} else if hunk != nil {
			hunk = append(hunk, line)
		}
	}
	flushHunk()
	if cur != nil {
		files = append(files, *cur)
	}

	if len(files) == 0 {
		return "", false, false
	}

	var sb strings.Builder
	totalAdded, totalRemoved := 0, 0
	for _, f := range files {
		totalAdded += f.added
		totalRemoved += f.removed
	}
	fmt.Fprintf(&sb, "git diff — %d file(s), +%d -%d lines\n\n", len(files), totalAdded, totalRemoved)

	for _, f := range files {
		fmt.Fprintf(&sb, "  %-50s +%-4d -%d\n", f.path, f.added, f.removed)
	}

	// Append kept sensitive hunks
	for _, f := range files {
		for _, section := range f.sections {
			fmt.Fprintf(&sb, "\n[%s — sensitive hunk]\n%s\n", f.path, section)
		}
	}

	result := sb.String()
	if len(result) >= len(input) {
		return input, false, true
	}
	// complete=false: the summary is useful but an LLM can add more context
	return result, false, true
}

// compressGitLog compacts `git log` output.
// Keeps hash + author + date + subject; drops bodies unless there are ≤5 commits.
func compressGitLog(input string) (string, bool, bool) {
	if !containsAny(input, "commit ", "Author:", "Date:") {
		return "", false, false
	}
	// Only trigger when there are many lines
	if strings.Count(input, "\n") < 30 {
		return "", false, false
	}

	type commit struct {
		hash, author, date, subject string
		body                        []string
	}

	var commits []commit
	var cur *commit

	for _, line := range strings.Split(input, "\n") {
		if strings.HasPrefix(line, "commit ") && len(line) > 7 {
			if cur != nil {
				commits = append(commits, *cur)
			}
			cur = &commit{hash: line[7:14]} // short hash
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "Author: ") {
			cur.author = strings.TrimPrefix(line, "Author: ")
			// Trim email for brevity
			if i := strings.Index(cur.author, " <"); i > 0 {
				cur.author = cur.author[:i]
			}
		} else if strings.HasPrefix(line, "Date:   ") {
			cur.date = strings.TrimSpace(strings.TrimPrefix(line, "Date:   "))
			// Trim time, keep date
			if i := strings.Index(cur.date, " "); i > 0 {
				cur.date = cur.date[:i]
			}
		} else if cur.subject == "" && strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "    Merge:") {
			cur.subject = strings.TrimSpace(line)
		} else if cur.subject != "" {
			cur.body = append(cur.body, line)
		}
	}
	if cur != nil {
		commits = append(commits, *cur)
	}

	if len(commits) == 0 {
		return "", false, false
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d commits:\n\n", len(commits))
	for _, c := range commits {
		fmt.Fprintf(&sb, "%s  %s  %s\n    %s\n", c.hash, c.date, c.author, c.subject)
	}

	result := sb.String()
	if len(result) >= len(input) {
		return input, true, true
	}
	return result, true, true
}
