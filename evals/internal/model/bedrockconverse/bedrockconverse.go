// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package bedrockconverse implements an ADK model.LLM backed directly by
// the AWS Bedrock Converse API, using the standard AWS SDK credential
// chain (env vars, shared config/profile, SSO, IMDS, etc.). This avoids
// needing a translating proxy such as LiteLLM in front of Bedrock.
package bedrockconverse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"google.golang.org/genai"

	"google.golang.org/adk/v2/model"
)

// Errors returned by NewModel and GenerateContent.
var (
	ErrModelNameRequired    = errors.New("bedrockconverse: model name is required")
	ErrRequestNil           = errors.New("bedrockconverse: request is nil")
	ErrNoContents           = errors.New("bedrockconverse: request has no contents")
	ErrStreamingUnsupported = errors.New("bedrockconverse: streaming is not supported")
	ErrNoOutputContent      = errors.New("bedrockconverse: response has no text or tool use content")
)

type converseClient interface {
	Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
}

type bedrockModel struct {
	client converseClient
	name   string
}

// NewModel constructs a model.LLM that calls the Bedrock Converse API for
// modelID (a foundation model ID or cross-region inference profile ID).
// Credentials and region come from the standard AWS SDK default chain; set
// AWS_REGION (or AWS_DEFAULT_REGION) and the usual AWS_ACCESS_KEY_ID /
// AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN, or an AWS_PROFILE.
func NewModel(ctx context.Context, modelID string) (model.LLM, error) {
	if modelID == "" {
		return nil, ErrModelNameRequired
	}
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("bedrockconverse: load AWS config: %w", err)
	}
	return &bedrockModel{client: bedrockruntime.NewFromConfig(awsCfg), name: modelID}, nil
}

func (m *bedrockModel) Name() string { return m.name }

// GenerateContent implements model.LLM. Only non-streaming generation is
// supported; the tfctl skill evaluator never requests streaming.
func (m *bedrockModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if stream {
			yield(nil, ErrStreamingUnsupported)
			return
		}
		params, err := buildConverseInput(m.name, req)
		if err != nil {
			yield(nil, err)
			return
		}
		resp, err := m.client.Converse(ctx, params)
		if err != nil {
			yield(nil, fmt.Errorf("bedrockconverse: call failed: %w", err))
			return
		}
		llmResp, err := convertOutput(resp)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(llmResp, nil)
	}
}

func buildConverseInput(modelID string, req *model.LLMRequest) (*bedrockruntime.ConverseInput, error) {
	if req == nil {
		return nil, ErrRequestNil
	}
	name := modelID
	if req.Model != "" {
		name = req.Model
	}
	messages, err := convertContents(req.Contents)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, ErrNoContents
	}
	input := &bedrockruntime.ConverseInput{ModelId: &name, Messages: messages}
	if cfg := req.Config; cfg != nil && cfg.SystemInstruction != nil {
		text, err := flattenText(cfg.SystemInstruction)
		if err != nil {
			return nil, fmt.Errorf("bedrockconverse: system instruction: %w", err)
		}
		if text != "" {
			input.System = []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: text}}
		}
	}
	applyInferenceConfig(input, req.Config)
	toolCfg, err := convertTools(req.Config)
	if err != nil {
		return nil, err
	}
	input.ToolConfig = toolCfg
	return input, nil
}

// convertContents converts the generic conversation history into Bedrock
// Converse messages. A genai.Content maps to a single message: text and
// function-call parts become content blocks on a user/assistant message,
// and function-response parts become tool-result content blocks.
func convertContents(contents []*genai.Content) ([]types.Message, error) {
	var messages []types.Message
	var tracker callTracker
	for _, content := range contents {
		if content == nil || len(content.Parts) == 0 {
			continue
		}
		role, err := convertRole(genai.Role(content.Role))
		if err != nil {
			return nil, err
		}
		var blocks []types.ContentBlock
		for _, part := range content.Parts {
			switch {
			case part == nil:
				continue
			case part.Thought:
				block := reasoningBlock(part)
				if block == nil {
					continue
				}
				blocks = append(blocks, block)
			case part.Text != "":
				blocks = append(blocks, &types.ContentBlockMemberText{Value: part.Text})
			case part.FunctionCall != nil:
				block, err := tracker.newCall(part.FunctionCall)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, block)
			case part.FunctionResponse != nil:
				block, err := tracker.newResult(part.FunctionResponse)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, block)
			default:
				return nil, fmt.Errorf("bedrockconverse: unsupported content part %T", part)
			}
		}
		if len(blocks) == 0 {
			continue
		}
		messages = append(messages, types.Message{Role: role, Content: blocks})
	}
	return messages, nil
}

