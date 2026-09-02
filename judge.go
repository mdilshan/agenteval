package agenteval

import "context"

// Verdict is one criterion's judged outcome.
type Verdict struct {
	Pass   bool
	Reason string
}

// Judge evaluates a single natural-language pass/fail criterion against
// an agent reply. It is pluggable so a host reuses its own LLM client;
// the framework core imports no LLM SDK. A nil Judge disables the judged
// layer (deterministic-only mode — no API key needed).
//
// Criterion is a positive statement the reply must satisfy (e.g. "Tells
// the customer a person will call shortly"). Transcript is the rendered
// conversation for context; reply is the text under test.
type Judge interface {
	Evaluate(
		ctx context.Context, criterion, transcript, reply string,
	) (Verdict, error)
}
