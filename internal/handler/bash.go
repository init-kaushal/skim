package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/kaushal/skim/internal/detect"
	"github.com/kaushal/skim/internal/hookio"
	"github.com/kaushal/skim/internal/metrics"
)

// BashHook is the PreToolUse interception for the Bash tool. Unlike the Read and
// Grep hooks it makes no worker call: it is a pure pattern match. It always
// returns nil after writing exactly one decision to d.Stdout — an allow
// (nothing) when the command looks quiet, or a deny that tells the model to
// re-run the command through `skim run` so the full output is captured to a log
// and reduced to a digest. Every error or negative check degrades open.
func BashHook(_ context.Context, in hookio.Input, d Deps) error {
	if d.Cfg.NotActiveReason(d.Env) != "" {
		return hookio.Allow(d.Stdout)
	}

	bi, err := in.Bash()
	if err != nil {
		d.Logf("bash-hook: bad tool_input: %v", err)
		return hookio.Allow(d.Stdout)
	}

	matched, pat := detect.MatchNoisy(bi.Command, d.Cfg.BashNoisyPatterns)
	if !matched {
		return hookio.Allow(d.Stdout)
	}

	reason := fmt.Sprintf(
		"skim: this command tends to produce large output (matched /%s/).\n"+
			"Re-run it through skim so the full output is captured to a log and you get a digest:\n\n"+
			"  skim run -- %s\n", pat, bi.Command)

	d.Record(metrics.Entry{
		TS:   d.Now().UTC().Format(time.RFC3339),
		Tool: "Bash",
	})
	return hookio.Deny(d.Stdout, reason)
}
