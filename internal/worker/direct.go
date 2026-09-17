package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// The direct path exists because shelling out to `claude -p` carries a fixed
// overhead that has nothing to do with the digest. Every nested CLI call pays
// for Claude Code's own system prompt — measured at 5,509 tokens with
// `--tools ""` — and Claude Code writes it to a 1-hour cache, billed at 2x the
// input rate. That is roughly $0.011 per cold call before any of the file's
// content is considered, and on a small file it dominated the bill.
//
// Talking to /v1/messages directly removes all of it: no Claude Code system
// prompt, no caching of one-shot content that will never be read again, no
// child process, and no need for the SKIM_ACTIVE recursion guard because there
// is no nested session to re-enter skim's own hooks.
//
// It is not a replacement, though — see credentials(). A Claude Code
// subscription keeps its OAuth token in the OS keychain, where no API client is
// meant to reach in and take it, so subscription-only installs still go through
// the CLI. Run() picks whichever is actually available.

const (
	defaultBaseURL   = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"

	// oauthBeta is required when authenticating with a bearer token rather than
	// an API key.
	oauthBeta = "oauth-2025-04-20"

	// maxOutputTokens bounds the digest. Real digests measured 353–1,339 output
	// tokens across file sizes from 17KB to 400KB, so this is generous headroom
	// without inviting the model to ramble. Hitting the cap truncates the JSON
	// mid-object, which fails to parse and degrades open — the error says so
	// explicitly rather than looking like a malformed response.
	maxOutputTokens = 4096
)

// creds is how the direct path authenticates.
type creds struct {
	apiKey      string // x-api-key
	bearerToken string // Authorization: Bearer, needs the oauth beta header
}

// credentials resolves API credentials from the environment, or reports that
// the direct path is unavailable.
//
// Only explicit environment credentials count. Notably absent: reading the
// Claude Code subscription token out of the OS keychain. It is there — on macOS
// as a `Claude Code-credentials` entry holding a `claudeAiOauth` access token —
// and it would technically work, but a plugin that reaches into the keychain to
// borrow a session credential for its own out-of-band API calls is the wrong
// shape on every axis: it is fragile against token refresh and expiry, it is a
// credential-exfiltration pattern regardless of intent, and a subscription is
// not an API entitlement. Subscription users get the CLI transport, which is
// exactly what they had before.
func credentials(env func(string) string) (creds, bool) {
	if k := env("ANTHROPIC_API_KEY"); k != "" {
		return creds{apiKey: k}, true
	}
	if t := env("ANTHROPIC_AUTH_TOKEN"); t != "" {
		return creds{bearerToken: t}, true
	}
	return creds{}, false
}

