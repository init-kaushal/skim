package hookio

import (
	"encoding/json"
	"io"
)

type Input struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	CWD           string          `json:"cwd"`
}

func Parse(r io.Reader) (Input, error) {
	var in Input
	dec := json.NewDecoder(r)
	if err := dec.Decode(&in); err != nil {
		return Input{}, err
	}
	return in, nil
}

type ReadInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

type BashInput struct {
	Command string `json:"command"`
}

// GrepInput mirrors the Grep tool's parameters. The hyphenated key names are
// not a typo — they are literally what Claude Code puts on the hook's stdin,
// confirmed by capturing real PreToolUse payloads from a live session:
//
//	{"pattern":"todo","path":".","output_mode":"content","-i":true,"-A":2}
//	{"pattern":"func","path":".","output_mode":"content","-B":1,"head_limit":5}
//	{"pattern":"type .* struct","path":".","output_mode":"content","multiline":true}
//
// Every field that changes the result set has to be parsed, or skim decides
// whether to intercept by running a *different query* than the one the model
// asked for: ignoring `-i` on a case-insensitive search under-counted matches
// 3-to-1 in a direct test, so a call that warranted interception passed
// straight through (and the reverse for patterns that only match one case).
type GrepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	OutputMode string `json:"output_mode"`
	Glob       string `json:"glob"`
	Type       string `json:"type"`
	// HeadLimit is Grep's own cap on returned lines. Absent means unbounded.
	HeadLimit int `json:"head_limit"`

	// CaseInsensitive and Multiline change which lines match at all, so they
	// must reach the counting pass.
	CaseInsensitive bool `json:"-i"`
	Multiline       bool `json:"multiline"`

	// After, Before and Context add surrounding lines. They do not change the
	// match count, but they do change how much output the call would have
	// returned — which is the baseline the saving is measured against.
	After   int `json:"-A"`
	Before  int `json:"-B"`
	Context int `json:"-C"`

	// LineNumbers only affects formatting, and is parsed so the sample pass can
	// reproduce the call's real output size.
	LineNumbers bool `json:"-n"`
}

func (i Input) Read() (ReadInput, error) {
	var x ReadInput
	return x, json.Unmarshal(i.ToolInput, &x)
}

func (i Input) Bash() (BashInput, error) {
	var x BashInput
	return x, json.Unmarshal(i.ToolInput, &x)
}

func (i Input) Grep() (GrepInput, error) {
	var x GrepInput
	return x, json.Unmarshal(i.ToolInput, &x)
}

func Allow(w io.Writer) error {
	return nil
}

type decision struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

func Deny(w io.Writer, reason string) error {
	b, err := json.Marshal(decision{hookSpecificOutput{
		HookEventName:            "PreToolUse",
		PermissionDecision:       "deny",
		PermissionDecisionReason: reason,
	}})
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}
