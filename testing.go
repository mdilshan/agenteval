package agenteval

import (
	"context"
	"testing"
)

// RunT loads the suites in dir, runs them through the agent, and fails
// the test on any case that misses its spec. Each case becomes a
// subtest, so `go test -run` can target one. Deterministic checks always
// run; judged criteria run when opts.Judge is set (otherwise skipped).
func RunT(t *testing.T, a Agent, dir string, opts Options) {
	t.Helper()
	suites, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("agenteval: %v", err)
	}
	rep := Run(context.Background(), a, suites, opts)
	for _, c := range rep.Cases {
		c := c
		t.Run(c.Suite+"/"+c.Case, func(t *testing.T) {
			if c.Err != "" {
				t.Fatalf("agent error: %s", c.Err)
			}
			for _, ch := range c.Checks {
				if ch.Pass || ch.Skipped {
					continue
				}
				t.Errorf("%s %s: %s", ch.Kind, ch.Detail, ch.Reason)
			}
		})
	}
}