// reasoningBlock re-encodes a thought part from prior conversation history
// back into a Bedrock reasoning content block. Bedrock requires reasoning
// blocks to be echoed back with their original text and signature unmodified
// in multi-turn conversations; returns nil if the part carries neither.
func reasoningBlock(part *genai.Part) types.ContentBlock {
	switch {
	case part.Text != "":
		text := types.ReasoningTextBlock{Text: &part.Text}
		if len(part.ThoughtSignature) > 0 {
			sig := string(part.ThoughtSignature)
			text.Signature = &sig
		}
		return &types.ContentBlockMemberReasoningContent{Value: &types.ReasoningContentBlockMemberReasoningText{Value: text}}
	case len(part.ThoughtSignature) > 0:
		return &types.ContentBlockMemberReasoningContent{Value: &types.ReasoningContentBlockMemberRedactedContent{Value: part.ThoughtSignature}}
	default:
		return nil
	}
}

func convertRole(role genai.Role) (types.ConversationRole, error) {
	switch role {
	case "", genai.RoleUser:
		return types.ConversationRoleUser, nil
	case genai.RoleModel:
		return types.ConversationRoleAssistant, nil
	default:
		return "", fmt.Errorf("bedrockconverse: unsupported role %q", role)
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

// callTracker assigns synthetic tool-use IDs to function calls that arrive
// without one, and matches function responses back to their call so a
// missing response ID can still be resolved to the oldest pending call.
type callTracker struct {
	nextID  int
	pending []string
}

func (t *callTracker) newCall(fc *genai.FunctionCall) (types.ContentBlock, error) {
	if fc.Name == "" {
		return nil, errors.New("bedrockconverse: function call missing name")
	}
	id := fc.ID
	if id == "" {
		id = fmt.Sprintf("adk-bedrock-call-%d", t.nextID)
		t.nextID++
	}
	t.pending = append(t.pending, id)
	args := fc.Args
	if args == nil {
		args = map[string]any{}
	}
	return &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
		ToolUseId: &id,
		Name:      &fc.Name,
		Input:     document.NewLazyDocument(args),
	}}, nil
}

func (t *callTracker) newResult(fr *genai.FunctionResponse) (types.ContentBlock, error) {
	id := fr.ID
	if id == "" {
		if len(t.pending) == 0 {
			return nil, fmt.Errorf("bedrockconverse: response for %q missing call id", fr.Name)
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
			return nil, fmt.Errorf("bedrockconverse: response for unknown or already completed call id %q", id)
		}
	}
	response := fr.Response
	if response == nil {
		response = map[string]any{}
	}
	return &types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{
		ToolUseId: &id,
		Content: []types.ToolResultContentBlock{
			&types.ToolResultContentBlockMemberJson{Value: document.NewLazyDocument(response)},
		},
	}}, nil
}

// applyInferenceConfig copies the handful of generation settings the tfctl
// evaluator actually uses. Fields left unset by the caller keep the
// model's defaults.
func applyInferenceConfig(input *bedrockruntime.ConverseInput, cfg *genai.GenerateContentConfig) {
	if cfg == nil {
		return
	}
	var inference types.InferenceConfiguration
	var set bool
	if cfg.Temperature != nil {
		v := *cfg.Temperature
		inference.Temperature = &v
		set = true
	}
	if cfg.TopP != nil {
		v := *cfg.TopP
		inference.TopP = &v
		set = true
	}
	if cfg.MaxOutputTokens > 0 {
		v := cfg.MaxOutputTokens
		inference.MaxTokens = &v
		set = true
	}
	if set {
		input.InferenceConfig = &inference
	}
}

func convertTools(cfg *genai.GenerateContentConfig) (*types.ToolConfiguration, error) {
	if cfg == nil || len(cfg.Tools) == 0 {
		return nil, nil
	}
	var tools []types.Tool
	for i, tool := range cfg.Tools {
		if tool == nil || len(tool.FunctionDeclarations) == 0 {
			return nil, fmt.Errorf("bedrockconverse: tool %d does not declare any functions", i)
		}
		for _, decl := range tool.FunctionDeclarations {
			spec, err := convertFunctionDeclaration(decl)
			if err != nil {
				return nil, err
			}
			tools = append(tools, &types.ToolMemberToolSpec{Value: *spec})
		}
	}
	if len(tools) == 0 {
		return nil, nil
	}
	return &types.ToolConfiguration{Tools: tools}, nil
}

func convertFunctionDeclaration(fn *genai.FunctionDeclaration) (*types.ToolSpecification, error) {
	if fn == nil || fn.Name == "" {
		return nil, errors.New("bedrockconverse: function declaration missing name")
	}
	params, err := schemaToMap(fn.Parameters)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	spec := &types.ToolSpecification{
		Name:        &fn.Name,
		InputSchema: &types.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(params)},
	}
	if fn.Description != "" {
		spec.Description = &fn.Description
	}
	return spec, nil
}

