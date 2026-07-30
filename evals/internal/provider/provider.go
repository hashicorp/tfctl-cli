// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package provider adapts Responses API providers for the evaluator.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/bedrock"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

type openAIResponder struct {
	client openai.Client
}

type tfctlArguments struct {
	Args []string `json:"args"`
}

type responseState struct {
	input responses.ResponseInputParam
}

const toolInstructions = "Use the tfctl tool to complete the task. When done, respond with text."

// New creates a responder configured for Bedrock or OpenAI.
func New(ctx context.Context, name string) (Responder, error) {
	if name == "bedrock" {
		client, err := bedrock.NewClient(ctx, bedrock.Config{})
		if err != nil {
			return nil, err
		}
		return &openAIResponder{client: client}, nil
	}
	return &openAIResponder{client: openai.NewClient()}, nil
}

func (r *openAIResponder) Respond(ctx context.Context, request Request) (Response, error) {
	result := Response{}
	if len(request.ToolResults) == 0 {
		result.Trace = append(result.Trace, TraceEntry{Type: TraceMessage, Role: "user", Content: request.Input})
	}
	var input responses.ResponseInputParam
	if state, ok := request.State.(responseState); ok {
		input = append(input, state.input...)
	} else {
		input = append(input, responses.ResponseInputItemParamOfMessage(request.Input, responses.EasyInputMessageRoleUser))
	}
	for _, toolResult := range request.ToolResults {
		input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(toolResult.CallID, toolResult.Output))
	}
	params := responses.ResponseNewParams{
		Model:             request.Model,
		Instructions:      openai.String(withToolInstructions(request.Instructions)),
		Input:             responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		ParallelToolCalls: openai.Bool(true),
		MaxToolCalls:      openai.Int(10),
		Store:             openai.Bool(false),
		Tools:             tfctlTools(),
	}
	response, err := r.client.Responses.New(ctx, params)
	if err != nil {
		return result, err
	}
	result.ID = response.ID
	result.Output = response.OutputText()
	result.InputTokens = response.Usage.InputTokens
	result.OutputTokens = response.Usage.OutputTokens
	itemTypes := make([]string, len(response.Output))
	for i, item := range response.Output {
		itemTypes[i] = item.Type
	}
	result.Trace = append(result.Trace, TraceEntry{
		Type: TraceResponse, ResponseID: response.ID, Status: string(response.Status),
		InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, ItemTypes: itemTypes,
	})
	result.Trace, result.ToolCalls, err = appendTrace(result.Trace, response.Output)
	if err != nil {
		return result, err
	}
	if len(result.ToolCalls) > 0 {
		for _, item := range response.Output {
			input = append(input, param.Override[responses.ResponseInputItemUnionParam](json.RawMessage(item.RawJSON())))
		}
		result.State = responseState{input: input}
	}
	if response.Status != responses.ResponseStatusCompleted {
		details := []string{string(response.Status)}
		if response.Error.Message != "" {
			details = append(details, response.Error.Message)
		}
		if response.IncompleteDetails.Reason != "" {
			details = append(details, response.IncompleteDetails.Reason)
		}
		return result, fmt.Errorf("response did not complete: %s", strings.Join(details, ": "))
	}
	return result, nil
}

func withToolInstructions(instructions string) string {
	return strings.TrimRight(instructions, "\n") + "\n\n" + toolInstructions
}

func tfctlTools() []responses.ToolUnionParam {
	return []responses.ToolUnionParam{{OfFunction: &responses.FunctionToolParam{
		Name:        "tfctl",
		Description: openai.String("Invoke the tfctl CLI. Pass each argument separately and omit the tfctl executable name."),
		Strict:      openai.Bool(true),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"args": map[string]any{
					"type":        "array",
					"description": "Arguments to pass to tfctl, for example [\"api\", \"/account/details\", \"--json\"].",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required":             []string{"args"},
			"additionalProperties": false,
		},
	}}}
}

func appendTrace(trace []TraceEntry, output []responses.ResponseOutputItemUnion) ([]TraceEntry, []ToolCall, error) {
	var toolCalls []ToolCall
	for _, item := range output {
		switch item.Type {
		case "message":
			message := item.AsMessage()
			var content strings.Builder
			for _, part := range message.Content {
				switch part.Type {
				case "output_text":
					content.WriteString(part.Text)
				case "refusal":
					content.WriteString(part.Refusal)
				}
			}
			trace = append(trace, TraceEntry{
				Type: TraceMessage, ItemID: item.ID, ItemType: item.Type, Status: item.Status,
				Role: "assistant", Content: content.String(),
			})
		case "function_call":
			call := item.AsFunctionCall()
			var arguments tfctlArguments
			if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil {
				return trace, toolCalls, fmt.Errorf("decode %s tool arguments: %w", call.Name, err)
			}
			trace = append(trace, TraceEntry{
				Type: TraceToolUse, ItemID: item.ID, ItemType: item.Type, Status: item.Status,
				CallID: call.CallID, Name: call.Name, Args: arguments.Args,
			})
			toolCalls = append(toolCalls, ToolCall{CallID: call.CallID, Name: call.Name, Args: arguments.Args})
		case "reasoning":
			summaries := make([]string, len(item.Summary))
			for i, summary := range item.Summary {
				summaries[i] = summary.Text
			}
			trace = append(trace, TraceEntry{
				Type: TraceResponseItem, ItemID: item.ID, ItemType: item.Type, Status: item.Status,
				Summary: strings.Join(summaries, "\n"),
			})
		default:
			trace = append(trace, TraceEntry{
				Type: TraceResponseItem, ItemID: item.ID, ItemType: item.Type, Status: item.Status,
			})
		}
	}
	return trace, toolCalls, nil
}
