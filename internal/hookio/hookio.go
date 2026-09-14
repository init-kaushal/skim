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

type GrepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	OutputMode string `json:"output_mode"`
	Glob       string `json:"glob"`
	Type       string `json:"type"`
	// HeadLimit is Grep's own cap on returned lines. Absent means unbounded.
	HeadLimit int `json:"head_limit"`
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
