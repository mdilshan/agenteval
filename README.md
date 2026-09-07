# agenteval

LLM-as-judge assertions for Go tests. No dependencies.

An LLM agent is non-deterministic — its wording changes on every prompt
tweak — so you can't pin it with `assert.Equal`. But most of what you want
to check about a turn *is* deterministic: which tools it called, how it
classified the input, whether a flag flipped. **Plain Go already tests
that better than any framework could.**

```go
if !slices.Contains(toolNames(res), "escalate_to_human") {
    t.Error("expected an escalation")
}
```

`agenteval` is for the remainder — the claims that are only expressible in
prose, which no amount of Go can assert:

```go
j.Assert(t, "Tells the customer a person will call shortly", turn)
j.Refute(t, "States a specific price", turn)
```

That's the whole library. It does one thing `go test` cannot do, and gets
out of the way for everything else.

## Install

```
go get github.com/mdilshan/agenteval
```

## Bring your own brain

There is no bundled provider. The judge runs on any `Model` you hand it —
that's the entire seam, one method:

```go
type Model interface {
    Complete(ctx context.Context, req Request) (Response, error)
}

type Request  struct { System, User string; JSONObject bool }
type Response struct { Text string; Usage Usage }
```

So the judging logic is written once and runs on whatever you already use
— your existing client, an official SDK, a local model, a gateway. No
provider dependency ever enters your build because of this library, and
you don't get a second SDK you didn't ask for.

An adapter is about fifteen lines:

```go
type myLLM struct{ client *openai.Client }

func (m myLLM) Complete(
    ctx context.Context, req agenteval.Request,
) (agenteval.Response, error) {
    out, err := m.client.Chat(ctx, req.System, req.User)
    if err != nil {
        return agenteval.Response{}, err // never a false verdict
    }
    return agenteval.Response{
        Text:  out.Text,
        Usage: agenteval.Usage{
            PromptTokens:     out.Usage.In,
            CompletionTokens: out.Usage.Out,
        },
    }, nil
}
```

Two things a `Model` should do:

- **Configure it deterministically** — temperature 0 where the provider
  exposes it, so a criterion doesn't flip between runs.
- **Return an error, never empty text, when the call fails.** The judge
  treats an error as *no verdict* and never as the agent misbehaving. This
  is the rule that keeps a rate-limited provider from looking like a
  regression. Retries, backoff and timeouts belong in your `Model` — your
  client almost certainly has them already.

`Request.JSONObject` is a hint that the provider should return a JSON
object. Honour it if you can; the judge parses tolerantly either way, so a
model that ignores it still works.

## Use

```go
//go:build eval

func TestAgent_Escalation(t *testing.T) {
    j := agenteval.New(myLLM{client})

    msg := "can you call me now?"
    res := myAgent.Handle(t.Context(), msg)

    // Deterministic: plain Go, no tokens.
    if !slices.Contains(toolNames(res), "escalate_to_human") {
        t.Error("expected an escalation")
    }

    // Prose-only: the judge.
    turn := agenteval.Turn{
        Input:     msg,
        Reply:     res.Reply,
        ToolCalls: res.Tools,
        Meta:      map[string]any{"language": res.Lang},
    }
    j.AssertAll(t, turn,
        "Tells the customer a person will contact them shortly",
        "Replies in the same language the customer used",
    )
    j.Refute(t, "States a specific price", turn)

    t.Logf("judging cost %d tokens", j.Usage().Total())
}
```

`Assert` takes any `t` — the parameter is a three-method `TB` interface
that `*testing.T`, `*testing.B` and `*testing.F` all satisfy, so this
package imports no testing machinery. Judging runs on `t.Context()`, so it
is cancelled when the test finishes.

## The four methods

`Check` is the primitive; the other three wrap it.

| | fails `t` | direction | criteria |
|---|---|---|---|
| `Check` | no — returns the verdict | — | 1 |
| `Assert` | yes | must hold | 1 |
| `Refute` | yes | must **not** hold | 1 |
| `AssertAll` | yes | must hold | n |

**`Check`** returns the verdict and never touches `t`. Use it when you
want to branch on the result, log it, or count pass rates across a table
rather than fail on the first miss:

```go
v, err := j.Check(ctx, "Replies in the customer's language", turn)
// v.Pass   bool   — did the criterion hold
// v.Reason string — the judge's justification
// v.Usage  Usage  — what this cost
```

The `error` means the **judge** broke — unreachable provider, unparseable
reply. It never means the agent misbehaved; that is `v.Pass == false`.

**`Assert`** fails `t` unless the criterion holds, and returns whether it
did so you can skip follow-up work:

```
criterion not met: Tells the customer a person will call shortly
  judge: the reply only offers a callback form, no mention of a person
  reply: Please fill in this form and we'll be in touch.
```

An unreachable judge fails the test too, but distinctly — `judge
unavailable: ...` — so a down provider is never read as an agent
regression.

**`Refute`** is `Assert` inverted: it fails unless the criterion is
*false*. See below for why it exists.

**`AssertAll`** is a loop of `Assert` over one turn. Each criterion is
reported independently, so a single run shows every failure rather than
stopping at the first.

One model call per criterion (or `n` with `WithSamples`), so `AssertAll`
with three criteria is three calls.

### It judges actions, not just prose

`Turn.ToolCalls` is what separates this from grading a chat message. The
judge sees the trace, so a criterion can be about what the agent *did* and
whether its words are backed by it:

```go
j.Assert(t, "Actually called a tool to hand off to a human", turn)
```

An agent replying *"I'm connecting you to a human"* while calling nothing
is indistinguishable from one that really escalates — until the judge can
see the trace. Pass `ToolCalls`.

### Multi-step runs (the agent loop)

