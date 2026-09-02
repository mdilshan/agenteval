package agenteval

import "testing"

func TestEvalDeterministic(t *testing.T) {
	res := Result{
		Reply:     "A team member will help.",
		ToolCalls: []ToolCall{{Name: "escalate_to_human"}},
		Meta:      map[string]any{"escalated": true, "stage": "done"},
	}
	exp := Expect{
		ToolsInclude:  []string{"escalate_to_human"},
		ToolsExclude:  []string{"book_appointment"},
		MetaEquals:    map[string]any{"escalated": true, "stage": "done"},
		ReplyMatches:  "(?i)team member",
		ReplyNonEmpty: true,
	}
	for _, c := range evalDeterministic(exp, res) {
		if !c.Pass {
			t.Errorf("expected pass, got fail: %s %s (%s)",
				c.Kind, c.Detail, c.Reason)
		}
	}
}

func TestEvalDeterministic_Failures(t *testing.T) {
	res := Result{
		Reply: "hello",
		Meta:  map[string]any{"escalated": false},
	}
	exp := Expect{
		ToolsInclude: []string{"escalate_to_human"}, // not called
		MetaEquals:   map[string]any{"escalated": true}, // false
		ReplyMatches: "team member",                     // no match
	}
	for _, c := range evalDeterministic(exp, res) {
		if c.Pass {
			t.Errorf("expected fail, got pass: %s %s", c.Kind, c.Detail)
		}
	}
}

func TestJSONEq(t *testing.T) {
	// bool from Go Meta vs bool from JSON fixture.
	if !jsonEq(true, true) {
		t.Error("true != true")
	}
	// int (Go) vs float64 (JSON) with the same value.
	if !jsonEq(3, float64(3)) {
		t.Error("3 != 3.0")
	}
	if jsonEq("a", "b") {
		t.Error("a == b")
	}
}
