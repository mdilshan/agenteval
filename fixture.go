package agenteval

// Suite is one fixture file: an optional shared setup + a list of cases.
// Config lives once at the suite level so each case stays clean
// input+expected. A stateless agent omits Setup.
type Suite struct {
	Name  string         `json:"name"`
	Setup map[string]any `json:"setup,omitempty"`
	Cases []Case         `json:"cases"`
}

// Case is one input+expected scenario.
type Case struct {
	Name    string `json:"name"`
	History []Turn `json:"history,omitempty"`
	Message string `json:"message"`
	Expect  Expect `json:"expect"`
}

// Expect is the behavior spec. The first group is deterministic (no
// LLM); Judge is the prose-only layer evaluated by a Judge.
type Expect struct {
	// ToolsInclude / ToolsExclude assert tool names present / absent.
	ToolsInclude []string `json:"tools_include,omitempty"`
	ToolsExclude []string `json:"tools_exclude,omitempty"`
	// MetaEquals asserts exact matches on Result.Meta (stage, language,
	// flags). Compared by JSON value so bool/number/string line up
	// across the Go/JSON boundary.
	MetaEquals map[string]any `json:"meta_equals,omitempty"`
	// ReplyMatches / ReplyNotMatches are regexps over the reply.
	ReplyMatches    string `json:"reply_matches,omitempty"`
	ReplyNotMatches string `json:"reply_not_matches,omitempty"`
	// ReplyNonEmpty requires a non-blank reply.
	ReplyNonEmpty bool `json:"reply_nonempty,omitempty"`
	// Judge is a list of natural-language pass/fail criteria (Layer 2).
	Judge []string `json:"judge,omitempty"`
}

// hasJudge reports whether the case needs the judged layer.
func (e Expect) hasJudge() bool { return len(e.Judge) > 0 }