func schemaToMap(schema *genai.Schema) (map[string]any, error) {
	if schema == nil {
		return nil, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("bedrockconverse: marshal schema: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("bedrockconverse: unmarshal schema: %w", err)
	}
	lowercaseSchemaTypes(result)
	return result, nil
}

// lowercaseSchemaTypes rewrites genai's uppercase JSON-schema "type" values
// (e.g. "OBJECT") to the lowercase form ("object") that JSON Schema and
// Bedrock's tool input schema expect.
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

func convertOutput(resp *bedrockruntime.ConverseOutput) (*model.LLMResponse, error) {
	if resp == nil {
		return nil, errors.New("bedrockconverse: empty response")
	}
	msg, ok := resp.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return nil, fmt.Errorf("bedrockconverse: unsupported output type %T", resp.Output)
	}
	parts, err := convertContentBlocks(msg.Value.Content)
	if err != nil {
		return nil, err
	}
	return &model.LLMResponse{
		Content:       &genai.Content{Role: string(genai.RoleModel), Parts: parts},
		FinishReason:  finishReason(resp.StopReason),
		UsageMetadata: convertUsage(resp.Usage),
	}, nil
}

func convertContentBlocks(blocks []types.ContentBlock) ([]*genai.Part, error) {
	var parts []*genai.Part
	for _, block := range blocks {
		switch b := block.(type) {
		case *types.ContentBlockMemberText:
			if b.Value != "" {
				parts = append(parts, &genai.Part{Text: b.Value})
			}
		case *types.ContentBlockMemberToolUse:
			args := map[string]any{}
			if b.Value.Input != nil {
				if err := b.Value.Input.UnmarshalSmithyDocument(&args); err != nil {
					return nil, fmt.Errorf("bedrockconverse: decode tool use input: %w", err)
				}
			}
			name, id := "", ""
			if b.Value.Name != nil {
				name = *b.Value.Name
			}
			if b.Value.ToolUseId != nil {
				id = *b.Value.ToolUseId
			}
			parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{Name: name, ID: id, Args: args}})
		case *types.ContentBlockMemberReasoningContent:
			part, err := convertReasoningContent(b.Value)
			if err != nil {
				return nil, err
			}
			if part != nil {
				parts = append(parts, part)
			}
		default:
			return nil, fmt.Errorf("bedrockconverse: unsupported output content block %T", block)
		}
	}
	if len(parts) == 0 {
		return nil, ErrNoOutputContent
	}
	return parts, nil
}

// convertReasoningContent decodes a Bedrock reasoning content block into a
// thought part. Its text and signature (or, for redacted content, the raw
// signature alone) round-trip unmodified through reasoningBlock if this
// response feeds back into a later request.
func convertReasoningContent(block types.ReasoningContentBlock) (*genai.Part, error) {
	switch r := block.(type) {
	case *types.ReasoningContentBlockMemberReasoningText:
		part := &genai.Part{Thought: true}
		if r.Value.Text != nil {
			part.Text = *r.Value.Text
		}
		if r.Value.Signature != nil {
			part.ThoughtSignature = []byte(*r.Value.Signature)
		}
		if part.Text == "" && len(part.ThoughtSignature) == 0 {
			return nil, nil
		}
		return part, nil
	case *types.ReasoningContentBlockMemberRedactedContent:
		if len(r.Value) == 0 {
			return nil, nil
		}
		return &genai.Part{Thought: true, ThoughtSignature: r.Value}, nil
	default:
		return nil, fmt.Errorf("bedrockconverse: unsupported reasoning content block %T", block)
	}
}

func finishReason(reason types.StopReason) genai.FinishReason {
	switch reason {
	case types.StopReasonEndTurn, types.StopReasonToolUse, types.StopReasonStopSequence:
		return genai.FinishReasonStop
	case types.StopReasonMaxTokens:
		return genai.FinishReasonMaxTokens
	case types.StopReasonContentFiltered, types.StopReasonGuardrailIntervened:
		return genai.FinishReasonSafety
	default:
		return genai.FinishReasonOther
	}
}

func convertUsage(usage *types.TokenUsage) *genai.GenerateContentResponseUsageMetadata {
	if usage == nil {
		return nil
	}
	meta := &genai.GenerateContentResponseUsageMetadata{}
	if usage.InputTokens != nil {
		meta.PromptTokenCount = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		meta.CandidatesTokenCount = *usage.OutputTokens
	}
	if usage.TotalTokens != nil {
		meta.TotalTokenCount = *usage.TotalTokens
	}
	return meta
}