// baseURL honours ANTHROPIC_BASE_URL so a gateway or proxy — and the test
// suite — can redirect the call.
func baseURL(env func(string) string) string {
	if u := env("ANTHROPIC_BASE_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return defaultBaseURL
}

// apiRequest is the /v1/messages body. Deliberately minimal:
//
//   - no `system`: the prompt carries its own instructions, and an empty system
//     prompt is the entire point of this transport.
//   - no `thinking`: on Haiku 4.5 omitting it means no thinking, which is what
//     we want — a structural digest is extraction, not reasoning, and thinking
//     cost 65% of the bill and 5.6x the latency when it was on.
//   - no `cache_control`: this content is read once and never again, so a cache
//     write at 1.25–2x the input rate would be pure loss.
type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []apiMessage `json:"messages"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

type apiError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

// runDirect performs one POST /v1/messages and returns the digest body plus
// what it billed.
//
// It does not retry. A retry inside a PreToolUse hook spends the user's
// latency budget twice over on an operation that is only an optimisation, and
// skim's contract is to degrade open quickly rather than to try hard. The HTTP
// status is carried into the error so a 429 or 529 is recognisable in skim.log.
func runDirect(ctx context.Context, req Request, c creds, base string, hc *http.Client) ([]byte, Usage, error) {
	body, err := json.Marshal(apiRequest{
		Model:     req.Model,
		MaxTokens: maxOutputTokens,
		Messages:  []apiMessage{{Role: "user", Content: promptFor(req)}},
	})
	if err != nil {
		return nil, Usage{}, fmt.Errorf("worker: marshal request: %w", err)
	}

	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, Usage{}, fmt.Errorf("worker: build request: %w", err)
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("anthropic-version", anthropicVersion)
	if c.apiKey != "" {
		hr.Header.Set("x-api-key", c.apiKey)
	} else {
		// An OAuth bearer token needs the beta opt-in; an API key must not have it.
		hr.Header.Set("Authorization", "Bearer "+c.bearerToken)
		hr.Header.Set("anthropic-beta", oauthBeta)
	}

	resp, err := hc.Do(hr)
	if err != nil {
		// A context deadline surfaces here; report it as skim's own timeout so
		// callers can match on it the same way they do for the CLI transport.
		if ctx.Err() == context.DeadlineExceeded {
			return nil, Usage{}, fmt.Errorf("%w after %s", ErrTimeout, req.Timeout)
		}
		return nil, Usage{}, fmt.Errorf("worker: POST /v1/messages: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, Usage{}, fmt.Errorf("worker: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var ae apiError
		if json.Unmarshal(raw, &ae) == nil && ae.Error.Message != "" {
			return nil, Usage{}, fmt.Errorf("worker: api %d %s: %s",
				resp.StatusCode, ae.Error.Type, ae.Error.Message)
		}
		return nil, Usage{}, fmt.Errorf("worker: api %d: %s",
			resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var ar apiResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, Usage{}, fmt.Errorf("worker: bad api response: %w", err)
	}

	use := Usage{
		InputTokens:      ar.Usage.InputTokens,
		OutputTokens:     ar.Usage.OutputTokens,
		CacheWriteTokens: ar.Usage.CacheCreationInputTokens,
		CacheReadTokens:  ar.Usage.CacheReadInputTokens,
	}
	// The Messages API reports tokens but not money — `total_cost_usd` is a
	// CLI-only convenience. So this transport has to price its own call; see
	// estimateCostUSD for why that is a compromise rather than an improvement.
	use.CostUSD = estimateCostUSD(req.Model, use)

	// Concatenate text blocks: the response is an array and a digest could in
	// principle arrive split across several.
	var sb strings.Builder
	for _, b := range ar.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}

	if ar.StopReason == "max_tokens" {
		// Say this plainly. The symptom is otherwise an unparseable digest, and
		// "truncated at the output cap" is a different problem from "the model
		// returned something malformed".
		return nil, use, fmt.Errorf("worker: response hit the %d-token output cap and is truncated",
			maxOutputTokens)
	}

	out := stripCodeFence(sb.String())
	if out == "" {
		return nil, use, fmt.Errorf("worker: empty result (stop_reason %q)", ar.StopReason)
	}
	return []byte(out), use, nil
}

// workerPrices is the per-million-token price of models skim runs as workers.
//
// This table is a regression and worth being honest about. The CLI transport
// reports `total_cost_usd` for every call, which is authoritative and survives
// price changes; the Messages API does not, so the direct transport has to
// compute a figure instead. A stale entry here makes `skim stats` quietly
// wrong in a way the CLI path cannot be.
//
// It is kept narrow on purpose — worker models only, which skim pins to a cheap
// tier — and an unknown model yields a zero cost rather than a guess, which
// `skim stats` already reports as "n of m calls reported cost".
//
// Source: the claude-api skill's model table. Re-check when adding a model.
var workerPrices = map[string]struct{ in, out float64 }{
	"claude-haiku-4-5":          {in: 1.00, out: 5.00},
	"claude-haiku-4-5-20251001": {in: 1.00, out: 5.00},
}

// Cache multipliers on the input rate. The direct transport sends no
// cache_control, so these normally apply to zero tokens; they are here so the
// figure stays correct if a future change starts caching.
const (
	cacheWriteMultiplier = 1.25
	cacheReadMultiplier  = 0.10
)

// estimateCostUSD prices a call from its token counts. Returns 0 for a model
// not in the table, which is reported as "no cost recorded" rather than as free.
func estimateCostUSD(model string, u Usage) float64 {
	p, ok := workerPrices[model]
	if !ok {
		return 0
	}
	return (float64(u.InputTokens)*p.in +
		float64(u.CacheWriteTokens)*p.in*cacheWriteMultiplier +
		float64(u.CacheReadTokens)*p.in*cacheReadMultiplier +
		float64(u.OutputTokens)*p.out) / 1e6
}

// osEnv is the default environment lookup, replaced in tests.
var osEnv = os.Getenv

// Transport describes how Run will reach the model, for `skim doctor`.
type Transport struct {
	Direct bool
	// Why names the deciding credential, or explains the fallback.
	Why string
}

// ActiveTransport reports which path Run would take right now. It exists so
// doctor can say so out loud: the two transports differ by roughly 5.5K tokens
// of Claude Code system prompt per interception, which is most of the cost on a
// small file, and there is otherwise no way for a user to tell which one they
// are getting.
func ActiveTransport() Transport {
	if c, ok := credentials(osEnv); ok {
		if c.apiKey != "" {
			return Transport{Direct: true, Why: "ANTHROPIC_API_KEY is set"}
		}
		return Transport{Direct: true, Why: "ANTHROPIC_AUTH_TOKEN is set"}
	}
	return Transport{Direct: false, Why: "no ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN; " +
		"using the claude CLI, which adds its own system prompt to every call"}
}
