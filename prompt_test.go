package agenteval

import (
	"encoding/json"
	"strings"
	"testing"
)

// The prompt must carry the agent's ACTIONS, not just its prose — that is
// the whole reason this judges agents rather than chat messages.
func TestRenderPrompt_CarriesActions(t *testing.T) {
	got := renderPrompt("Hands off to a human.", Turn{
		History: []Message{
			{Role: RoleUser, Text: "hi"},
			{Role: RoleAssistant, Text: "hello!"},
		},
		Input: "get me a human",
		Reply: "Connecting you now.",
		ToolCalls: []ToolCall{{
			Name: "escalate_to_human",
			Args: json.RawMessage(`{"priority":  "high"}`),
		}},
		Meta: map[string]any{"escalated": true},
	})

	for _, want := range []string{
		"User: hi",
		"Assistant: hello!",
		"User: get me a human",
		"Connecting you now.",
		"1. escalate_to_human",
		`{"priority":"high"}`, // compacted
		`{"escalated":true}`,
		"Hands off to a human.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, got)
		}
	}
}

// "No tools were called" must be stated, not left to inference from an
// absent section — that silence is exactly what some criteria turn on.
func TestRenderPrompt_StatesAbsences(t *testing.T) {
	got := renderPrompt("crit", Turn{Reply: ""})
	for _, want := range []string{
		"(the agent called no tools)",
		"(the agent replied with nothing)",
		"(not provided)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, got)
		}
	}
}

// The criterion sits last, closest to the answer.
func TestRenderPrompt_CriterionComesLast(t *testing.T) {
	got := renderPrompt("THE CRITERION", Turn{Reply: "hi"})
	if strings.Index(got, "THE CRITERION") < strings.Index(got, "hi") {
		t.Errorf("the criterion should come after the turn:\n%s", got)
	}
}

func TestRenderPrompt_MalformedToolArgsDoNotBreakIt(t *testing.T) {
	got := renderPrompt("crit", Turn{
		Reply:     "x",
		ToolCalls: []ToolCall{{Name: "t", Args: json.RawMessage(`not json`)}},
	})
	if !strings.Contains(got, "1. t") {
		t.Errorf("the tool name must survive bad args:\n%s", got)
	}
}

// A brain that ignores the JSON hint still has to parse.
func TestParseVerdict_Tolerant(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		pass       bool
	}{
		{"plain", `{"reason":"r","pass":true}`, true},
		{"fenced", "```json\n{\"reason\":\"r\",\"pass\":false}\n```", false},
		{"prose wrapped",
			`Verdict: {"reason":"r","pass":true} — hope that helps.`, true},
		{"reversed fields", `{"pass":false,"reason":"r"}`, false},
		{"whitespace", "\n\n  {\"reason\":\"r\",\"pass\":true}  \n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, err := parseVerdict(tc.text)
			if err != nil {
				t.Fatalf("parseVerdict: %v", err)
			}
			if v.Pass != tc.pass {
				t.Errorf("pass = %v, want %v", v.Pass, tc.pass)
			}
		})
	}
}

func TestParseVerdict_Rejects(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"no object", "it passes, I reckon"},
		{"no pass field", `{"reason":"looks fine"}`},
		{"malformed", `{"reason":"x","pass":`},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if v, err := parseVerdict(tc.text); err == nil {
				t.Fatalf("want an error, got %+v", v)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("  hello  ", 100); got != "hello" {
		t.Errorf("truncate = %q", got)
	}
	got := truncate(strings.Repeat("x", 300), 10)
	if !strings.HasPrefix(got, strings.Repeat("x", 10)) {
		t.Errorf("truncate = %q", got)
	}
	// The cut must be marked, or the judge reads a truncated result as
	// the whole of what the agent saw.
	if !strings.Contains(got, "truncated") {
		t.Errorf("truncation must be marked, got %q", got)
	}
}

// An agent run is a loop. The judge must see the steps in order, with
// what each returned, or no criterion about sequence, recovery or
// repetition is decidable.
func TestRenderPrompt_RendersTheLoopInOrder(t *testing.T) {
	got := renderPrompt("Recovered from the failed booking.", Turn{
		Input: "book me in for tomorrow",
		Reply: "You're booked for 14:00.",
		ToolCalls: []ToolCall{
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
	})

	for _, want := range []string{
		"1. check_availability",
		"2. book_appointment",
		"3. book_appointment",
		"ERROR: slot no longer available",
		`{"slots":["10:00","14:00"]}`,
		`{"id":"bk_123"}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, got)
		}
	}

	// Steps must appear in the order the agent took them.
	first := strings.Index(got, "1. check_availability")
	last := strings.Index(got, "3. book_appointment")
	if first < 0 || last < first {
		t.Errorf("steps are out of order:\n%s", got)
	}
}

// A failed step must be distinguishable from a successful one, or
// recovery criteria cannot be judged.
func TestRenderPrompt_MarksFailedSteps(t *testing.T) {
	got := renderPrompt("crit", Turn{
		Reply:     "done",
		ToolCalls: []ToolCall{{Name: "t", Err: "boom"}},
	})
	if !strings.Contains(got, "ERROR: boom") {
		t.Errorf("a failed step must be marked:\n%s", got)
	}
}

// A huge tool result must not blow the prompt, and the cut must be
// visible so the judge does not treat it as complete.
func TestRenderPrompt_TruncatesLargeResults(t *testing.T) {
	got := renderPrompt("crit", Turn{
		Reply: "ok",
		ToolCalls: []ToolCall{
			{Name: "dump", Result: strings.Repeat("y", 50_000)},
		},
	})
	if len(got) > 5_000 {
		t.Errorf("prompt is %d bytes; a large result must be capped",
			len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("the cut must be marked:\n%s", got)
	}
}

// The judge is told that order is significant, or it will not reason
// about sequence.
func TestSystemPrompt_ExplainsOrdering(t *testing.T) {
	for _, want := range []string{"order", "ERROR"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt should mention %q", want)
		}
	}
}
