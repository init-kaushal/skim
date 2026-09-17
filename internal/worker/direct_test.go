package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// captured holds what the fake API server received, so tests can assert on the
// request body rather than only on the response handling.
type captured struct {
	headers http.Header
	body    map[string]any
	rawBody string
}

// fakeAPI stands in for /v1/messages. status and reply describe what to send
// back; got records what arrived.
func fakeAPI(t *testing.T, status int, reply string, got *captured) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if got != nil {
			got.headers = r.Header.Clone()
			got.rawBody = string(raw)
			_ = json.Unmarshal(raw, &got.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const okReply = `{"content":[{"type":"text","text":"{\"summary\":\"a file\",\"map\":[{\"lines\":\"1-10\",\"kind\":\"x\"}]}"}],
 "stop_reason":"end_turn",
 "usage":{"input_tokens":5000,"output_tokens":400,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`

func fileMapReq() Request {
	return Request{
		Model:   "claude-haiku-4-5-20251001",
		Kind:    KindFileMap,
		Content: "package x\n",
		Meta:    "/x/a.go",
		Timeout: 30 * time.Second,
	}
}

// TestRunDirect_SendsNoSystemPromptOrCaching is the whole reason this transport
// exists. Each nested `claude -p` call paid for Claude Code's own system prompt
// — measured at 5,509 tokens with `--tools ""`, billed as a 1-hour cache write
// at 2x input, about $0.011 before any file content. The direct request must
// carry none of that: no system prompt, no cache_control on one-shot content,
// and no thinking (which cost 65% of the bill when it was on).
func TestRunDirect_SendsNoSystemPromptOrCaching(t *testing.T) {
	var got captured
	srv := fakeAPI(t, http.StatusOK, okReply, &got)

	_, use, err := runDirect(context.Background(), fileMapReq(),
		creds{apiKey: "sk-test"}, srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{"system", "cache_control", "thinking"} {
		if _, present := got.body[forbidden]; present {
			t.Errorf("request must not carry %q: %s", forbidden, got.rawBody)
		}
	}
	if strings.Contains(got.rawBody, "cache_control") {
		t.Errorf("no cache_control anywhere in the body: %s", got.rawBody)
	}
	// The prompt itself must still arrive, as a single user message.
	msgs, _ := got.body["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v, want exactly one user message", got.body["messages"])
	}
	m, _ := msgs[0].(map[string]any)
	if m["role"] != "user" {
		t.Errorf("role = %v, want user", m["role"])
	}
	if !strings.Contains(m["content"].(string), "/x/a.go") {
		t.Errorf("prompt did not reach the request: %v", m["content"])
	}
	if got.body["model"] != "claude-haiku-4-5-20251001" {
		t.Errorf("model = %v, want the request's model", got.body["model"])
	}
	if got.body["max_tokens"].(float64) != float64(maxOutputTokens) {
		t.Errorf("max_tokens = %v, want %d", got.body["max_tokens"], maxOutputTokens)
	}

	// Usage must be carried per tier, as the CLI transport does.
	if use.InputTokens != 5000 || use.OutputTokens != 400 {
		t.Errorf("usage = %+v, want 5000 in / 400 out", use)
	}
}

// TestRunDirect_AuthHeaders pins the two credential shapes. An API key goes in
// x-api-key with no beta header; a bearer token needs the oauth beta opt-in.
// Sending the beta header with an API key, or omitting it with a token, is
// rejected upstream.
func TestRunDirect_AuthHeaders(t *testing.T) {
	t.Run("api key", func(t *testing.T) {
		var got captured
		srv := fakeAPI(t, http.StatusOK, okReply, &got)
		if _, _, err := runDirect(context.Background(), fileMapReq(),
			creds{apiKey: "sk-test"}, srv.URL, srv.Client()); err != nil {
			t.Fatal(err)
		}
		if got.headers.Get("x-api-key") != "sk-test" {
			t.Errorf("x-api-key = %q", got.headers.Get("x-api-key"))
		}
		if got.headers.Get("Authorization") != "" {
			t.Error("an API key must not also send Authorization")
		}
		if got.headers.Get("anthropic-beta") != "" {
			t.Error("the oauth beta header must not be sent with an API key")
		}
		if got.headers.Get("anthropic-version") != anthropicVersion {
			t.Errorf("anthropic-version = %q, want %q",
				got.headers.Get("anthropic-version"), anthropicVersion)
		}
	})

	t.Run("bearer token", func(t *testing.T) {
		var got captured
		srv := fakeAPI(t, http.StatusOK, okReply, &got)
		if _, _, err := runDirect(context.Background(), fileMapReq(),
			creds{bearerToken: "oauth-tok"}, srv.URL, srv.Client()); err != nil {
			t.Fatal(err)
		}
		if got.headers.Get("Authorization") != "Bearer oauth-tok" {
			t.Errorf("Authorization = %q", got.headers.Get("Authorization"))
		}
		if got.headers.Get("anthropic-beta") != oauthBeta {
			t.Errorf("anthropic-beta = %q, want %q", got.headers.Get("anthropic-beta"), oauthBeta)
		}
		if got.headers.Get("x-api-key") != "" {
			t.Error("a bearer token must not also send x-api-key")
		}
	})
}

// TestRunDirect_StripsCodeFence keeps the original production bug covered on
// the new transport. Haiku wraps its JSON in a markdown fence despite being
// told not to; that made every real interception fail to parse and degrade
// open, silently doing nothing. It was found by a live smoke test, not a unit
// test, so the second transport gets the assertion explicitly.
func TestRunDirect_StripsCodeFence(t *testing.T) {
	fenced := `{"content":[{"type":"text","text":"` + "```json\\n" +
		`{\"summary\":\"s\",\"map\":[{\"lines\":\"1-2\",\"kind\":\"k\"}]}` + "\\n```" +
		`"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	srv := fakeAPI(t, http.StatusOK, fenced, nil)

	out, _, err := runDirect(context.Background(), fileMapReq(),
		creds{apiKey: "k"}, srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(string(out), "`") {
		t.Errorf("fence not stripped: %q", out)
	}
	var probe map[string]any
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Errorf("result should be parseable JSON, got %q: %v", out, err)
	}
}

