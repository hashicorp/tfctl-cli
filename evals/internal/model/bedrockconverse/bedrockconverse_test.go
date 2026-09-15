// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package bedrockconverse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"google.golang.org/genai"

	"google.golang.org/adk/v2/model"
)

func TestBuildConverseInputConvertsTextAndSystemInstruction(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: "list workspaces"}}}},
		Config:   &genai.GenerateContentConfig{SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: "you are tfctl"}}}},
	}
	input, err := buildConverseInput("my-model", req)
	if err != nil {
		t.Fatal(err)
	}
	if got := *input.ModelId; got != "my-model" {
		t.Errorf("ModelId = %q", got)
	}
	if len(input.System) != 1 {
		t.Fatalf("System = %#v", input.System)
	}
	sys, ok := input.System[0].(*types.SystemContentBlockMemberText)
	if !ok || sys.Value != "you are tfctl" {
		t.Fatalf("System[0] = %#v", input.System[0])
	}
	if len(input.Messages) != 1 || input.Messages[0].Role != types.ConversationRoleUser {
		t.Fatalf("Messages = %#v", input.Messages)
	}
	text, ok := input.Messages[0].Content[0].(*types.ContentBlockMemberText)
	if !ok || text.Value != "list workspaces" {
		t.Fatalf("Messages[0].Content[0] = %#v", input.Messages[0].Content[0])
	}
}

func TestBuildConverseInputRoundTripsFunctionCallAndResponse(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: string(genai.RoleModel), Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: "tfctl", Args: map[string]any{"args": []string{"version"}}}}}},
			{Role: string(genai.RoleUser), Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{Name: "tfctl", Response: map[string]any{"exit_code": float64(0)}}}}},
		},
	}
	input, err := buildConverseInput("my-model", req)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Messages) != 2 {
		t.Fatalf("Messages = %#v", input.Messages)
	}
	use, ok := input.Messages[0].Content[0].(*types.ContentBlockMemberToolUse)
	if !ok || *use.Value.Name != "tfctl" || *use.Value.ToolUseId == "" {
		t.Fatalf("Messages[0].Content[0] = %#v", input.Messages[0].Content[0])
	}
	result, ok := input.Messages[1].Content[0].(*types.ContentBlockMemberToolResult)
	if !ok || *result.Value.ToolUseId != *use.Value.ToolUseId {
		t.Fatalf("Messages[1].Content[0] = %#v", input.Messages[1].Content[0])
	}
}

func TestBuildConverseInputRoundTripsReasoningContent(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: string(genai.RoleModel), Parts: []*genai.Part{
			{Text: "let me think", Thought: true, ThoughtSignature: []byte("sig-123")},
			{Text: "the answer"},
		}}},
	}
	input, err := buildConverseInput("my-model", req)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Messages) != 1 || len(input.Messages[0].Content) != 2 {
		t.Fatalf("Messages = %#v", input.Messages)
	}
	reasoning, ok := input.Messages[0].Content[0].(*types.ContentBlockMemberReasoningContent)
	if !ok {
		t.Fatalf("Content[0] = %#v", input.Messages[0].Content[0])
	}
	text, ok := reasoning.Value.(*types.ReasoningContentBlockMemberReasoningText)
	if !ok || *text.Value.Text != "let me think" || *text.Value.Signature != "sig-123" {
		t.Fatalf("reasoning content = %#v", reasoning.Value)
	}
	if _, ok := input.Messages[0].Content[1].(*types.ContentBlockMemberText); !ok {
		t.Fatalf("Content[1] = %#v", input.Messages[0].Content[1])
	}
}

