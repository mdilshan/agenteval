package agenteval

import "context"

// Model is the brain behind a [Judge]: anything that can turn a prompt
// into text. It is deliberately the smallest useful interface, so a new
// provider is a short file and not a plugin system — implement it over
// your existing client, an SDK, or raw HTTP.
//
// An implementation SHOULD be configured for deterministic output
// (temperature 0 on providers that expose it) and SHOULD return an error,
// never empty text, when the call fails. That distinction matters: the
// Judge treats an error as "no verdict" and never as the agent failing.
//
// It SHOULD also pin an immutable or dated model snapshot, never a
// floating alias. Because the brain is pluggable and no provider is
// bundled, this package never chooses a model and cannot pin one for you —
// your implementation is the only place that decision exists, and no
// lockfile covers it. A provider re-pointing an alias at new weights moves
// your judged results with nothing changing on your side, so treat the
// model ID as part of the test suite rather than as configuration.
type Model interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// Request is one completion. System and User are plain text; a Model maps
// them onto whatever shape its provider wants (an OpenAI messages array,
// an Anthropic system parameter, a single concatenated prompt).
//
// JSONObject asks the provider to constrain output to a JSON object, for
// those that support it. It is a hint: the Judge parses tolerantly and
// works against a Model that ignores it.
type Request struct {
	System     string
	User       string
	JSONObject bool
}

// Response is a completion's result. Usage may be zero if the provider
// does not report token counts.
type Response struct {
	Text  string
	Usage Usage
}

// Usage is a token count.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Add accumulates o into u.
func (u *Usage) Add(o Usage) {
	u.PromptTokens += o.PromptTokens
	u.CompletionTokens += o.CompletionTokens
}

// Total is prompt plus completion tokens.
func (u Usage) Total() int { return u.PromptTokens + u.CompletionTokens }
