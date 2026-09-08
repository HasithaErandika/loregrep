package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"

// Role is a chat message's speaker, per OpenRouter's OpenAI-compatible
// chat completions schema.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one turn in a chat completion request.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// JSONSchema constrains a completion to a specific output shape
// (OpenRouter's `response_format: {type: "json_schema"}`, strict mode) so
// callers — the agent orchestrator's planner, sufficiency checker, and
// synthesizer (src/internal/agent) — can parse a decision directly instead
// of scraping free text out of a model's reply.
type JSONSchema struct {
	Name   string
	Schema map[string]any
}

// CompletionRequest is one chat completion call.
type CompletionRequest struct {
	Model       string
	Messages    []Message
	Temperature float64
	JSONSchema  *JSONSchema // nil = free-form text response
}

// CompletionResponse is a chat completion's result.
type CompletionResponse struct {
	Content      string
	FinishReason string
}

// LLMClient generates chat completions via an OpenRouter-compatible API —
// used by the agent orchestrator's planner, sufficiency checker, and
// synthesizer. Separate from EmbeddingClient/Voyage: OpenRouter generates
// text, never embeddings, and Voyage the reverse — see
// docs/architecture.md's "LLM access" section.
type LLMClient interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

// OpenRouterClient calls OpenRouter's chat completions API
// (https://openrouter.ai/docs/api-reference/chat-completion).
type OpenRouterClient struct {
	apiKey      string
	endpoint    string // overridable in tests
	httpClient  *http.Client
	maxRetries  int
	baseBackoff time.Duration
}

// NewOpenRouterClient builds a client for the given API key. The model to
// use is chosen per-request (CompletionRequest.Model), not fixed at
// construction, so a single client can serve calls to different models
// (e.g. a cheaper model for the sufficiency check, a stronger one for
// synthesis) if the orchestrator ever wants that.
func NewOpenRouterClient(apiKey string) *OpenRouterClient {
	return &OpenRouterClient{
		apiKey:      apiKey,
		endpoint:    openRouterEndpoint,
		httpClient:  &http.Client{Timeout: 60 * time.Second},
		maxRetries:  3,
		baseBackoff: time.Second,
	}
}

type openRouterRequest struct {
	Model          string                `json:"model"`
	Messages       []Message             `json:"messages"`
	Temperature    float64               `json:"temperature"`
	ResponseFormat *openRouterRespFormat `json:"response_format,omitempty"`
}

type openRouterRespFormat struct {
	Type       string             `json:"type"`
	JSONSchema openRouterJSONSpec `json:"json_schema"`
}

type openRouterJSONSpec struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

type openRouterResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

type openRouterErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete runs one chat completion request, retrying transient (429/5xx)
// failures with exponential backoff — same retry shape as VoyageClient.Embed,
// duplicated rather than shared since the two APIs' request/response
// shapes and error bodies differ enough that a shared abstraction would
// need its own escape hatches anyway.
func (c *OpenRouterClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	body := openRouterRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
	}
	if req.JSONSchema != nil {
		body.ResponseFormat = &openRouterRespFormat{
			Type: "json_schema",
			JSONSchema: openRouterJSONSpec{
				Name:   req.JSONSchema.Name,
				Schema: req.JSONSchema.Schema,
				Strict: true,
			},
		}
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: marshaling OpenRouter request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.baseBackoff * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return CompletionResponse{}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, retryable, err := c.doComplete(ctx, reqBody)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable {
			return CompletionResponse{}, err
		}
	}
	return CompletionResponse{}, fmt.Errorf("llm: OpenRouter completion failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

func (c *OpenRouterClient) doComplete(ctx context.Context, body []byte) (CompletionResponse, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, false, fmt.Errorf("llm: building OpenRouter request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CompletionResponse{}, true, fmt.Errorf("llm: calling OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, true, fmt.Errorf("llm: reading OpenRouter response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		var apiErr openRouterErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return CompletionResponse{}, retryable, fmt.Errorf("llm: OpenRouter API error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		return CompletionResponse{}, retryable, fmt.Errorf("llm: OpenRouter API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var parsed openRouterResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return CompletionResponse{}, false, fmt.Errorf("llm: parsing OpenRouter response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return CompletionResponse{}, false, fmt.Errorf("llm: OpenRouter returned no choices")
	}

	return CompletionResponse{
		Content:      parsed.Choices[0].Message.Content,
		FinishReason: parsed.Choices[0].FinishReason,
	}, false, nil
}
