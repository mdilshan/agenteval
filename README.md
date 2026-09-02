# agenteval

Behavioral regression testing for LLM agents, in Go.

An LLM agent is non-deterministic — its wording changes on every prompt
tweak — so you can't pin it with `assert.Equal`. `agenteval` instead
freezes a conversation and asserts what the agent **did**: which tools it
called, how it classified the turn, and — for prose-only checks — an
LLM-as-judge verdict against explicit pass/fail criteria.

- **Two layers.** A deterministic layer (tool calls, metadata, regex) that
  needs no LLM and runs in plain CI, plus an optional LLM-as-judge layer
  for things only expressible in prose (tone, language, "didn't reveal a
  price").
- **One integration point.** You implement a single `Agent` interface (an
  adapter over your engine). The framework owns fixtures, the runner,
  matchers, the judge loop, reporting, and `go test` glue.
- **Zero dependencies.** The core imports only the standard library and
  no LLM SDK — the `Judge` is an interface you satisfy with your own
  client.

## Quick start

Implement `Agent`:

```go
type myAgent struct{ /* your engine, real LLM */ }

func (a myAgent) Run(ctx context.Context, in agenteval.RunInput) (agenteval.Result, error) {
    res := a.engine.Handle(ctx, in.Setup, in.History, in.Message)
    return agenteval.Result{
        Reply:     res.Reply,
        ToolCalls: mapTools(res.Tools),
        Meta:      map[string]any{"escalated": res.Escalated},
    }, nil
}
```

Write a suite (`fixtures/escalation.json`):

```json
{
  "setup": { "escalation_mode": "call_asap" },
  "cases": [
    { "name": "call_now",
      "message": "can you call me now?",
      "expect": {
        "tools_include": ["escalate_to_human"],
        "meta_equals": { "escalated": true },
        "judge": ["Tells the customer a person will call shortly"] } }
  ]
}
```

Run it from a test:

```go
func TestAgent(t *testing.T) {
    agenteval.RunT(t, myAgent{...}, "fixtures", agenteval.Options{
        Judge: myJudge{}, // nil = deterministic-only, no API key
    })
}
```

## Fixture reference

A suite is a JSON file: an optional host-opaque `setup` (shared config)
plus `cases`. Each case is `history` + `message` + `expect`.

`expect` fields:

| field | layer | meaning |
|---|---|---|
| `tools_include` | deterministic | tool names that must be called |
| `tools_exclude` | deterministic | tool names that must NOT be called |
| `meta_equals` | deterministic | exact match on `Result.Meta` (JSON value) |
| `reply_matches` | deterministic | regexp the reply must match |
| `reply_not_matches` | deterministic | regexp the reply must not match |
| `reply_nonempty` | deterministic | reply must be non-blank |
| `judge` | judged | natural-language pass/fail criteria |

`setup` is opaque to the framework — only your `Agent` adapter reads it.

## The judge

`Judge` is an interface, so you reuse your own LLM client (no second
provider, no hard SDK dependency in the core):

```go
type Judge interface {
    Evaluate(ctx context.Context, criterion, transcript, reply string) (Verdict, error)
}
```

Give it temperature 0 and criterion-scoped prompts; set
`Options.JudgeSamples` > 1 to majority-vote borderline criteria. A nil
`Judge` skips judged criteria (they're recorded as skipped, not failed),
so the deterministic layer runs with no API key.

## Status

Early. Core types + deterministic runner + judge interface + `go test`
integration are in. Planned: a default OpenAI judge subpackage, a CLI
(`agenteval run ./fixtures`, `diff` of two runs), and JSON reporting.

## License

TODO (MIT or Apache-2.0).
