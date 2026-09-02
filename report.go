package agenteval

import (
	"fmt"
	"strings"
)

// CaseReport is the outcome of one case: the agent's reply, any run
// error, and every evaluated Check.
type CaseReport struct {
	Suite  string  `json:"suite"`
	Case   string  `json:"case"`
	Reply  string  `json:"reply"`
	Err    string  `json:"error,omitempty"`
	Checks []Check `json:"checks"`
}

// Passed reports whether the case ran and every non-skipped check
// passed.
func (c CaseReport) Passed() bool {
	if c.Err != "" {
		return false
	}
	for _, ch := range c.Checks {
		if !ch.Pass && !ch.Skipped {
			return false
		}
	}
	return true
}

// Report is the full run outcome.
type Report struct {
	Cases []CaseReport `json:"cases"`
}

// Passed reports whether every case passed.
func (r Report) Passed() bool {
	for _, c := range r.Cases {
		if !c.Passed() {
			return false
		}
	}
	return true
}

// Counts returns total and passed case counts.
func (r Report) Counts() (total, passed int) {
	total = len(r.Cases)
	for _, c := range r.Cases {
		if c.Passed() {
			passed++
		}
	}
	return total, passed
}

// String renders a human-readable summary: a line per case, with the
// failing checks indented beneath.
func (r Report) String() string {
	var b strings.Builder
	total, passed := r.Counts()
	fmt.Fprintf(&b, "agenteval: %d/%d cases passed\n", passed, total)
	for _, c := range r.Cases {
		status := "PASS"
		if !c.Passed() {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "  [%s] %s / %s\n", status, c.Suite, c.Case)
		if c.Err != "" {
			fmt.Fprintf(&b, "      error: %s\n", c.Err)
		}
		for _, ch := range c.Checks {
			if ch.Pass || ch.Skipped {
				continue
			}
			fmt.Fprintf(&b, "      x %s %s: %s\n",
				ch.Kind, ch.Detail, ch.Reason)
		}
	}
	return b.String()
}
