// Package agenteval behaviorally regression-tests LLM agents. Instead of
// asserting exact output (impossible for a non-deterministic model), it
// runs a frozen conversation through a real agent and asserts what the
// agent DID: which tools it called, how it classified the turn, and —
// for prose-only checks — an LLM-as-judge verdict against explicit
// pass/fail criteria.
//
// The framework knows nothing about any specific agent. A host
// implements the single Agent interface (an adapter over its engine) and
// the framework does the rest: load fixtures, run, evaluate, report.
package agenteval

import (
	"context"
	"encoding/json"
)

// Turn is one message of prior conversation.
type Turn struct {
	Role string `json:"role"` // "user" | "assistant"
	Text string `json:"text"`
}

// ToolCall is one tool the agent invoked this turn — the primary
// observable action asserted against.
type ToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// RunInput is one scenario handed to the agent. Setup is the suite's
// optional, host-opaque configuration; the framework never reads it —
// the adapter uses it to construct the agent for this scenario.
type RunInput struct {
	History []Turn
	Message string
	Setup   map[string]any
}

// Result is the agent's normalized output for one turn. ToolCalls and
// Meta are optional but are where most assertions land; Meta carries
// classification flags and anything else worth checking.
type Result struct {
	Reply     string
	ToolCalls []ToolCall
	Meta      map[string]any
}

// Agent is the only thing a host must implement: run one turn and report
// what the agent did.
type Agent interface {
	Run(ctx context.Context, in RunInput) (Result, error)
}

// hasTool reports whether the result called a tool by name.
func (r Result) hasTool(name string) bool {
	for _, tc := range r.ToolCalls {
		if tc.Name == name {
			return true
		}
	}
	return false
}
