package agenteval

import (
	"context"
	"strings"
)

// Options tunes a run.
type Options struct {
	// Judge evaluates prose-only criteria. Nil disables the judged layer
	// (deterministic-only; those criteria are recorded as skipped).
	Judge Judge
	// JudgeSamples runs each judged criterion N times and takes the
	// majority (to damp judge flakiness). <=1 means a single call.
	JudgeSamples int
}

// Run executes every case in suites through the agent and evaluates its
// Expect. Deterministic checks always run; judged criteria run only when
// opts.Judge is set.
func Run(
	ctx context.Context, a Agent, suites []Suite, opts Options,
) Report {
	var rep Report
	for _, s := range suites {
		for _, c := range s.Cases {
			rep.Cases = append(rep.Cases, runCase(ctx, a, s, c, opts))
		}
	}
	return rep
}

// RunDir loads suites from dir and runs them.
func RunDir(
	ctx context.Context, a Agent, dir string, opts Options,
) (Report, error) {
	suites, err := LoadDir(dir)
	if err != nil {
		return Report{}, err
	}
	return Run(ctx, a, suites, opts), nil
}

func runCase(
	ctx context.Context, a Agent, s Suite, c Case, opts Options,
) CaseReport {
	cr := CaseReport{Suite: s.Name, Case: c.Name}
	res, err := a.Run(ctx, RunInput{
		History: c.History,
		Message: c.Message,
		Setup:   s.Setup,
	})
	if err != nil {
		cr.Err = err.Error()
		return cr
	}
	cr.Reply = res.Reply
	cr.Checks = evalDeterministic(c.Expect, res)

	if !c.Expect.hasJudge() {
		return cr
	}
	transcript := renderTranscript(c.History, c.Message)
	for _, crit := range c.Expect.Judge {
		if opts.Judge == nil {
			cr.Checks = append(cr.Checks, Check{
				Kind: "judge", Detail: crit, Pass: true,
				Skipped: true, Reason: "no judge configured",
			})
			continue
		}
		cr.Checks = append(cr.Checks,
			judgeCriterion(ctx, opts, crit, transcript, res.Reply))
	}
	return cr
}

// judgeCriterion evaluates one criterion, taking a majority over
// JudgeSamples calls. A judge that errors on every sample fails the
// check with the error reason.
func judgeCriterion(
	ctx context.Context, opts Options,
	criterion, transcript, reply string,
) Check {
	n := opts.JudgeSamples
	if n < 1 {
		n = 1
	}
	passes, failReason, lastErr := 0, "", error(nil)
	for i := 0; i < n; i++ {
		v, err := opts.Judge.Evaluate(ctx, criterion, transcript, reply)
		if err != nil {
			lastErr = err
			continue
		}
		if v.Pass {
			passes++
		} else if v.Reason != "" {
			failReason = v.Reason
		}
	}
	if passes == 0 && lastErr != nil {
		return Check{
			Kind: "judge", Detail: criterion, Pass: false,
			Reason: "judge error: " + lastErr.Error(),
		}
	}
	ok := passes*2 > n
	reason := ""
	if !ok {
		reason = failReason
		if reason == "" {
			reason = "criterion not met"
		}
	}
	return Check{Kind: "judge", Detail: criterion, Pass: ok, Reason: reason}
}

// renderTranscript compacts the prior turns plus the current message
// into a plain block for the judge.
func renderTranscript(history []Turn, message string) string {
	var b strings.Builder
	for _, t := range history {
		who := "Assistant"
		if t.Role == "user" {
			who = "Customer"
		}
		b.WriteString(who)
		b.WriteString(": ")
		b.WriteString(t.Text)
		b.WriteString("\n")
	}
	b.WriteString("Customer: ")
	b.WriteString(message)
	return b.String()
}
