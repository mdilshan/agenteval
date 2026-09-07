package agenteval_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mdilshan/agenteval"
)

// myLLM adapts whatever client you already have onto agenteval.Model.
// That is the entire integration: one method. Configure it for
// deterministic output, and return an error rather than empty text when
// the call fails — the judge treats an error as "no verdict" and never as
// the agent having misbehaved.
type myLLM struct { /* your client */
}

func (m myLLM) Complete(
	ctx context.Context, req agenteval.Request,
) (agenteval.Response, error) {
	// out, err := m.client.Chat(ctx, req.System, req.User)
	// if err != nil {
	//     return agenteval.Response{}, err
	// }
	// return agenteval.Response{
	//     Text:  out.Text,
	//     Usage: agenteval.Usage{PromptTokens: out.In, CompletionTokens: out.Out},
	// }, nil

	// Stubbed so this file runs offline: a real model decides these.
	if strings.Contains(req.User, "price") {
		return agenteval.Response{
			Text: `{"reason":"no amount appears","pass":false}`}, nil
	}
	return agenteval.Response{Text: `{"reason":"stub","pass":true}`}, nil
}

// agentResult stands in for whatever your agent returns.
type agentResult struct {
	Reply string
	Tools []agenteval.ToolCall
}

func runMyAgent(_ context.Context, _ string) agentResult {
	return agentResult{
		Reply: "A team member will call you shortly.",
		Tools: []agenteval.ToolCall{{
			Name: "escalate_to_human",
			Args: json.RawMessage(`{"reason":"call_asap"}`),
		}},
	}
}

// A judged test: everything mechanical stays plain Go, and the judge is
// used only for the claims Go cannot express.
func TestExample_JudgedTurn(t *testing.T) {
	j := agenteval.New(myLLM{})

	msg := "can you call me now?"
	res := runMyAgent(t.Context(), msg)

	// Deterministic checks: no framework, no tokens, no API key.
	if !slices.ContainsFunc(res.Tools, func(tc agenteval.ToolCall) bool {
		return tc.Name == "escalate_to_human"
	}) {
		t.Error("expected an escalation")
	}

	// Prose-only checks: this is what the judge is for.
	turn := agenteval.Turn{
		Input:     msg,
		Reply:     res.Reply,
		ToolCalls: res.Tools,
	}
	j.AssertAll(t, turn,
		"Tells the customer a person will contact them shortly",
		"Replies in the same language the customer used",
	)
	// Prefer refuting a positive statement over asserting a negation —
	// models judge "States a price" far more reliably than "Does not
	// state a price".
	j.Refute(t, "States a specific price or monetary amount", turn)

	t.Logf("judging cost %d tokens", j.Usage().Total())
}

// Check returns the verdict instead of failing the test, so you can
// branch on it. Here a table of turns is scored rather than gated: one
// weak reply should not stop the rest from being measured.
func TestExample_CheckScoresATable(t *testing.T) {
	j := agenteval.New(myLLM{})

	const criterion = "Tells the customer a person will contact them shortly"
	cases := []struct{ name, message string }{
		{"asap", "can you call me now?"},
		{"scheduled", "can we set up a call sometime?"},
	}

	passed := 0
	for _, tc := range cases {
		res := runMyAgent(t.Context(), tc.message)
		v, err := j.Check(t.Context(), criterion, agenteval.Turn{
			Input:     tc.message,
			Reply:     res.Reply,
			ToolCalls: res.Tools,
		})
		if err != nil {
			// The judge broke — that is not a verdict about the agent,
			// so it is a test failure in its own right.
			t.Fatalf("judge unavailable on %q: %v", tc.name, err)
		}
		if v.Pass {
			passed++
		} else {
			t.Logf("%s: %s", tc.name, v.Reason)
		}
	}

	// Gate on the rate, not on any single turn.
	if passed < len(cases) {
		t.Errorf("%d/%d turns met the criterion", passed, len(cases))
	}
}

// Assert on its own, for a single criterion.
func TestExample_SingleAssert(t *testing.T) {
	j := agenteval.New(myLLM{})

	msg := "can you call me now?"
	res := runMyAgent(t.Context(), msg)

	j.Assert(t, "Acknowledges the customer's request for a call",
		agenteval.Turn{Input: msg, Reply: res.Reply, ToolCalls: res.Tools})
}

// A multi-step run: one user message, a loop of tool calls, one reply.
// The trace is what makes the loop judgeable — order, results, failures.
func TestExample_MultiStepRun(t *testing.T) {
	j := agenteval.New(myLLM{})

	msg := "book me in for tomorrow"
	turn := agenteval.Turn{
		Input: msg,
		Reply: "You're booked for 14:00 tomorrow.",
		// Recorded in the order the agent took them, as your loop ran.
		ToolCalls: []agenteval.ToolCall{
			{
				Name:   "check_availability",
				Args:   json.RawMessage(`{"date":"2026-09-04"}`),
				Result: `{"slots":["10:00","14:00"]}`,
			},
			{
				Name: "book_appointment",
				Args: json.RawMessage(`{"slot":"10:00"}`),
				Err:  "slot no longer available",
			},
			{
				Name:   "book_appointment",
				Args:   json.RawMessage(`{"slot":"14:00"}`),
				Result: `{"id":"bk_123"}`,
			},
		},
	}

	// Criteria about the loop, not just the reply. None of these are
	// decidable from a flat list of tool names.
	j.AssertAll(t, turn,
		"Checked availability before confirming a slot",
		"Recovered after the first booking attempt failed",
		"Confirmed a time that the availability check actually offered",
	)
}

// Recording the trace is the only integration work required: append in
// whatever path already dispatches your tools.
type trace []agenteval.ToolCall

func (tr *trace) record(name string, args json.RawMessage, result string, err error) {
	tc := agenteval.ToolCall{Name: name, Args: args, Result: result}
	if err != nil {
		tc.Err = err.Error()
	}
	*tr = append(*tr, tc)
}

func TestExample_RecordingATrace(t *testing.T) {
	var tr trace
	tr.record("check_availability", json.RawMessage(`{}`), `{"slots":[]}`, nil)
	tr.record("book_appointment", nil, "", errNoSlots)

	j := agenteval.New(myLLM{})
	j.Assert(t, "Told the customer no slots were available",
		agenteval.Turn{
			Input:     "book me in",
			Reply:     "Sorry — there's nothing free tomorrow.",
			ToolCalls: tr,
		})
}

var errNoSlots = errors.New("no slots")
