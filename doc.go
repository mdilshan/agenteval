// Package agenteval is LLM-as-judge assertions for Go tests.
//
// An LLM agent is non-deterministic: its wording changes on every prompt
// tweak, so you cannot pin it with assert.Equal. Most of what you want to
// check about a turn is still ordinary Go — which tools it called, how it
// classified the input, whether a flag flipped — and plain table-driven
// tests already do that better than any framework could:
//
//	res := myAgent.Handle(ctx, "can you call me now?")
//
//	if !slices.Contains(toolNames(res), "escalate_to_human") {
//	    t.Error("expected an escalation")
//	}
//
// This package is for the remainder: the claims that are only expressible
// in prose, which no amount of Go can assert.
//
//	j := agenteval.New(brain)
//
//	j.Assert(t, "Tells the customer a person will call shortly",
//	    agenteval.Turn{Input: msg, Reply: res.Reply, ToolCalls: res.Tools})
//
// A Judge is one implementation over a pluggable [Model] — the brain — so
// the same judging logic runs on any provider. No provider is bundled:
// [Model] is a single method, so wiring one over your existing client is
// about forty lines. See the package example for one.
//
// # Multi-step runs
//
// An agent run is usually a loop: call a tool, read the result, decide,
// call another, then reply. One [Turn] is one RUN, not one step of its
// loop — a run that calls six tools is a single Turn with six ordered
// entries in ToolCalls, each carrying what came back:
//
//	turn := agenteval.Turn{
//	    Input: msg,
//	    Reply: res.Reply,
//	    ToolCalls: []agenteval.ToolCall{
//	        {Name: "check_availability", Result: `{"slots":["14:00"]}`},
//	        {Name: "book_appointment", Err: "slot no longer available"},
//	        {Name: "book_appointment", Result: `{"id":"bk_123"}`},
//	    },
//	}
//
// The judge is told that the order is significant and that an Err step
// failed, which is what makes the loop itself judgeable — sequence,
// recovery, repetition, and when the agent stopped.
//
// This package never drives your agent: there is no runner and no
// adapter, so there is nothing to pause or resume. Pausing and step
// limits belong to your loop. The upside is that judging mid-run costs
// nothing extra — build a Turn from however much of the trace you have
// and call [Judge.Check] on it.
//
// # Judging is a real model call
//
// Every Assert costs tokens and latency. Keep judged criteria few and
// high-signal, assert everything mechanical in plain Go, and put the
// judged tests behind a build tag so `go test ./...` stays free:
//
//	//go:build eval
//
// # Criteria
//
// A criterion is a positive pass/fail statement about the turn, never a
// score and never "is this good?":
//
//	j.Assert(t, "Asks whether the customer wants a call now or later", turn)
//	j.Refute(t, "States a specific price", turn)
//
// [Judge.Refute] exists because models judge positive statements more
// reliably than negative ones: prefer refuting "States a price" over
// asserting "Does not state a price".
package agenteval
