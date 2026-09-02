// Package echo is a runnable example of using agenteval: a trivial
// deterministic agent + a rule-based judge, exercised against the
// fixtures in testdata/. It shows the whole integration in ~50 lines and
// doubles as an end-to-end test of the framework (no LLM required).
package echo

import (
	"context"
	"strings"
	"testing"

	"github.com/mdilshan/agenteval"
)

// echoAgent is a stand-in for a real agent: it maps a message to a
// canned reply + tool calls + metadata, so the framework can be
// exercised without any LLM.
type echoAgent struct{}

func (echoAgent) Run(
	_ context.Context, in agenteval.RunInput,
) (agenteval.Result, error) {
	msg := strings.ToLower(in.Message)
	r := agenteval.Result{Meta: map[string]any{"escalated": false}}
	switch {
	case strings.Contains(msg, "call now"),
		strings.Contains(msg, "call?"):
		r.Reply = "A team member will call you shortly."
		r.ToolCalls = []agenteval.ToolCall{{Name: "escalate_to_human"}}
		r.Meta["escalated"] = true
	case strings.Contains(msg, "book"):
		r.Reply = "Sure — what time suits you?"
		r.ToolCalls = []agenteval.ToolCall{{Name: "check_availability"}}
	default:
		r.Reply = "How can I help?"
	}
	return r, nil
}

// keywordJudge is a rule-based stand-in Judge (no LLM) so the judged
// layer runs deterministically in the example.
type keywordJudge struct{}

func (keywordJudge) Evaluate(
	_ context.Context, criterion, _, reply string,
) (agenteval.Verdict, error) {
	if strings.Contains(strings.ToLower(criterion), "call") {
		ok := strings.Contains(strings.ToLower(reply), "team member")
		reason := ""
		if !ok {
			reason = "no mention of a call-back"
		}
		return agenteval.Verdict{Pass: ok, Reason: reason}, nil
	}
	return agenteval.Verdict{Pass: true}, nil
}

// End-to-end over testdata/, including the judged layer.
func TestExample(t *testing.T) {
	agenteval.RunT(t, echoAgent{}, "testdata", agenteval.Options{
		Judge: keywordJudge{},
	})
}

// A nil Judge skips judged criteria without failing the case.
func TestNoJudgeSkips(t *testing.T) {
	suites, err := agenteval.LoadDir("testdata")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rep := agenteval.Run(
		context.Background(), echoAgent{}, suites, agenteval.Options{})
	if !rep.Passed() {
		t.Fatalf("deterministic-only run should pass:\n%s", rep)
	}
}

// The matchers must catch a misbehaving agent.
func TestCatchesRegression(t *testing.T) {
	suites, _ := agenteval.LoadDir("testdata")
	rep := agenteval.Run(context.Background(), brokenAgent{}, suites,
		agenteval.Options{Judge: keywordJudge{}})
	if rep.Passed() {
		t.Fatalf("expected failures from a broken agent")
	}
}

// brokenAgent never escalates and never calls tools.
type brokenAgent struct{}

func (brokenAgent) Run(
	_ context.Context, _ agenteval.RunInput,
) (agenteval.Result, error) {
	return agenteval.Result{
		Reply: "ok", Meta: map[string]any{"escalated": false},
	}, nil
}
