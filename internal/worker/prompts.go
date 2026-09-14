package worker

import "fmt"

// promptFor builds the instruction sent to the nested `claude -p` process for a
// given request kind. Every branch asks for a bare JSON object matching the
// schema that internal/digest parses.
func promptFor(req Request) string {
	switch req.Kind {
	case KindFileMap:
		// What the worker is shown is capped, so on a large file this is a
		// prefix. Saying "the full contents" and demanding a map "top to bottom
		// with no gaps" made the model extrapolate past what it was given —
		// observed emitting ranges up to line 556 for a 411-line file. Describe
		// the excerpt honestly and bound the map to it.
		scope, rule := "the full contents of the file", "The map must cover the file top to bottom with no gaps."
		if req.Partial {
			scope = "the FIRST PART ONLY (the file is longer than this excerpt) of the file"
			rule = "The map must cover the excerpt shown top to bottom with no gaps. " +
				"Do NOT describe or guess at anything beyond where the excerpt ends, " +
				"and do not emit line numbers past its final line."
		}
		return fmt.Sprintf(`You are a code-reading assistant. Below is %s %s.
Return ONLY a JSON object, no prose, with this shape:
{"summary": "<3-5 sentences on purpose and shape>",
 "map": [{"lines": "<start>-<end>", "kind": "<what lives there>"}, ...],
 "symbols": ["<top-level names, at most 40 of the most significant>"],
 "notes": "line numbers approximate +/- 3"}
%s

FILE CONTENTS:
%s`, scope, req.Meta, rule, req.Content)

	case KindClusters:
		return fmt.Sprintf(`You are a search-result summariser. Below are ripgrep matches for pattern %q.
Return ONLY a JSON object, no prose:
{"summary": "<what the matches collectively represent>",
 "clusters": [{"where": "<dir or file prefix>", "matches": <int>, "gist": "<why these match>"}, ...],
 "total": <int total matches>,
 "representative_files": ["<path>", ...]}

MATCHES:
%s`, req.Meta, req.Content)

	case KindRun:
		return fmt.Sprintf(`You are a command-output summariser. Below is the tail of output from: %s
Return ONLY a JSON object, no prose:
{"summary": "<1-3 sentence outcome>",
 "key_lines": ["<the few lines that matter: errors, failures, totals>"],
 "exit_code": <int>,
 "log_path": ""}

OUTPUT:
%s`, req.Meta, req.Content)
	}
	return req.Content
}
