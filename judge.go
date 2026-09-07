package agenteval

import (
	"context"
	"fmt"
	"sync"
)

// TB is the slice of *testing.T that the assertion helpers need.
// *testing.T, *testing.B and *testing.F all satisfy it, so you pass a
// plain t. It is declared here rather than using testing.TB so this
// package imports no testing machinery — and so its own helpers are
// testable.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Context() context.Context
}

// Verdict is one criterion's judged outcome.
type Verdict struct {
	// Pass reports whether the criterion held.
	Pass bool
	// Reason is the judge's justification. This is the whole value of a
	// red eval — a bare false tells you nothing about what to fix.
	Reason string
	// Usage is what the judging cost.
	Usage Usage
}

// Judge evaluates natural-language criteria against an agent turn, using
// any [Model] as its brain. It is safe for concurrent use.
type Judge struct {
	model   Model
	samples int
	system  string

	mu    sync.Mutex
	usage Usage
}

// Option configures a [Judge].
type Option func(*Judge)

// WithSamples judges each criterion n times and takes the majority of the
// samples that returned a verdict. Use it for criteria you have seen flip.
//
// It only helps if the Model produces variation: at temperature 0 the
// samples are near-identical and you are paying n times for one opinion.
// Raise the Model's temperature if you turn this up.
func WithSamples(n int) Option {
	return func(j *Judge) {
		if n > 0 {
			j.samples = n
		}
	}
}

// WithSystemPrompt replaces the built-in judging instructions. The
// default is tuned to produce binary, criterion-scoped, evidence-bound
// verdicts; change it only if you know what you are trading away.
func WithSystemPrompt(s string) Option {
	return func(j *Judge) {
		if s != "" {
			j.system = s
		}
	}
}

// New builds a Judge over the given brain.
func New(m Model, opts ...Option) *Judge {
	j := &Judge{model: m, samples: 1, system: systemPrompt}
	for _, o := range opts {
		o(j)
	}
	return j
}

// Usage reports the tokens this Judge has spent so far, across every
// call. Print it at the end of a run to see what the suite costs.
func (j *Judge) Usage() Usage {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.usage
}

// Check evaluates one criterion and returns the verdict. Use it when you
// want to inspect the result; use [Judge.Assert] in a test.
//
// The error is reserved for the judge failing — an unreachable provider,
// an unparseable reply. It never means the agent misbehaved; that is a
// Verdict with Pass false.
func (j *Judge) Check(
	ctx context.Context, criterion string, turn Turn,
) (Verdict, error) {
	if criterion == "" {
		return Verdict{}, fmt.Errorf("agenteval: empty criterion")
	}
	req := Request{
		System:     j.system,
		User:       renderPrompt(criterion, turn),
		JSONObject: true,
	}

	var total Usage
	votes, passes := 0, 0
	failReason, passReason := "", ""
	var lastErr error

	for i := 0; i < j.samples; i++ {
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		resp, err := j.model.Complete(ctx, req)
		total.Add(resp.Usage)
		if err != nil {
			lastErr = err
			continue
		}
		v, err := parseVerdict(resp.Text)
		if err != nil {
			lastErr = err
			continue
		}
		votes++
		if v.Pass {
			passes++
			if passReason == "" {
				passReason = v.Reason
			}
		} else if failReason == "" {
			failReason = v.Reason
		}
	}

	j.mu.Lock()
	j.usage.Add(total)
	j.mu.Unlock()

	// Not one usable verdict: that is the judge failing, not the agent.
	// Reporting it as an error rather than Pass:false is what keeps a
	// flaky provider from looking like a behavioral regression.
	if votes == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("no samples were run")
		}
		return Verdict{Usage: total},
			fmt.Errorf("agenteval: judging %q: %w", criterion, lastErr)
	}

	// Majority of the verdicts received; a tie fails, conservatively.
	if passes*2 > votes {
		return Verdict{Pass: true, Reason: passReason, Usage: total}, nil
	}
	reason := failReason
	if reason == "" {
		reason = "criterion not met"
	}
	if votes > 1 {
		reason += fmt.Sprintf(" [%d/%d samples passed]", passes, votes)
	}
	return Verdict{Reason: reason, Usage: total}, nil
}

// Assert fails t unless the criterion holds, and reports the judge's
// reason. It returns whether the criterion held, so you can skip
// follow-up work.
//
// A judge that cannot be reached fails the test too, but says so
// distinctly: a test suite that silently passes when its judge is down is
// worse than no suite. Judging runs on t.Context, so it is cancelled when
// the test finishes.
func (j *Judge) Assert(t TB, criterion string, turn Turn) bool {
	t.Helper()
	return j.assert(t, criterion, turn, true)
}

// Refute fails t unless the criterion is FALSE of the turn.
//
// Prefer it over asserting a negation. Models judge "States a specific
// price" far more reliably than "Does not state a specific price" —
// negation is where LLM judges are weakest, so keep the criterion
// positive and invert here.
func (j *Judge) Refute(t TB, criterion string, turn Turn) bool {
	t.Helper()
	return j.assert(t, criterion, turn, false)
}

// AssertAll asserts every criterion against one turn, reporting each
// independently so a run shows all failures rather than only the first.
func (j *Judge) AssertAll(t TB, turn Turn, criteria ...string) {
	t.Helper()
	for _, c := range criteria {
		j.Assert(t, c, turn)
	}
}

func (j *Judge) assert(
	t TB, criterion string, turn Turn, want bool,
) bool {
	t.Helper()
	v, err := j.Check(t.Context(), criterion, turn)
	if err != nil {
		t.Errorf("judge unavailable: %v", err)
		return false
	}
	if v.Pass == want {
		return true
	}
	if want {
		t.Errorf("criterion not met: %s\n  judge: %s\n  reply: %s",
			criterion, v.Reason, turn.Reply)
	} else {
		t.Errorf("criterion held but should not have: %s\n"+
			"  judge: %s\n  reply: %s", criterion, v.Reason, turn.Reply)
	}
	return false
}
