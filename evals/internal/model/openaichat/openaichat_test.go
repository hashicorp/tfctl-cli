// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package openaichat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/model"
)

func TestBuildParamsConvertsTextAndSystemInstruction(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: "list workspaces"}}}},
		Config:   &genai.GenerateContentConfig{SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: "you are tfctl"}}}},
	}
	params, err := buildParams("my-model", req)
	if err != nil {
		t.Fatal(err)
	}
	if got := params.Model; got != "my-model" {
		t.Errorf("Model = %q", got)
	}
	if len(params.Messages) != 2 {
		t.Fatalf("Messages = %#v", params.Messages)
	}
	if params.Messages[0].OfSystem == nil || params.Messages[0].OfSystem.Content.OfString.Value != "you are tfctl" {
		t.Fatalf("Messages[0] = %#v", params.Messages[0])
	}
	if params.Messages[1].OfUser == nil || params.Messages[1].OfUser.Content.OfString.Value != "list workspaces" {
		t.Fatalf("Messages[1] = %#v", params.Messages[1])
	}
}

func TestBuildParamsRoundTripsFunctionCallAndResponse(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: string(genai.RoleModel), Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: "tfctl", Args: map[string]any{"args": []string{"version"}}}}}},
			{Role: string(genai.RoleUser), Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{Name: "tfctl", Response: map[string]any{"exit_code": float64(0)}}}}},
		},
	}
	params, err := buildParams("my-model", req)
	if err != nil {
		t.Fatal(err)
	}
	if len(params.Messages) != 2 {
		t.Fatalf("Messages = %#v", params.Messages)
	}
	assistant := params.Messages[0].OfAssistant
	if assistant == nil || len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].OfFunction.Function.Name != "tfctl" {
		t.Fatalf("Messages[0] = %#v", params.Messages[0])
	}
	callID := assistant.ToolCalls[0].OfFunction.ID
	if callID == "" {
		t.Fatal("expected a synthesized call ID")
	}
	tool := params.Messages[1].OfTool
	if tool == nil || tool.ToolCallID != callID {
		t.Fatalf("Messages[1] = %#v, want tool call id %q", params.Messages[1], callID)
	}
}

func TestBuildParamsConvertsTools(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: "run tfctl"}}}},
		Config: &genai.GenerateContentConfig{Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{{
			Name:        "tfctl",
			Description: "Run a tfctl command.",
			Parameters:  &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{"args": {Type: genai.TypeArray}}},
		}}}}},
	}
	params, err := buildParams("my-model", req)
	if err != nil {
		t.Fatal(err)
	}
	if len(params.Tools) != 1 {
		t.Fatalf("Tools = %#v", params.Tools)
	}
	fn := params.Tools[0].OfFunction
	if fn == nil || fn.Function.Name != "tfctl" {
		t.Fatalf("Tools[0] = %#v", params.Tools[0])
	}
	if got := fn.Function.Parameters["type"]; got != "object" {
		t.Errorf("schema type = %v, want lowercase object", got)
	}
}

func TestGenerateContentParsesTextAndToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "resp-1",
			"object": "chat.completion",
			"created": 0,
			"model": "my-model",
			"choices": [{
				"index": 0,
				"finish_reason": "tool_calls",
				"message": {
					"role": "assistant",
					"content": "running it",
					"tool_calls": [{"id": "call-1", "type": "function", "function": {"name": "tfctl", "arguments": "{\"args\":[\"version\"]}"}}]
				}
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`))
	}))
	defer server.Close()

	llm, err := NewModel(context.Background(), "my-model", &ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	req := &model.LLMRequest{Contents: []*genai.Content{{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: "run tfctl version"}}}}}
	var llmResp *model.LLMResponse
	for resp, err := range llm.GenerateContent(context.Background(), req, false) {
		if err != nil {
			t.Fatal(err)
		}
		llmResp = resp
	}
	if llmResp == nil {
		t.Fatal("GenerateContent yielded no response")
	}
	if got := llmResp.FinishReason; got != genai.FinishReasonStop {
		t.Errorf("FinishReason = %v", got)
	}
	if len(llmResp.Content.Parts) != 2 {
		t.Fatalf("Parts = %#v", llmResp.Content.Parts)
	}
	if llmResp.Content.Parts[0].Text != "running it" {
		t.Errorf("Parts[0] = %#v", llmResp.Content.Parts[0])
	}
	fc := llmResp.Content.Parts[1].FunctionCall
	if fc == nil || fc.Name != "tfctl" || fc.ID != "call-1" || fc.Args["args"] == nil {
		t.Fatalf("Parts[1].FunctionCall = %#v", fc)
	}
	if llmResp.UsageMetadata == nil || llmResp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("UsageMetadata = %#v", llmResp.UsageMetadata)
	}
}

func TestGenerateContentRejectsStreaming(t *testing.T) {
	llm, err := NewModel(context.Background(), "my-model", &ClientConfig{BaseURL: "http://127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range llm.GenerateContent(context.Background(), &model.LLMRequest{}, true) {
		if err != ErrStreamingUnsupported {
			t.Fatalf("err = %v, want ErrStreamingUnsupported", err)
		}
		return
	}
	t.Fatal("expected one yielded error")
}
