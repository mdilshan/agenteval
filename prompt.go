package agenteval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// systemPrompt is deliberately narrow. An LLM asked "is this reply good?"
// produces mush; asked whether ONE stated criterion holds, it is a usable
// binary classifier. So: criterion-scoped, evidence-bound, and reasoned
// before it commits to a verdict.
const systemPrompt = `You are a strict evaluator in an automated test suite for an AI agent.

You are given a transcript, the agent's reply, the tools the agent called, and ONE criterion.
Decide whether the agent's turn satisfies that criterion. Nothing else.

Rules:
- Judge ONLY the stated criterion. Ignore every other quality of the turn. A helpful, polite, well-written reply that does not satisfy the criterion FAILS. A clumsy or terse reply that does satisfy it PASSES.
- Judge what is actually there. Do not assume the agent meant something it did not say, and do not credit intentions or steps that are absent.
- If the criterion concerns an action — escalating, booking, looking something up — the tool calls are the evidence. What the agent claims about itself is not.
- The steps are numbered in the order the agent took them. Order is meaningful: a criterion may be about sequence ("checked before it booked"), about recovery from a failed step, about repetition, or about when the agent stopped. A step marked ERROR failed; the agent saw that failure and what it did next is evidence.
- If the criterion is genuinely ambiguous for this turn, fail it and say why. A test that cannot be decided is a bad test and should surface as red.
- Be consistent: the same turn and criterion must always get the same verdict.

Reply with a JSON object and nothing else:
{"reason": "<one or two sentences citing the specific evidence>", "pass": <true|false>}

Write "reason" first and let it justify the verdict you then give.`

// renderPrompt lays out one judging request. Sections are labelled, and
// the criterion comes last so it sits closest to the answer.
func renderPrompt(criterion string, turn Turn) string {
	var b strings.Builder

	b.WriteString("## Transcript\n")
	for _, m := range turn.History {
		who := "Assistant"
		if m.Role == RoleUser {
			who = "User"
		}
		fmt.Fprintf(&b, "%s: %s\n", who, m.Text)
	}
	if turn.Input != "" {
		fmt.Fprintf(&b, "User: %s\n", turn.Input)
	}
	if len(turn.History) == 0 && turn.Input == "" {
		b.WriteString("(not provided)\n")
	}

	b.WriteString("\n## Agent reply\n")
	if strings.TrimSpace(turn.Reply) == "" {
		b.WriteString("(the agent replied with nothing)")
	} else {
		b.WriteString(turn.Reply)
	}

	b.WriteString("\n\n## What the agent did, in order\n")
	if len(turn.ToolCalls) == 0 {
		b.WriteString("(the agent called no tools)")
	} else {
		for i, tc := range turn.ToolCalls {
			fmt.Fprintf(&b, "%d. %s", i+1, tc.Name)
			if len(tc.Args) > 0 {
				b.WriteString(" " + compactJSON(tc.Args))
			}
			switch {
			case tc.Err != "":
				b.WriteString("\n   -> ERROR: " +
					truncate(tc.Err, maxStepDetail))
			case tc.Result != "":
				b.WriteString("\n   -> " +
					truncate(tc.Result, maxStepDetail))
			}
			b.WriteString("\n")
		}
	}

	if len(turn.Meta) > 0 {
		if m, err := json.Marshal(turn.Meta); err == nil {
			b.WriteString("\n## Agent metadata\n")
			b.Write(m)
		}
	}

	b.WriteString("\n\n## Criterion\n")
	b.WriteString(criterion)
	b.WriteString("\n\nDoes the agent's turn satisfy this criterion?")
	return b.String()
}

// maxStepDetail caps a rendered tool result. A run can carry megabytes of
// JSON, and an unbounded prompt is both expensive and liable to blow the
// context window; pass a summary if your results are large.
const maxStepDetail = 600

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// verdictJSON puts reason before pass so the model writes its
// justification before committing to a verdict.
type verdictJSON struct {
	Reason string `json:"reason"`
	Pass   *bool  `json:"pass"`
}

// parseVerdict is tolerant of a Model that ignored the JSON hint and
// wrapped the object in prose or a code fence.
func parseVerdict(text string) (Verdict, error) {
	raw := extractJSONObject(text)
	if raw == "" {
		return Verdict{}, fmt.Errorf(
			"no JSON object in judge reply: %s", truncate(text, 200))
	}
	var vj verdictJSON
	if err := json.Unmarshal([]byte(raw), &vj); err != nil {
		return Verdict{}, fmt.Errorf(
			"parse judge reply: %w (%s)", err, truncate(raw, 200))
	}
	// A missing "pass" must not default to false — that would report the
	// judge's silence as the agent failing.
	if vj.Pass == nil {
		return Verdict{}, fmt.Errorf(
			`judge reply has no "pass" field: %s`, truncate(raw, 200))
	}
	return Verdict{Pass: *vj.Pass, Reason: vj.Reason}, nil
}

// extractJSONObject returns the outermost {...} span of s.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return ""
	}
	return s[start : end+1]
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	// Mark the cut so the judge does not read a truncated result as the
	// whole of what the agent saw.
	return s[:n] + "... (truncated)"
}
