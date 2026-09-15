// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package openaichat implements an ADK model.LLM backed directly by an
// OpenAI-compatible Chat Completions endpoint (the format most local model
// servers such as llama.cpp, vLLM, and Ollama actually speak). This avoids
// depending on google.golang.org/adk/v2/model/openaimodel, which only
// speaks the newer Responses API, and on any translating proxy in front of
// the local server.
package openaichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
	"google.golang.org/genai"

	"google.golang.org/adk/v2/model"
)

// Errors returned by NewModel and GenerateContent.
var (
	ErrModelNameRequired    = errors.New("openaichat: model name is required")
	ErrRequestNil           = errors.New("openaichat: request is nil")
	ErrNoContents           = errors.New("openaichat: request has no contents")
	ErrStreamingUnsupported = errors.New("openaichat: streaming is not supported")
	ErrEmptyResponse        = errors.New("openaichat: empty response")
	ErrNoOutputContent      = errors.New("openaichat: response has no text or tool calls")
)

// ClientConfig configures the underlying OpenAI-compatible client.
type ClientConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

type chatModel struct {
	client *openai.Client
	name   string
}

// NewModel constructs a model.LLM that calls an OpenAI-compatible Chat
// Completions endpoint at cfg.BaseURL.
func NewModel(_ context.Context, modelName string, cfg *ClientConfig) (model.LLM, error) {
	if modelName == "" {
		return nil, ErrModelNameRequired
	}
	if cfg == nil {
		cfg = &ClientConfig{}
	}
	var opts []option.RequestOption
	if cfg.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.APIKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}
	client := openai.NewClient(opts...)
	return &chatModel{client: &client, name: modelName}, nil
}

func (m *chatModel) Name() string { return m.name }

// GenerateContent implements model.LLM. Only non-streaming generation is
// supported; the tfctl skill evaluator never requests streaming.
func (m *chatModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if stream {
			yield(nil, ErrStreamingUnsupported)
			return
		}
		params, err := buildParams(m.name, req)
		if err != nil {
			yield(nil, err)
			return
		}
		resp, err := m.client.Chat.Completions.New(ctx, params)
		if err != nil {
			yield(nil, fmt.Errorf("openaichat: call failed: %w", err))
			return
		}
		llmResp, err := convertResponse(resp)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(llmResp, nil)
	}
}

func buildParams(modelName string, req *model.LLMRequest) (openai.ChatCompletionNewParams, error) {
	if req == nil {
		return openai.ChatCompletionNewParams{}, ErrRequestNil
	}
	name := modelName
	if req.Model != "" {
		name = req.Model
	}
	messages, err := convertContents(req.Contents, req.Config)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	if len(messages) == 0 {
		return openai.ChatCompletionNewParams{}, ErrNoContents
	}
	params := openai.ChatCompletionNewParams{Model: name, Messages: messages}
	applyGenerationConfig(&params, req.Config)
	tools, err := convertTools(req.Config)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}
	if len(tools) > 0 {
		params.Tools = tools
	}
	return params, nil
}

