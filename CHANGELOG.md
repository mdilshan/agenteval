# Changelog

## v0.3.0 (2026-09-07) — scope reset

`agenteval` is now only the thing `go test` cannot do: LLM-as-judge
assertions. Everything it was reinventing is gone.

### Removed

- **The deterministic assertion layer** — `tools_include`,
  `tools_exclude`, `meta_equals`, `reply_matches`, `reply_not_matches`,
  `reply_nonempty`. These re-implemented, worse, what a plain `if` and
  `slices.Contains` already do in a table-driven test.
- **The JSON fixture format**, its loader, and its validation. Go test
  tables are better at this, and the cross-language ambition the format
  existed to serve is dropped — this targets Go agents.
- **The runner and reporter** — `Run`, `RunDir`, `RunT`, `Report`,
  `CaseReport`, `Check`. `go test` is the runner; `-run`, `-v`, subtests
  and `t.Errorf` are the reporting.
- **The `Agent` adapter seam** (`Agent`, `RunInput`, `Result`). You call
  your own agent directly in your own test; nothing needs to wrap it.
- **The bundled `judge/openai` provider.** See below.

### Bring your own brain

There is no bundled provider any more. The judge runs on a `Model` you
supply:

```go
type Model interface {
    Complete(ctx context.Context, req Request) (Response, error)
}
```

One judging implementation, any LLM behind it — rather than a judge per
provider. Shipping a provider would have meant either an SDK dependency in
a zero-dependency library, or a hand-rolled HTTP client competing with the
one you already have. Neither is this library's job. An adapter over your
existing client is about fifteen lines; the README has one.

### Multi-step runs

- `ToolCall` gained `Result` and `Err`, and the prompt now renders the
  trace as a numbered, ordered trajectory with each step's outcome. One
  `Turn` is one agent *run* — a loop of six tool calls is one Turn with
  six ordered `ToolCalls`, not six Turns.

  This is what makes the loop judgeable. Ordering, recovery from a failed
  step, repeated calls and when the agent stopped are all undecidable
  from a flat list of tool names:

  ```go
  j.Assert(t, "Checked availability before confirming a slot", turn)
  j.Assert(t, "Recovered after the first booking attempt failed", turn)
  ```

  The system prompt now tells the judge that step order is significant
  and that an `ERROR` step failed. Long results are truncated with the
  cut marked, so the judge does not read a partial result as complete.

### Documented

- **Pin the judge model.** Because the brain is pluggable and nothing is
  bundled, this package never chooses a model and cannot pin one — and no
  lockfile reaches provider weights, so a re-pointed alias moves judged
  results with nothing changing on the caller's side. The judging prompt
  needs no mechanism here: Go module versions are immutable, so `go.mod`
  already pins it.

### Added

- `Judge.Assert` / `Refute` / `AssertAll` — `Refute` exists because models
  judge positive statements far more reliably than negations.
- `Judge.Check` for the verdict without the assertion.
- `Judge.Usage()` totals judging cost across a run.
- `WithSamples(n)` majority-voting; `WithSystemPrompt` to override the
  judging instructions.
- `Turn.ToolCalls` and `Turn.Meta` reach the judge, so criteria can be
  about what the agent *did*, not only what it said.
- The library imports no `testing` package: assertion helpers take a
  three-method `TB` interface that `*testing.T` satisfies.

### Kept from v0.2.0

- A judge that errors is *no verdict*, never a failing one — a flaky
  provider must not read as an agent regression.
- A judge reply with no `pass` field is an error, not a `false`.
- Majority is over samples that returned a verdict; ties fail.
- Tolerant verdict parsing (fenced or prose-wrapped JSON).

## v0.2.0

Never released.

## v0.1.0

Initial release: core types, fixture loading, deterministic matchers, the
`Judge` interface, reporting, and `go test` integration.
