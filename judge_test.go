package agenteval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// stubModel returns a scripted sequence of replies/errors, one per call,
// and records the requests it saw.
type stubModel struct {
	mu      sync.Mutex
	replies []string
	errs    []error
	usage   Usage
	seen    []Request
	n       int
}

func (m *stubModel) Complete(
	_ context.Context, req Request,
) (Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.n
	m.n++
	m.seen = append(m.seen, req)
	if i < len(m.errs) && m.errs[i] != nil {
		return Response{}, m.errs[i]
	}
	if i < len(m.replies) {
		return Response{Text: m.replies[i], Usage: m.usage}, nil
	}
	return Response{Text: `{"reason":"ok","pass":true}`, Usage: m.usage}, nil
}

// fakeTB captures what the assertion helpers report, so the helpers
// themselves can be tested. It satisfies TB.
type fakeTB struct {
	ctx    context.Context
	errs   []string
	helper int
}

func newFakeTB() *fakeTB { return &fakeTB{ctx: context.Background()} }

func (f *fakeTB) Helper()                  { f.helper++ }
func (f *fakeTB) Context() context.Context { return f.ctx }
func (f *fakeTB) Errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}
func (f *fakeTB) failed() bool { return len(f.errs) > 0 }
func (f *fakeTB) log() string  { return strings.Join(f.errs, "\n") }

var _ TB = (*fakeTB)(nil)

// *testing.T must satisfy TB, or callers cannot pass a plain t.
var _ TB = (*testing.T)(nil)

func turn() Turn {
	return Turn{Input: "call me", Reply: "A team member will call you."}
}

func TestCheck_PassAndReason(t *testing.T) {
	m := &stubModel{
		replies: []string{`{"reason":"promises a callback","pass":true}`},
		usage:   Usage{PromptTokens: 100, CompletionTokens: 20},
	}
	v, err := New(m).Check(context.Background(), "crit", turn())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !v.Pass {
		t.Error("want pass")
	}
	if v.Reason != "promises a callback" {
		t.Errorf("reason = %q", v.Reason)
	}
	if v.Usage.Total() != 120 {
		t.Errorf("usage = %d, want 120", v.Usage.Total())
	}
}

func TestCheck_FailCarriesReason(t *testing.T) {
	m := &stubModel{
		replies: []string{`{"reason":"never mentions a call","pass":false}`}}
	v, err := New(m).Check(context.Background(), "crit", turn())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if v.Pass {
		t.Error("want fail")
	}
	if v.Reason != "never mentions a call" {
		t.Errorf("a failure must carry the judge's reason, got %q",
			v.Reason)
	}
}

func TestCheck_EmptyCriterionIsAnError(t *testing.T) {
	if _, err := New(&stubModel{}).Check(
		context.Background(), "", turn()); err == nil {
		t.Fatal("an empty criterion must be an error")
	}
}

// The whole trustworthiness of the suite rests on this: a broken judge is
// an error, never a failing verdict.
func TestCheck_ModelFailureIsAnErrorNotAFailedVerdict(t *testing.T) {
	for _, tc := range []struct {
		name  string
		model *stubModel
	}{
		{"transport error", &stubModel{errs: []error{errors.New("503")}}},
		{"no json", &stubModel{replies: []string{"I think it passes"}}},
		{"missing pass field",
			&stubModel{replies: []string{`{"reason":"fine"}`}}},
		{"malformed json",
			&stubModel{replies: []string{`{"pass":`}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := New(tc.model).Check(
				context.Background(), "crit", turn())
			if err == nil {
				t.Fatalf("want an error, got verdict %+v", v)
			}
			if v.Pass {
				t.Error("a failed judge must not report a pass")
			}
		})
	}
}

func TestCheck_MajorityVote(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replies []string
		want    bool
	}{
		{"unanimous pass", []string{p(true), p(true), p(true)}, true},
		{"majority pass", []string{p(true), p(true), p(false)}, true},
		{"majority fail", []string{p(true), p(false), p(false)}, false},
		{"tie fails", []string{p(true), p(false)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &stubModel{replies: tc.replies}
			v, err := New(m, WithSamples(len(tc.replies))).Check(
				context.Background(), "crit", turn())
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if v.Pass != tc.want {
				t.Errorf("pass = %v, want %v (%s)",
					v.Pass, tc.want, v.Reason)
			}
		})
	}
}

// A flaky provider must not read as an agent regression: errored samples
// are not failing votes.
func TestCheck_ErroredSamplesAreNotFailingVotes(t *testing.T) {
	m := &stubModel{
		replies: []string{p(true)},
		errs:    []error{nil, errors.New("429"), errors.New("timeout")},
	}
	v, err := New(m, WithSamples(3)).Check(
		context.Background(), "crit", turn())
	if err != nil {
		t.Fatalf("1 pass + 2 errors should still yield a verdict: %v", err)
	}
	if !v.Pass {
		t.Errorf("want pass, got fail: %s", v.Reason)
	}
}

