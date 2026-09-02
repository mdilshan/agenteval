package agenteval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Check is one evaluated assertion.
type Check struct {
	Kind    string // e.g. "tools_include", "meta_equals", "judge"
	Detail  string // the specific thing checked
	Pass    bool
	Skipped bool   // e.g. a judged criterion with no Judge configured
	Reason  string // why it failed / was skipped (empty when a clean pass)
}

// evalDeterministic runs the no-LLM assertions of an Expect against a
// Result and returns one Check per assertion.
func evalDeterministic(e Expect, r Result) []Check {
	var out []Check

	for _, name := range e.ToolsInclude {
		ok := r.hasTool(name)
		out = append(out, Check{
			Kind:   "tools_include",
			Detail: name,
			Pass:   ok,
			Reason: reasonIf(!ok, "tool not called"),
		})
	}
	for _, name := range e.ToolsExclude {
		called := r.hasTool(name)
		out = append(out, Check{
			Kind:   "tools_exclude",
			Detail: name,
			Pass:   !called,
			Reason: reasonIf(called, "tool was called"),
		})
	}
	for k, want := range e.MetaEquals {
		got, present := r.Meta[k]
		ok := present && jsonEq(got, want)
		out = append(out, Check{
			Kind:   "meta_equals",
			Detail: fmt.Sprintf("%s=%v", k, want),
			Pass:   ok,
			Reason: reasonIf(!ok,
				fmt.Sprintf("got %v", metaVal(got, present))),
		})
	}
	if e.ReplyMatches != "" {
		ok := matchRegex(e.ReplyMatches, r.Reply)
		out = append(out, Check{
			Kind:   "reply_matches",
			Detail: e.ReplyMatches,
			Pass:   ok,
			Reason: reasonIf(!ok, "no match"),
		})
	}
	if e.ReplyNotMatches != "" {
		matched := matchRegex(e.ReplyNotMatches, r.Reply)
		out = append(out, Check{
			Kind:   "reply_not_matches",
			Detail: e.ReplyNotMatches,
			Pass:   !matched,
			Reason: reasonIf(matched, "matched"),
		})
	}
	if e.ReplyNonEmpty {
		ok := strings.TrimSpace(r.Reply) != ""
		out = append(out, Check{
			Kind:   "reply_nonempty",
			Pass:   ok,
			Reason: reasonIf(!ok, "reply was blank"),
		})
	}
	return out
}

func matchRegex(pattern, s string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

// jsonEq compares two values by their JSON encoding, so a Go bool/int/
// string lines up with the bool/float64/string a fixture parses to.
func jsonEq(a, b any) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(ab, bb)
}

func metaVal(v any, present bool) string {
	if !present {
		return "<absent>"
	}
	return fmt.Sprintf("%v", v)
}

func reasonIf(cond bool, reason string) string {
	if cond {
		return reason
	}
	return ""
}
