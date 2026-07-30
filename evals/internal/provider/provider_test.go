// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponderSendsTFCTLToolInUnstoredRequest(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/v1/responses", request.URL.Path)
		require.NoError(t, json.NewDecoder(request.Body).Decode(&requestBody))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{
  "id":"resp_1","object":"response","created_at":1,"status":"completed","model":"test-model",
  "output":[{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"final answer","annotations":[],"logprobs":[]}]}],
  "usage":{"input_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens":3,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":5}
}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client := openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL+"/v1"),
		option.WithHTTPClient(server.Client()),
		option.WithMaxRetries(0),
	)
	responder := &openAIResponder{client: client}
	response, err := responder.Respond(context.Background(), Request{
		Model: "test-model", Instructions: "skill text", Input: "task text",
	})
	require.NoError(t, err)
	require.Equal(t, Response{
		ID: "resp_1", Output: "final answer", InputTokens: 2, OutputTokens: 3,
		Trace: []TraceEntry{
			{Type: TraceMessage, Role: "user", Content: "task text"},
			{Type: TraceResponse, ResponseID: "resp_1", Status: "completed", InputTokens: 2, OutputTokens: 3, ItemTypes: []string{"message"}},
			{Type: TraceMessage, ItemID: "msg_1", ItemType: "message", Status: "completed", Role: "assistant", Content: "final answer"},
		},
	}, response)
	require.Equal(t, "test-model", requestBody["model"])
	require.Equal(t, "skill text\n\nUse the tfctl tool to complete the task. When done, respond with text.", requestBody["instructions"])
	require.Equal(t, []any{map[string]any{"role": "user", "content": "task text"}}, requestBody["input"])
	require.Equal(t, false, requestBody["store"])
	tools := requestBody["tools"].([]any)
	require.Len(t, tools, 1)
	tool := tools[0].(map[string]any)
	require.Equal(t, "function", tool["type"])
	require.Equal(t, "tfctl", tool["name"])
	require.Equal(t, true, tool["strict"])
	parameters := tool["parameters"].(map[string]any)
	require.Equal(t, "object", parameters["type"])
	require.Equal(t, []any{"args"}, parameters["required"])
}

