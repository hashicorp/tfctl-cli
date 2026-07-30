// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import "context"

// Responder submits model turns and records tfctl tool calls.
type Responder interface {
	Respond(context.Context, Request) (Response, error)
}

// Request contains the model input needed by a responder.
type Request struct {
	Model        string
	Instructions string
	Input        string
	Turns        int
	ToolResults  []ToolResult
	State        any
}

// ToolResult is the output returned for one model-requested tool call.
type ToolResult struct {
	CallID   string
	Output   string
	ExitCode int
	Stdout   string
	Stderr   string
	Error    string
}

// ToolCall is one model-requested tool invocation.
type ToolCall struct {
	CallID string
	Name   string
	Args   []string
}

// Trace entry types.
const (
	TraceMessage      = "message"
	TraceResponse     = "response"
	TraceResponseItem = "response_item"
	TraceToolUse      = "tool_use"
	TraceToolResult   = "tool_result"
	TraceError        = "error"
)

// TraceEntry records one observable event in a model and tool exchange.
type TraceEntry struct {
	Type         string
	Turn         int
	ResponseID   string
	ItemID       string
	ItemType     string
	ItemTypes    []string
	Status       string
	Summary      string
	Role         string
	Content      string
	Name         string
	Args         []string
	CallID       string
	ExitCode     *int
	Stdout       string
	Stderr       string
	Error        string
	InputTokens  int64
	OutputTokens int64
}

// Response contains final text, the turn trace, and available token usage.
type Response struct {
	ID           string
	Output       string
	InputTokens  int64
	OutputTokens int64
	Trace        []TraceEntry
	ToolCalls    []ToolCall
	State        any
}
