package agenteval

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden judge input")

// goldenTurn deliberately exercises everything that reaches the judge:
// history, the current input, a reply, an ordered trace with arguments, a
// success, a failure, metadata, and a result long enough to truncate.
func goldenTurn() Turn {
	return Turn{
		History: []Message{
			{Role: RoleUser, Text: "do you do evening slots?"},
			{Role: RoleAssistant, Text: "We do, up to 7pm."},
		},
		Input: "book me the latest one tomorrow",
		Reply: "You're booked for 18:30 tomorrow.",
		ToolCalls: []ToolCall{
			{
				Name:   "check_availability",
				Args:   json.RawMessage(`{"date":  "2026-09-04"}`),
				Result: `{"slots":["17:00","18:30"]}`,
			},
			{
				Name: "book_appointment",
				Args: json.RawMessage(`{"slot":"19:30"}`),
				Err:  "outside business hours",
			},
			{
				Name:   "book_appointment",
				Args:   json.RawMessage(`{"slot":"18:30"}`),
				Result: `{"id":"bk_123"}` + strings.Repeat(" pad", 400),
			},
		},
		Meta: map[string]any{"stage": "booked", "language": "en"},
	}
}

// TestJudgeInputIsStable pins the exact bytes the judge receives — the
// system prompt and a fully-exercised rendered turn.
//
// It exists because a change can be perfectly API-compatible, compile
// everywhere, pass every other test, and still move every verdict: the
// prompt wording, the layout of the trace, the truncation limit, the
// section order. SemVer has no vocabulary for that, so this test supplies
// it. A verdict-affecting change cannot be made by accident; it fails here
// until someone acknowledges it.
//
// If this fails, decide which it is:
//
//   - Unintended. Revert it.
//   - Intended. Run `go test ./... -update`, review the diff as carefully
//     as you would a prompt change, and add a "Verdict-affecting" entry to
//     CHANGELOG.md. Ship it in a minor release, never a patch — users read
//     that heading to decide whether to re-baseline their suites.
func TestJudgeInputIsStable(t *testing.T) {
	got := systemPrompt +
		"\n\n===== RENDERED TURN =====\n\n" +
		renderPrompt("Recovered after the first booking attempt failed.",
			goldenTurn())

	path := filepath.Join("testdata", "judge_input.golden")

	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Log("golden updated — review the diff and add a " +
			"\"Verdict-affecting\" changelog entry")
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf(
			"what the judge sees has changed.\n\n"+
				"This is a VERDICT-AFFECTING change: it can move pass "+
				"rates for every user without breaking their build.\n"+
				"If it was intended, run `go test ./... -update`, review "+
				"the diff, and add a \"Verdict-affecting\" entry to "+
				"CHANGELOG.md for the next MINOR release.\n\n"+
				"--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}