func TestOpenAIResponderReturnsTFCTLToolCall(t *testing.T) {
	var requestBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var requestBody map[string]any
		require.NoError(t, json.NewDecoder(request.Body).Decode(&requestBody))
		requestBodies = append(requestBodies, requestBody)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{
  "id":"resp_1","object":"response","created_at":1,"status":"completed","model":"test-model",
  "output":[
    {"id":"reasoning_1","type":"reasoning","status":"completed","summary":[{"type":"summary_text","text":"I should inspect the workspace's current run relationship first."}]},
    {"id":"call_item_1","type":"function_call","status":"completed","call_id":"call_1","name":"tfctl","arguments":"{\"args\":[\"api\",\"/account/details\",\"--json\"]}"}
  ],
  "usage":{"input_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens":3,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":5}
}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client := openai.NewClient(
		option.WithAPIKey("test-key"), option.WithBaseURL(server.URL+"/v1"),
		option.WithHTTPClient(server.Client()), option.WithMaxRetries(0),
	)
	responder := &openAIResponder{client: client}
	response, err := responder.Respond(context.Background(), Request{
		Model: "test-model", Instructions: "skill text", Input: "task text", Turns: 2,
	})
	require.NoError(t, err)
	require.Equal(t, Response{
		ID: "resp_1", InputTokens: 2, OutputTokens: 3,
		ToolCalls: []ToolCall{{CallID: "call_1", Name: "tfctl", Args: []string{"api", "/account/details", "--json"}}},
		Trace: []TraceEntry{
			{Type: TraceMessage, Role: "user", Content: "task text"},
			{Type: TraceResponse, ResponseID: "resp_1", Status: "completed", InputTokens: 2, OutputTokens: 3, ItemTypes: []string{"reasoning", "function_call"}},
			{
				Type: TraceResponseItem, ItemID: "reasoning_1", ItemType: "reasoning", Status: "completed",
				Summary: "I should inspect the workspace's current run relationship first.",
			},
			{Type: TraceToolUse, ItemID: "call_item_1", ItemType: "function_call", Status: "completed", CallID: "call_1", Name: "tfctl", Args: []string{"api", "/account/details", "--json"}},
		},
	}, Response{
		ID: response.ID, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens,
		ToolCalls: response.ToolCalls, Trace: response.Trace,
	})
	require.NotNil(t, response.State)
	require.Len(t, requestBodies, 1)
}

func TestOpenAIResponderSendsConversationAndToolResultsOnNextTurn(t *testing.T) {
	var requestBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var requestBody map[string]any
		require.NoError(t, json.NewDecoder(request.Body).Decode(&requestBody))
		requestBodies = append(requestBodies, requestBody)
		w.Header().Set("Content-Type", "application/json")
		var body string
		if len(requestBodies) == 1 {
			body = `{
  "id":"resp_1","object":"response","created_at":1,"status":"completed","model":"test-model",
  "output":[{"id":"call_item_1","type":"function_call","status":"completed","call_id":"call_1","name":"tfctl","arguments":"{\"args\":[\"version\"]}"}],
  "usage":{"input_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens":3,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":5}
}`
		} else {
			body = `{
  "id":"resp_2","object":"response","created_at":1,"status":"completed","model":"test-model",
  "output":[{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[],"logprobs":[]}]}],
  "usage":{"input_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens":1,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":5}
}`
		}
		_, err := w.Write([]byte(body))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client := openai.NewClient(
		option.WithAPIKey("test-key"), option.WithBaseURL(server.URL+"/v1"),
		option.WithHTTPClient(server.Client()), option.WithMaxRetries(0),
	)
	responder := &openAIResponder{client: client}
	firstResponse, err := responder.Respond(context.Background(), Request{
		Model: "test-model", Instructions: "skill text", Input: "task text",
	})
	require.NoError(t, err)
	response, err := responder.Respond(context.Background(), Request{
		Model: "test-model", Instructions: "skill text", State: firstResponse.State,
		ToolResults: []ToolResult{{CallID: "call_1", Output: `{"exit_code":2,"stderr":"not found"}`}},
	})
	require.NoError(t, err)
	require.Equal(t, "done", response.Output)
	require.Len(t, requestBodies, 2)
	require.NotContains(t, requestBodies[1], "previous_response_id")
	require.Equal(t, false, requestBodies[1]["store"])
	input := requestBodies[1]["input"].([]any)
	require.Len(t, input, 3)
	require.Equal(t, "user", input[0].(map[string]any)["role"])
	require.Equal(t, "function_call", input[1].(map[string]any)["type"])
	require.Equal(t, map[string]any{
		"type": "function_call_output", "call_id": "call_1", "output": `{"exit_code":2,"stderr":"not found"}`,
	}, input[2])
}

func TestOpenAIResponderTreatsToolUseAsCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{
  "id":"resp_1","object":"response","created_at":1,"status":"completed","model":"test-model",
  "output":[{"id":"call_item_1","type":"function_call","status":"completed","call_id":"call_1","name":"tfctl","arguments":"{\"args\":[\"api\",\"/account/details\"]}"}],
  "usage":{"input_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens":3,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":5}
}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client := openai.NewClient(
		option.WithAPIKey("test-key"), option.WithBaseURL(server.URL+"/v1"),
		option.WithHTTPClient(server.Client()), option.WithMaxRetries(0),
	)
	responder := &openAIResponder{client: client}
	response, err := responder.Respond(context.Background(), Request{Model: "test-model", Turns: 1})
	require.NoError(t, err)
	require.Equal(t, int64(2), response.InputTokens)
	require.Equal(t, "resp_1", response.ID)
	require.Equal(t, []TraceEntry{
		{Type: TraceMessage, Role: "user"},
		{Type: TraceResponse, ResponseID: "resp_1", Status: "completed", InputTokens: 2, OutputTokens: 3, ItemTypes: []string{"function_call"}},
		{Type: TraceToolUse, ItemID: "call_item_1", ItemType: "function_call", Status: "completed", CallID: "call_1", Name: "tfctl", Args: []string{"api", "/account/details"}},
	}, response.Trace)
}

func TestOpenAIResponderRejectsIncompleteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{
  "id":"resp_1","object":"response","created_at":1,"status":"incomplete","model":"test-model",
  "incomplete_details":{"reason":"max_output_tokens"},"output":[],
  "usage":{"input_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens":3,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":5}
}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client := openai.NewClient(
		option.WithAPIKey("test-key"), option.WithBaseURL(server.URL+"/v1"),
		option.WithHTTPClient(server.Client()), option.WithMaxRetries(0),
	)
	response, err := (&openAIResponder{client: client}).Respond(context.Background(), Request{Model: "test-model"})
	require.Equal(t, int64(2), response.InputTokens)
	require.ErrorContains(t, err, "incomplete")
	require.ErrorContains(t, err, "max_output_tokens")
}
