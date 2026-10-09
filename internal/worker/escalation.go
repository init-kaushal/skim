package worker

import "encoding/json"

// escalationProbe is the JSON shape the worker emits when it cannot produce a
// confident summary. The full response is exactly {"escalate": true} with no
// other fields — the worker is instructed to output nothing else when signaling.
type escalationProbe struct {
	Escalate bool `json:"escalate"`
}

// IsEscalation reports whether raw is an escalation signal from the worker.
// Any valid JSON object with "escalate": true is treated as a signal regardless
// of other fields, because the worker prompt says to output only that object.
func IsEscalation(raw []byte) bool {
	var p escalationProbe
	return json.Unmarshal(raw, &p) == nil && p.Escalate
}

// escalationInstruction is appended to every worker prompt that supports
// escalation. It teaches the worker to signal when it is uncertain rather than
// guessing and producing a low-quality digest.
const escalationInstruction = `

ESCALATION: If the content is highly specialized or domain-specific in a way
that makes you uncertain you can produce an accurate structural summary (for
example: advanced cryptographic algorithms, novel DSP processing, architecture-
specific assembler, or heavily obfuscated code), respond with ONLY:
{"escalate": true}
Do not include any other text or JSON fields when signaling escalation.`
