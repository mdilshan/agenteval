package agenteval

import (
	"context"
	"strings"
	"testing"
)

// echoAgent is a trivial deterministic stand-in for a real agent, enough
// to exercise the framework without any LLM.
type echoAgent struct{}

func (echoAgent) Run(_ context.Context, in RunInput) (Result, error) {
	msg := strings.ToLower(in.Message)
	r := Result{Meta: map[string]any{"escalated": false}}
	switch {
	case strings.Contains(msg, "call now"), strings.Contains(msg, "call?"):
		r.Reply = "A team member will call you shortly."
		r.ToolCalls = []ToolCall{{Name: "escalate_to_human"}}
		r.Meta["escalated"] = true
	case strings.Contains(msg, "book"):
		r.Reply = "Sure — what time suits you?"
		r.ToolCalls = []ToolCall{{Name: "check_availability"}}
	default:
		r.Reply = "How can I help?"
	}
	return r, nil
}

// keywordJudge is a rule-based stand-in Judge (no LLM) so the judged
// layer is exercised deterministically in tests.
type keywordJudge struct{}

func (keywordJudge) Evaluate(
	_ context.Context, criterion, _, reply string,
) (Verdict, error) {
	if strings.Contains(strings.ToLower(criterion), "call") {
		ok := strings.Contains(strings.ToLower(reply), "team member")
		return Verdict{
			Pass:   ok,
			Reason: reasonIf(!ok, "no mention of a call-back"),
		}, nil
	}
	return Verdict{Pass: true}, nil
}

// End-to-end over testdata/, including the judged layer.
func TestRunT_Example(t *testing.T) {
	RunT(t, echoAgent{}, "testdata", Options{Judge: keywordJudge{}})
}

func TestRun_AllPass(t *testing.T) {
	suites, err := LoadDir("testdata")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rep := Run(context.Background(), echoAgent{}, suites,
		Options{Judge: keywordJudge{}})
	if !rep.Passed() {
		t.Fatalf("expected all pass:\n%s", rep.String())
	}
	if total, passed := rep.Counts(); total != 2 || passed != 2 {
		t.Fatalf("counts = %d/%d, want 2/2", passed, total)
	}
}

// A nil Judge skips judged criteria without failing the case.
func TestRun_NoJudgeSkipsButPasses(t *testing.T) {
	suites, _ := LoadDir("testdata")
	rep := Run(context.Background(), echoAgent{}, suites, Options{})
	if !rep.Passed() {
		t.Fatalf("deterministic-only run should pass:\n%s", rep.String())
	}
	// The judged criterion is recorded as skipped, not failed.
	var skipped int
	for _, c := range rep.Cases {
		for _, ch := range c.Checks {
			if ch.Kind == "judge" && ch.Skipped {
				skipped++
			}
		}
	}
	if skipped != 1 {
		t.Fatalf("expected 1 skipped judge check, got %d", skipped)
	}
}

// Matchers must actually catch a misbehaving agent.
func TestRun_CatchesRegression(t *testing.T) {
	broken := brokenAgent{}
	suites, _ := LoadDir("testdata")
	rep := Run(context.Background(), broken, suites,
		Options{Judge: keywordJudge{}})
	if rep.Passed() {
		t.Fatalf("expected failures from a broken agent")
	}
}

// brokenAgent never escalates and never calls tools.
type brokenAgent struct{}

func (brokenAgent) Run(_ context.Context, _ RunInput) (Result, error) {
	return Result{
		Reply: "ok", Meta: map[string]any{"escalated": false},
	}, nil
}