func TestCheck_SplitVoteReportsTally(t *testing.T) {
	m := &stubModel{replies: []string{p(true), p(false), p(false)}}
	v, _ := New(m, WithSamples(3)).Check(
		context.Background(), "crit", turn())
	if !strings.Contains(v.Reason, "1/3 samples passed") {
		t.Errorf("a split vote should report its tally, got %q", v.Reason)
	}
}

func TestCheck_CancelledContext(t *testing.T) {
	m := &stubModel{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(m).Check(ctx, "crit", turn()); err == nil {
		t.Fatal("want an error from the cancelled context")
	}
	if m.n != 0 {
		t.Errorf("model called %d times under a cancelled context", m.n)
	}
}

func TestJudge_UsageAccumulates(t *testing.T) {
	m := &stubModel{usage: Usage{PromptTokens: 10, CompletionTokens: 5}}
	j := New(m)
	for i := 0; i < 3; i++ {
		if _, err := j.Check(
			context.Background(), "crit", turn()); err != nil {
			t.Fatalf("Check: %v", err)
		}
	}
	if got := j.Usage().Total(); got != 45 {
		t.Errorf("total usage = %d, want 45", got)
	}
}

func TestJudge_ConcurrentUse(t *testing.T) {
	m := &stubModel{usage: Usage{PromptTokens: 1}}
	j := New(m)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = j.Check(context.Background(), "crit", turn())
		}()
	}
	wg.Wait()
	if got := j.Usage().PromptTokens; got != 20 {
		t.Errorf("usage = %d, want 20", got)
	}
}

func TestAssert(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reply    string
		wantFail bool
	}{
		{"passing criterion", p(true), false},
		{"failing criterion", p(false), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTB()
			ok := New(&stubModel{replies: []string{tc.reply}}).
				Assert(f, "crit", turn())
			if f.failed() == !tc.wantFail {
				t.Errorf("failed = %v, want %v (%s)",
					f.failed(), tc.wantFail, f.log())
			}
			if ok == tc.wantFail {
				t.Errorf("Assert returned %v", ok)
			}
		})
	}
}

// Refute inverts: the criterion must NOT hold.
func TestRefute(t *testing.T) {
	f := newFakeTB()
	if !New(&stubModel{replies: []string{p(false)}}).
		Refute(f, "States a price", turn()) {
		t.Errorf("a false criterion must satisfy Refute: %s", f.log())
	}

	f2 := newFakeTB()
	if New(&stubModel{replies: []string{p(true)}}).
		Refute(f2, "States a price", turn()) {
		t.Error("a true criterion must fail Refute")
	}
	if !strings.Contains(f2.log(), "should not have") {
		t.Errorf("the message should read as a refutation: %s", f2.log())
	}
}

// A suite whose judge is down must go red, not silently green.
func TestAssert_JudgeUnavailableFailsTheTest(t *testing.T) {
	f := newFakeTB()
	ok := New(&stubModel{errs: []error{errors.New("no route to host")}}).
		Assert(f, "crit", turn())
	if ok || !f.failed() {
		t.Fatal("an unreachable judge must fail the test")
	}
	if !strings.Contains(f.log(), "judge unavailable") {
		t.Errorf("the failure must name the judge, not the agent: %s",
			f.log())
	}
}

// Every criterion reports independently, so one run shows them all.
func TestAssertAll_ReportsEveryFailure(t *testing.T) {
	f := newFakeTB()
	m := &stubModel{replies: []string{p(false), p(false), p(true)}}
	New(m).AssertAll(f, turn(), "a", "b", "c")
	if len(f.errs) != 2 {
		t.Errorf("got %d failures, want 2:\n%s", len(f.errs), f.log())
	}
}

func TestWithSystemPrompt(t *testing.T) {
	m := &stubModel{}
	if _, err := New(m, WithSystemPrompt("custom")).Check(
		context.Background(), "crit", turn()); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if m.seen[0].System != "custom" {
		t.Errorf("System = %q", m.seen[0].System)
	}
}

func TestOptions_IgnoreNonsense(t *testing.T) {
	j := New(&stubModel{}, WithSamples(0), WithSystemPrompt(""))
	if j.samples != 1 {
		t.Errorf("samples = %d, want the default of 1", j.samples)
	}
	if j.system != systemPrompt {
		t.Error("an empty system prompt must not clear the default")
	}
}

// p is a scripted verdict reply.
func p(pass bool) string {
	if pass {
		return `{"reason":"r","pass":true}`
	}
	return `{"reason":"r","pass":false}`
}