// convertContents converts the generic conversation history into Chat
// Completions messages. A genai.Content maps to a single message: text
// parts join into the message body, function-call parts become an
// assistant message's tool_calls, and function-response parts become
// individual tool-role messages carrying their matching call ID.
func convertContents(contents []*genai.Content, cfg *genai.GenerateContentConfig) ([]openai.ChatCompletionMessageParamUnion, error) {
	var messages []openai.ChatCompletionMessageParamUnion
	if cfg != nil && cfg.SystemInstruction != nil {
		text, err := flattenText(cfg.SystemInstruction)
		if err != nil {
			return nil, fmt.Errorf("openaichat: system instruction: %w", err)
		}
		if text != "" {
			messages = append(messages, openai.SystemMessage(text))
		}
	}

	var tracker callTracker
	for _, content := range contents {
		if content == nil || len(content.Parts) == 0 {
			continue
		}
		role := genai.Role(content.Role)
		var textParts []string
		var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
		for _, part := range content.Parts {
			switch {
			case part == nil:
				continue
			case part.Text != "":
				textParts = append(textParts, part.Text)
			case part.FunctionCall != nil:
				call, err := tracker.newCall(part.FunctionCall)
				if err != nil {
					return nil, err
				}
				toolCalls = append(toolCalls, call)
			case part.FunctionResponse != nil:
				msg, err := tracker.newResult(part.FunctionResponse)
				if err != nil {
					return nil, err
				}
				messages = append(messages, msg)
			default:
				return nil, fmt.Errorf("openaichat: unsupported content part %T", part)
			}
		}
		text := strings.Join(textParts, "\n")
		switch {
		case len(toolCalls) > 0:
			assistant := openai.ChatCompletionAssistantMessageParam{ToolCalls: toolCalls}
			if text != "" {
				assistant.Content.OfString = param.NewOpt(text)
			}
			messages = append(messages, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
		case text != "":
			msg, err := newTextMessage(role, text)
			if err != nil {
				return nil, err
			}
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

func newTextMessage(role genai.Role, text string) (openai.ChatCompletionMessageParamUnion, error) {
	switch role {
	case "", genai.RoleUser:
		return openai.UserMessage(text), nil
	case genai.RoleModel:
		return openai.AssistantMessage(text), nil
	case "system", "developer":
		return openai.SystemMessage(text), nil
	default:
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("openaichat: unsupported role %q", role)
	}
}

func flattenText(content *genai.Content) (string, error) {
	if content == nil {
		return "", nil
	}
	var b strings.Builder
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		if part.Text == "" {
			return "", fmt.Errorf("non-text part %T", part)
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(part.Text)
	}
	return b.String(), nil
}

// callTracker assigns synthetic call IDs to function calls that arrive
// without one, and matches function responses back to their call so a
// missing response ID can still be resolved to the oldest pending call.
type callTracker struct {
	nextID  int
	pending []string
}

func (t *callTracker) newCall(fc *genai.FunctionCall) (openai.ChatCompletionMessageToolCallUnionParam, error) {
	if fc.Name == "" {
		return openai.ChatCompletionMessageToolCallUnionParam{}, errors.New("openaichat: function call missing name")
	}
	id := fc.ID
	if id == "" {
		id = fmt.Sprintf("adk-openaichat-call-%d", t.nextID)
		t.nextID++
	}
	t.pending = append(t.pending, id)
	args := fc.Args
	if args == nil {
		args = map[string]any{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return openai.ChatCompletionMessageToolCallUnionParam{}, fmt.Errorf("openaichat: marshal function args: %w", err)
	}
	return openai.ChatCompletionMessageToolCallUnionParam{
		OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
			ID: id,
			Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
				Name:      fc.Name,
				Arguments: string(raw),
			},
		},
	}, nil
}

func (t *callTracker) newResult(fr *genai.FunctionResponse) (openai.ChatCompletionMessageParamUnion, error) {
	id := fr.ID
	if id == "" {
		if len(t.pending) == 0 {
			return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("openaichat: response for %q missing call id", fr.Name)
		}
		id = t.pending[0]
		t.pending = t.pending[1:]
	} else {
		found := false
		for i, p := range t.pending {
			if p == id {
				t.pending = append(t.pending[:i], t.pending[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("openaichat: response for unknown or already completed call id %q", id)
		}
	}
	payload, err := json.Marshal(fr.Response)
	if err != nil {
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("openaichat: marshal function response: %w", err)
	}
	return openai.ToolMessage(string(payload), id), nil
}

// applyGenerationConfig copies the handful of generation settings the tfctl
// evaluator actually uses. Fields left unset by the caller keep the
// server's defaults.
func applyGenerationConfig(params *openai.ChatCompletionNewParams, cfg *genai.GenerateContentConfig) {
	if cfg == nil {
		return
	}
	if cfg.Temperature != nil {
		params.Temperature = param.NewOpt(float64(*cfg.Temperature))
	}
	if cfg.TopP != nil {
		params.TopP = param.NewOpt(float64(*cfg.TopP))
	}
	if cfg.MaxOutputTokens > 0 {
		params.MaxCompletionTokens = param.NewOpt(int64(cfg.MaxOutputTokens))
	}
}

func convertTools(cfg *genai.GenerateContentConfig) ([]openai.ChatCompletionToolUnionParam, error) {
	if cfg == nil || len(cfg.Tools) == 0 {
		return nil, nil
	}
	var tools []openai.ChatCompletionToolUnionParam
	for i, tool := range cfg.Tools {
		if tool == nil || len(tool.FunctionDeclarations) == 0 {
			return nil, fmt.Errorf("openaichat: tool %d does not declare any functions", i)
		}
		for _, decl := range tool.FunctionDeclarations {
			fn, err := convertFunctionDeclaration(decl)
			if err != nil {
				return nil, err
			}
			tools = append(tools, openai.ChatCompletionFunctionTool(*fn))
		}
	}
	return tools, nil
}

func convertFunctionDeclaration(fn *genai.FunctionDeclaration) (*shared.FunctionDefinitionParam, error) {
	if fn == nil || fn.Name == "" {
		return nil, errors.New("openaichat: function declaration missing name")
	}
	params, err := schemaToMap(fn.Parameters)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	def := &shared.FunctionDefinitionParam{Name: fn.Name, Parameters: params}
	if fn.Description != "" {
		def.Description = param.NewOpt(fn.Description)
	}
	return def, nil
}

func schemaToMap(schema *genai.Schema) (map[string]any, error) {
	if schema == nil {
		return nil, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("openaichat: marshal schema: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("openaichat: unmarshal schema: %w", err)
	}
	lowercaseSchemaTypes(result)
	return result, nil
}

// lowercaseSchemaTypes rewrites genai's uppercase JSON-schema "type" values
// (e.g. "OBJECT") to the lowercase form ("object") that JSON Schema and
// OpenAI-compatible servers expect.
func lowercaseSchemaTypes(val any) {
	switch v := val.(type) {
	case map[string]any:
		if t, ok := v["type"]; ok {
			switch tVal := t.(type) {
			case string:
				v["type"] = strings.ToLower(tVal)
			case []any:
				for i, item := range tVal {
					if str, ok := item.(string); ok {
						tVal[i] = strings.ToLower(str)
					}
				}
			}
		}
		for _, child := range v {
			lowercaseSchemaTypes(child)
		}
	case []any:
		for _, child := range v {
			lowercaseSchemaTypes(child)
		}
	}
}

func convertResponse(resp *openai.ChatCompletion) (*model.LLMResponse, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, ErrEmptyResponse
	}
	choice := resp.Choices[0]
	parts, err := convertMessage(choice.Message)
	if err != nil {
		return nil, err
	}
	return &model.LLMResponse{
		Content:        &genai.Content{Role: string(genai.RoleModel), Parts: parts},
		FinishReason:   finishReason(choice.FinishReason),
		UsageMetadata:  convertUsage(resp.Usage),
		ModelVersion:   resp.Model,
		CustomMetadata: map[string]any{"openai_response_id": resp.ID},
	}, nil
}

func convertMessage(msg openai.ChatCompletionMessage) ([]*genai.Part, error) {
	var parts []*genai.Part
	if msg.Content != "" {
		parts = append(parts, &genai.Part{Text: msg.Content})
	}
	if msg.Refusal != "" {
		parts = append(parts, &genai.Part{Text: msg.Refusal})
	}
	for _, call := range msg.ToolCalls {
		args, err := functionCallArgs(call.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("%w (name %q, id %q)", err, call.Function.Name, call.ID)
		}
		parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{Name: call.Function.Name, ID: call.ID, Args: args}})
	}
	if len(parts) == 0 {
		return nil, ErrNoOutputContent
	}
	return parts, nil
}

func functionCallArgs(raw string) (map[string]any, error) {
	if raw == "" {
		return map[string]any{}, nil
	}
	args := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, fmt.Errorf("openaichat: decode function call arguments: %w", err)
	}
	return args, nil
}

func finishReason(reason string) genai.FinishReason {
	switch reason {
	case "stop", "tool_calls", "function_call":
		return genai.FinishReasonStop
	case "length":
		return genai.FinishReasonMaxTokens
	case "content_filter":
		return genai.FinishReasonSafety
	case "":
		return genai.FinishReasonUnspecified
	default:
		return genai.FinishReasonOther
	}
}

func convertUsage(usage openai.CompletionUsage) *genai.GenerateContentResponseUsageMetadata {
	return &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     int32(usage.PromptTokens),
		CandidatesTokenCount: int32(usage.CompletionTokens),
		TotalTokenCount:      int32(usage.TotalTokens),
	}
}