// TestRunDirect_ConcatenatesTextBlocks covers content arriving split across
// blocks, and non-text blocks being ignored rather than stringified into the
// digest.
func TestRunDirect_ConcatenatesTextBlocks(t *testing.T) {
	split := `{"content":[
	  {"type":"thinking","thinking":"ignore me"},
	  {"type":"text","text":"{\"summary\":\"ab\","},
	  {"type":"text","text":"\"map\":[{\"lines\":\"1\",\"kind\":\"k\"}]}"}
	],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	srv := fakeAPI(t, http.StatusOK, split, nil)

	out, _, err := runDirect(context.Background(), fileMapReq(),
		creds{apiKey: "k"}, srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "ignore me") {
		t.Errorf("non-text blocks must not reach the digest: %q", out)
	}
	var probe map[string]any
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Errorf("blocks not concatenated into valid JSON: %q", out)
	}
}

// TestRunDirect_ErrorsCarryStatus makes a failure diagnosable from skim.log.
// The hook degrades open either way, so the log line is the only signal the
// user gets.
func TestRunDirect_ErrorsCarryStatus(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		status      int
		wantIn      []string
	}{
		{
			name:   "auth",
			status: http.StatusUnauthorized,
			reply:  `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`,
			wantIn: []string{"401", "authentication_error", "invalid x-api-key"},
		},
		{
			name:   "rate limit",
			status: http.StatusTooManyRequests,
			reply:  `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`,
			wantIn: []string{"429", "rate_limit_error"},
		},
		{
			name:   "unparseable body",
			status: http.StatusBadGateway,
			reply:  `<html>gateway</html>`,
			wantIn: []string{"502", "gateway"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeAPI(t, tc.status, tc.reply, nil)
			_, _, err := runDirect(context.Background(), fileMapReq(),
				creds{apiKey: "k"}, srv.URL, srv.Client())
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tc.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should mention %q", err, want)
				}
			}
		})
	}
}

// TestRunDirect_MaxTokensIsNamed distinguishes a truncated digest from a
// malformed one. Both fail to parse; only one is fixed by raising the cap.
func TestRunDirect_MaxTokensIsNamed(t *testing.T) {
	truncated := `{"content":[{"type":"text","text":"{\"summary\":\"cut off mid"}],
	 "stop_reason":"max_tokens","usage":{"input_tokens":10,"output_tokens":4096}}`
	srv := fakeAPI(t, http.StatusOK, truncated, nil)

	_, use, err := runDirect(context.Background(), fileMapReq(),
		creds{apiKey: "k"}, srv.URL, srv.Client())
	if err == nil {
		t.Fatal("a truncated response must be an error")
	}
	if !strings.Contains(err.Error(), "output cap") {
		t.Errorf("error should name the cap, got: %v", err)
	}
	// Usage is still reported: the call was billed even though it was unusable.
	if use.OutputTokens != 4096 {
		t.Errorf("usage should survive the error, got %+v", use)
	}
}

// TestRunDirect_TimeoutMatchesErrTimeout keeps the two transports
// interchangeable for callers, which test timeouts with errors.Is.
func TestRunDirect_TimeoutMatchesErrTimeout(t *testing.T) {
	// The handler must be released by the test, not left parked on the request
	// context: httptest's Close waits for outstanding handlers, so waiting only
	// on r.Context() deadlocks Close against the very request it is draining.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	req := fileMapReq()
	req.Timeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), req.Timeout)
	defer cancel()

	_, _, err := runDirect(ctx, req, creds{apiKey: "k"}, srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("error should satisfy errors.Is(err, ErrTimeout), got %v", err)
	}
}

func TestCredentials(t *testing.T) {
	envOf := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	t.Run("api key wins", func(t *testing.T) {
		c, ok := credentials(envOf(map[string]string{
			"ANTHROPIC_API_KEY": "sk", "ANTHROPIC_AUTH_TOKEN": "tok",
		}))
		if !ok || c.apiKey != "sk" || c.bearerToken != "" {
			t.Errorf("got %+v ok=%v, want the API key to take precedence", c, ok)
		}
	})
	t.Run("token alone", func(t *testing.T) {
		c, ok := credentials(envOf(map[string]string{"ANTHROPIC_AUTH_TOKEN": "tok"}))
		if !ok || c.bearerToken != "tok" {
			t.Errorf("got %+v ok=%v", c, ok)
		}
	})
	t.Run("none", func(t *testing.T) {
		// This is the subscription case: credentials live in the OS keychain,
		// which skim deliberately does not read. The direct transport must
		// report itself unavailable so Run falls back to the CLI.
		if _, ok := credentials(envOf(nil)); ok {
			t.Error("no env credentials must mean the direct transport is unavailable")
		}
	})
}

// TestChooseTransport_FallsBackWithoutCredentials pins the dispatch rule. A
// subscription-only install has no API credentials and must keep working
// exactly as before, through the CLI.
func TestChooseTransport_FallsBackWithoutCredentials(t *testing.T) {
	envOf := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	if got := chooseTransport(envOf(map[string]string{"ANTHROPIC_API_KEY": "sk"})); got != transportDirect {
		t.Error("an API key should select the direct transport")
	}
	if got := chooseTransport(envOf(map[string]string{"ANTHROPIC_AUTH_TOKEN": "t"})); got != transportDirect {
		t.Error("a bearer token should select the direct transport")
	}
	if got := chooseTransport(envOf(nil)); got != transportCLI {
		t.Error("no credentials should fall back to the CLI transport")
	}
}

// TestRun_UsesDirectWhenCredentialsExist drives the exported entry point, so
// the dispatch and the base-URL override are covered together rather than only
// the internals.
func TestRun_UsesDirectWhenCredentialsExist(t *testing.T) {
	var got captured
	srv := fakeAPI(t, http.StatusOK, okReply, &got)

	old := osEnv
	t.Cleanup(func() { osEnv = old })
	osEnv = func(k string) string {
		switch k {
		case "ANTHROPIC_API_KEY":
			return "sk-test"
		case "ANTHROPIC_BASE_URL":
			return srv.URL
		}
		return ""
	}

	out, use, err := Run(context.Background(), fileMapReq())
	if err != nil {
		t.Fatal(err)
	}
	if got.headers.Get("x-api-key") != "sk-test" {
		t.Error("Run did not take the direct transport")
	}
	if len(out) == 0 {
		t.Error("no digest returned")
	}
	// Cost is computed rather than server-reported on this transport: 5,000
	// input at $1/M plus 400 output at $5/M.
	want := (5000*1.00 + 400*5.00) / 1e6
	if use.CostUSD < want*0.999 || use.CostUSD > want*1.001 {
		t.Errorf("CostUSD = %v, want ~%v", use.CostUSD, want)
	}
}

func TestBaseURL(t *testing.T) {
	envOf := func(v string) func(string) string {
		return func(k string) string {
			if k == "ANTHROPIC_BASE_URL" {
				return v
			}
			return ""
		}
	}
	if got := baseURL(envOf("")); got != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", got, defaultBaseURL)
	}
	// A trailing slash would otherwise produce //v1/messages.
	if got := baseURL(envOf("https://gw.example.com/")); got != "https://gw.example.com" {
		t.Errorf("baseURL = %q, trailing slash not trimmed", got)
	}
}

func TestEstimateCostUSD(t *testing.T) {
	u := Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	if got := estimateCostUSD("claude-haiku-4-5-20251001", u); got != 6.00 {
		t.Errorf("cost = %v, want 6.00 ($1 in + $5 out per Mtok)", got)
	}
	// An unknown model yields zero rather than a guess; skim stats reports that
	// as "n of m calls reported cost" instead of presenting it as free.
	if got := estimateCostUSD("some-future-model", u); got != 0 {
		t.Errorf("unknown model cost = %v, want 0", got)
	}
	// Cache tiers are priced off the input rate, not at parity with it.
	c := Usage{CacheWriteTokens: 1_000_000, CacheReadTokens: 1_000_000}
	want := 1.00*cacheWriteMultiplier + 1.00*cacheReadMultiplier
	if got := estimateCostUSD("claude-haiku-4-5", c); got < want-1e-9 || got > want+1e-9 {
		t.Errorf("cache cost = %v, want %v", got, want)
	}
}