func TestBuildConverseInputConvertsTools(t *testing.T) {
	req := &model.LLMRequest{
		Contents: []*genai.Content{{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: "run tfctl"}}}},
		Config: &genai.GenerateContentConfig{Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{{
			Name:        "tfctl",
			Description: "Run a tfctl command.",
			Parameters:  &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{"args": {Type: genai.TypeArray}}},
		}}}}},
	}
	input, err := buildConverseInput("my-model", req)
	if err != nil {
		t.Fatal(err)
	}
	if input.ToolConfig == nil || len(input.ToolConfig.Tools) != 1 {
		t.Fatalf("ToolConfig = %#v", input.ToolConfig)
	}
	spec, ok := input.ToolConfig.Tools[0].(*types.ToolMemberToolSpec)
	if !ok || *spec.Value.Name != "tfctl" {
		t.Fatalf("Tools[0] = %#v", input.ToolConfig.Tools[0])
	}
	schema, ok := spec.Value.InputSchema.(*types.ToolInputSchemaMemberJson)
	if !ok {
		t.Fatalf("InputSchema = %#v", spec.Value.InputSchema)
	}
	raw, err := schema.Value.MarshalSmithyDocument()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); !strings.Contains(got, `"type":"object"`) {
		t.Errorf("schema = %s, want lowercase object type", got)
	}
}

// TestGenerateContentParsesTextAndToolUse drives a real bedrockruntime
// client against a stub HTTP server returning a canned Converse response,
// so the response is decoded by the SDK's own JSON deserializer rather
// than a hand-built types.ConverseOutput. types.ToolUseBlock.Input only
// implements document.Interface.UnmarshalSmithyDocument correctly once it
// has gone through that deserializer.
func TestGenerateContentParsesTextAndToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output": {"message": {"role": "assistant", "content": [
				{"reasoningContent": {"reasoningText": {"text": "thinking it through", "signature": "sig-123"}}},
				{"text": "running it"},
				{"toolUse": {"toolUseId": "call-1", "name": "tfctl", "input": {"args": ["version"]}}}
			]}},
			"stopReason": "tool_use",
			"usage": {"inputTokens": 10, "outputTokens": 5, "totalTokens": 15},
			"metrics": {"latencyMs": 1}
		}`))
	}))
	defer server.Close()

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := bedrockruntime.NewFromConfig(awsCfg, func(o *bedrockruntime.Options) { o.BaseEndpoint = aws.String(server.URL) })
	m := &bedrockModel{client: client, name: "my-model"}

	req := &model.LLMRequest{Contents: []*genai.Content{{Role: string(genai.RoleUser), Parts: []*genai.Part{{Text: "run tfctl version"}}}}}
	var llmResp *model.LLMResponse
	for resp, err := range m.GenerateContent(context.Background(), req, false) {
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
	if len(llmResp.Content.Parts) != 3 {
		t.Fatalf("Parts = %#v", llmResp.Content.Parts)
	}
	thought := llmResp.Content.Parts[0]
	if !thought.Thought || thought.Text != "thinking it through" || string(thought.ThoughtSignature) != "sig-123" {
		t.Errorf("Parts[0] = %#v", thought)
	}
	if llmResp.Content.Parts[1].Text != "running it" {
		t.Errorf("Parts[1] = %#v", llmResp.Content.Parts[1])
	}
	fc := llmResp.Content.Parts[2].FunctionCall
	if fc == nil || fc.Name != "tfctl" || fc.ID != "call-1" {
		t.Fatalf("Parts[2].FunctionCall = %#v", fc)
	}
	if diff := fc.Args["args"]; diff == nil {
		t.Errorf("FunctionCall.Args = %#v, want args key", fc.Args)
	}
	if llmResp.UsageMetadata == nil || llmResp.UsageMetadata.TotalTokenCount != 15 {
		t.Errorf("UsageMetadata = %#v", llmResp.UsageMetadata)
	}
}

func TestGenerateContentRejectsStreaming(t *testing.T) {
	m := &bedrockModel{client: stubClient{}, name: "my-model"}
	for _, err := range m.GenerateContent(context.Background(), &model.LLMRequest{}, true) {
		if err != ErrStreamingUnsupported {
			t.Fatalf("err = %v, want ErrStreamingUnsupported", err)
		}
		return
	}
	t.Fatal("expected one yielded error")
}

type stubClient struct{}

func (stubClient) Converse(context.Context, *bedrockruntime.ConverseInput, ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error) {
	panic("not called")
}
