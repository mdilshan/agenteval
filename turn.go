package agenteval

import "encoding/json"

// Roles for a [Message].
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is one message of prior conversation.
type Message struct {
	Role string // RoleUser | RoleAssistant
	Text string
}

// ToolCall is one observable action the agent took, and what came back.
//
// An agent run is usually a loop — call a tool, read the result, decide,
// call another — so this records a step of that loop, not just an
// intention. Supplying Result and Err is what makes the loop itself
// judgeable: whether the agent checked before it acted, whether it
// recovered from a failure, whether it stopped once it had the answer.
// Without them the judge sees only that a tool was named.
//
// Record EVERY action worth judging here, including ones your agent takes
// outside its tool-calling mechanism. Engines often act deterministically
// in code — escalating to a human when a classifier is confident, say —
// without routing that through the model's tool loop. Such an action
// leaves no tool call behind, so a criterion like "actually handed off to
// a human" would be judged against an empty trace and fail an agent that
// did exactly the right thing. Synthesize an entry for it.
type ToolCall struct {
	// Name of the tool.
	Name string
	// Args the agent passed. Optional.
	Args json.RawMessage
	// Result the tool returned, as the agent saw it. Optional. Long
	// results are truncated in the prompt — pass a summary rather than a
	// megabyte of JSON.
	Result string
	// Err is the tool's failure, if it failed. Optional. Set this rather
	// than folding the error into Result: recovery criteria depend on
	// the judge being able to tell a failed step from a successful one.
	Err string
}

// Turn is one agent run under test — what it was asked, what it did, and
// what it finally said.
//
// "Turn" is one run of your agent, not one step of its loop. A run that
// calls six tools before replying is a single Turn with six ToolCalls in
// ToolCalls, in order.
//
// Only Reply is strictly required. Supplying ToolCalls is what separates
// this from judging a chat message: it lets a criterion be about the
// agent's actions, and about whether its words are backed by them. An
// agent that says "I'm connecting you to a human" and calls no tool is
// indistinguishable from one that does, until the judge can see the
// trace.
type Turn struct {
	// History is the conversation before this turn. Optional.
	History []Message
	// Input is the user message that produced this turn. Optional but
	// usually worth setting — many criteria are about responsiveness to
	// what was actually asked.
	Input string
	// Reply is the agent's final user-visible text for this turn. If the
	// agent emitted interim messages during a multi-step run and you
	// want them judged, include them here.
	Reply string
	// ToolCalls is what the agent did this turn, IN THE ORDER IT DID IT.
	// The judge is told the order is significant, so a criterion may be
	// about sequence, retries, or recovery — not just membership.
	ToolCalls []ToolCall
	// Meta is any classification or flags worth judging against —
	// detected language, stage, escalation flags.
	Meta map[string]any
}