An agent run is usually a loop: call a tool, read the result, decide, call
another, then reply. **One `Turn` is one run, not one step of its loop.**
A run that calls six tools before replying is a single `Turn` with six
entries in `ToolCalls`, in the order the agent took them, each carrying
what came back:

```go
turn := agenteval.Turn{
    Input: "book me in for tomorrow",
    Reply: res.Reply,
    ToolCalls: []agenteval.ToolCall{
        {Name: "check_availability", Args: a1, Result: `{"slots":["10:00","14:00"]}`},
        {Name: "book_appointment",   Args: a2, Err:    "slot no longer available"},
        {Name: "book_appointment",   Args: a3, Result: `{"id":"bk_123"}`},
    },
}
```

The judge is told the order is significant and that an `Err` step failed,
so the loop itself becomes judgeable:

```go
j.Assert(t, "Checked availability before confirming a slot", turn)
j.Assert(t, "Recovered after the first booking attempt failed", turn)
j.Assert(t, "Stopped once the booking succeeded", turn)
j.Refute(t, "Called the same tool with the same arguments more than once", turn)
```

None of those are decidable from a flat list of tool names — they need
order and outcomes. Set `Result` and `Err`; without them the judge sees
only that a tool was named. Set `Err` rather than folding the failure into
`Result`, or recovery criteria have nothing to key on.

Long results are truncated in the prompt (and the cut is marked). If your
tools return large payloads, pass a summary rather than the raw blob —
it's cheaper and judges better.

**Record actions your agent takes outside the tool loop.** Engines often
act deterministically in code — escalating when a classifier is confident,
short-circuiting on a moderation hit — without routing that through the
model's tool calls. Those leave no trace, so a criterion like *"actually
handed off to a human"* is judged against an empty list and fails an agent
that did the right thing. Synthesize an entry:

```go
if esc.fired { // observed from a recording fake
    calls = append([]agenteval.ToolCall{{
        Name:   "escalate_to_human",
        Result: "handed off: " + esc.reason,
    }}, calls...)
}
```

### Multi-turn conversations

Prior conversation goes in `History`; it is context for judging the
current run, not something judged itself:

```go
turn := agenteval.Turn{
    History: []agenteval.Message{
        {Role: agenteval.RoleUser, Text: "do you do evening slots?"},
        {Role: agenteval.RoleAssistant, Text: "We do, up to 7pm."},
    },
    Input: "great, book the latest one",
    Reply: res.Reply,
}
j.Assert(t, "Books an evening slot without re-asking what was already answered", turn)
```

To judge a whole conversation rather than one run, assert on the last
turn with the rest as `History` — that is the turn where a
conversation-level failure actually shows up.

### There is nothing to pause

`agenteval` never drives your agent. It has no runner, no adapter, no
control over your loop — you call your agent yourself and hand the judge a
value. So there is no pause/resume mechanism here, and none is needed.

That is also what makes mid-run judging free. If you want to assert
something part-way through a loop, build a `Turn` from however much of the
trajectory you have at that point and judge it:

```go
// Inside your own agent loop, wherever you already have a step hook.
agent.OnStep = func(s Step) {
    trace = append(trace, agenteval.ToolCall{
        Name: s.Tool, Args: s.Args, Result: s.Result, Err: errText(s.Err),
    })
    if len(trace) == 3 {
        v, err := j.Check(ctx, "Has enough information to answer by now", agenteval.Turn{
            Input: msg, ToolCalls: trace,
        })
        ...
    }
}
```

Pausing, step limits, and resumption are your agent's concern — they are
implementation details of your loop, and a test framework that owned them
would be dictating your architecture. `agenteval` judges whatever snapshot
you give it, whenever you give it.

A practical consequence: **record the trace as you go.** If your agent
already returns one, map it; if it doesn't, a slice appended to in your
tool-dispatch path is the whole implementation. That recording is the only
integration work this library asks of you.

### Writing criteria

A criterion is a **positive pass/fail statement**, never a score and never
"is this good?".

Prefer `Refute` over asserting a negation. Models judge *"States a
specific price"* far more reliably than *"Does not state a specific
price"* — negation is where LLM judges are weakest, so keep the criterion
positive and invert with `Refute`.

### Flakiness

`WithSamples(n)` judges a criterion n times and takes the majority of the
samples that **returned a verdict**; ties fail, and a split vote reports
its tally in the failure message.

It only helps if your `Model` produces variation — at temperature 0 you
are paying n times for one opinion. Raise the model's temperature if you
turn this up.

`WithSystemPrompt(s)` replaces the built-in judging instructions. The
default is tuned to produce binary, criterion-scoped, evidence-bound
verdicts — and to treat the tool trace, not the agent's claims about
itself, as the evidence for any criterion about an action. Override it
only when you know what you are trading away.

### Cost

Every `Assert` is a real model call. Keep judged criteria few and
high-signal, assert everything mechanical in plain Go, and put judged
tests behind a build tag so `go test ./...` stays free:

```
//go:build eval
```
```make
eval:
	go test -tags eval ./...
```

`Judge.Usage()` totals what the suite spent.

## Design

- **Zero dependencies, standard library only.** No LLM SDK, and no
  `testing` import in the library itself.
- **Nothing reinvented.** No fixture format, no assertion DSL, no runner,
  no reporter — `go test` already has all of that, and table-driven tests
  beat a JSON fixture loader. What's here is only what Go lacks.
- **Errors and failures are different things.** A broken judge fails your
  test loudly and distinctly; it is never reported as the agent
  misbehaving, and never silently swallowed into a green run.
- **Go-native.** Built for Go agents, tested with `go test`.

## Development

```
make check   # gofmt + go vet + go test -race   (what CI runs)
```

The whole test suite runs offline with no API key.

## License

[MIT](LICENSE).
