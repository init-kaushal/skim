package worker_test

import (
	"testing"

	"github.com/kaushal/skim/internal/worker"
)

func TestIsEscalation_True(t *testing.T) {
	cases := []string{
		`{"escalate": true}`,
		`{"escalate":true}`,
		`{ "escalate" : true }`,
	}
	for _, c := range cases {
		if !worker.IsEscalation([]byte(c)) {
			t.Errorf("IsEscalation(%q) = false, want true", c)
		}
	}
}

func TestIsEscalation_False(t *testing.T) {
	cases := []string{
		`{"escalate": false}`,
		`{"summary":"ok","map":[{"lines":"1","kind":"x"}]}`,
		`not json at all`,
		``,
	}
	for _, c := range cases {
		if worker.IsEscalation([]byte(c)) {
			t.Errorf("IsEscalation(%q) = true, want false", c)
		}
	}
}

func TestIsEscalation_EscalateTrueWithOtherFields(t *testing.T) {
	// A response with "escalate": true alongside other fields is still treated
	// as an escalation signal — the router instructs the model to output only
	// {"escalate": true}, but we are permissive in parsing to avoid silently
	// treating a malformed escalation as a real result.
	fm := `{"escalate": true, "summary": "partial attempt"}`
	if !worker.IsEscalation([]byte(fm)) {
		t.Error("JSON with escalate:true should be treated as escalation regardless of other fields")
	}
}

func TestIsEscalation_ValidFilemapIsNotEscalation(t *testing.T) {
	fm := `{"summary":"a file","map":[{"lines":"1-10","kind":"code"}],"symbols":["Foo"]}`
	if worker.IsEscalation([]byte(fm)) {
		t.Error("valid FileMap JSON should not be treated as escalation")
	}
}
