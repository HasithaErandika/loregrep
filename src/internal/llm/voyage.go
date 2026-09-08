// Package llm provides clients for external LLM/embedding APIs. Per
// docs/architecture.md, OpenRouter powers text generation (planner, tool
// router, synthesizer — not yet implemented) and Voyage AI powers the
// semantic_search fallback tool's embeddings; this file implements the
// Voyage side.
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

const (
	voyageEndpoint = "https://api.voyageai.com/v1/embeddings"
	// voyageMaxBatch is the Voyage API's own cap on input strings per
	// request (independent of the per-model token budget) — see
	// https://docs.voyageai.com/reference/embeddings-api.
	voyageMaxBatch = 1000
)

// InputType distinguishes how a text will be used, per Voyage's asymmetric
// embedding support: a corpus chunk (embedded once, offline, at
// input_type "document") and a search query (embedded per request, at
// input_type "query") get different embeddings for the same text even
// under the same model, tuned for retrieval rather than symmetry.
type InputType string

const (
	InputTypeDocument InputType = "document"
	InputTypeQuery    InputType = "query"
)

// EmbeddingClient embeds up to one Voyage request's worth of text into
// vectors. An interface so callers (and their tests) don't depend on a
// live API call — see index.SemanticIndex for how it's used.
type EmbeddingClient interface {
	Embed(ctx context.Context, texts []string, inputType InputType) ([][]float32, error)
}

// BatchEmbeddingClient embeds an arbitrary number of texts, splitting into
// multiple requests as needed. Separate from EmbeddingClient because
// building a corpus-wide vector store (index.BuildVectorStore) needs it,
// while query-time search (index.SemanticIndex) only ever embeds one text
// and shouldn't require it.
type BatchEmbeddingClient interface {
	BatchEmbed(ctx context.Context, texts []string, inputType InputType) ([][]float32, error)
}

// VoyageClient calls Voyage AI's embeddings API.
type VoyageClient struct {
	apiKey      string
	model       string
	endpoint    string // overridable in tests
	httpClient  *http.Client
	maxRetries  int
	baseBackoff time.Duration
	maxBatch    int
}

// NewVoyageClient builds a client for the given API key and model (e.g.
// "voyage-4-lite", the default per docs/architecture.md).
func NewVoyageClient(apiKey, model string) *VoyageClient {
	return &VoyageClient{
		apiKey:      apiKey,
		model:       model,
		endpoint:    voyageEndpoint,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		maxRetries:  3,
		baseBackoff: time.Second,
		maxBatch:    voyageMaxBatch,
	}
}

type voyageRequest struct {
	Input     []string  `json:"input"`
	Model     string    `json:"model"`
	InputType InputType `json:"input_type,omitempty"`
}

type voyageEmbeddingDatum struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type voyageResponse struct {
	Data []voyageEmbeddingDatum `json:"data"`
}

type voyageErrorResponse struct {
	Detail string `json:"detail"`
}

// Embed embeds texts (at most c.maxBatch per call — BatchEmbed splits
// larger requests) via one Voyage API call, retrying transient (429/5xx)
// failures with exponential backoff.
func (c *VoyageClient) Embed(ctx context.Context, texts []string, inputType InputType) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) > c.maxBatch {
		return nil, fmt.Errorf("llm: Embed: %d texts exceeds the %d-per-request limit; use BatchEmbed", len(texts), c.maxBatch)
	}

	reqBody, err := json.Marshal(voyageRequest{Input: texts, Model: c.model, InputType: inputType})
	if err != nil {
		return nil, fmt.Errorf("llm: marshaling Voyage request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.baseBackoff * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		embeddings, retryable, err := c.doEmbed(ctx, reqBody, len(texts))
		if err == nil {
			return embeddings, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("llm: Voyage embed failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// doEmbed makes one HTTP round trip. The bool return says whether a
// non-nil error is worth retrying (network errors, 429, 5xx) or not
// (a malformed request, an unparseable response, a 4xx that won't fix
// itself on retry).
func (c *VoyageClient) doEmbed(ctx context.Context, body []byte, want int) ([][]float32, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("llm: building Voyage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("llm: calling Voyage: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("llm: reading Voyage response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		var apiErr voyageErrorResponse
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Detail != "" {
			return nil, retryable, fmt.Errorf("llm: Voyage API error (%d): %s", resp.StatusCode, apiErr.Detail)
		}
		return nil, retryable, fmt.Errorf("llm: Voyage API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var parsed voyageResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, false, fmt.Errorf("llm: parsing Voyage response: %w", err)
	}
	if len(parsed.Data) != want {
		return nil, false, fmt.Errorf("llm: Voyage returned %d embeddings for %d inputs", len(parsed.Data), want)
	}

	embeddings := make([][]float32, want)
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= want {
			return nil, false, fmt.Errorf("llm: Voyage returned out-of-range index %d for %d inputs", d.Index, want)
		}
		embeddings[d.Index] = d.Embedding
	}
	return embeddings, false, nil
}

// BatchEmbed embeds an arbitrary number of texts, splitting into
// c.maxBatch-sized requests as needed and concatenating results in input
// order.
func (c *VoyageClient) BatchEmbed(ctx context.Context, texts []string, inputType InputType) ([][]float32, error) {
	all := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += c.maxBatch {
		end := min(start+c.maxBatch, len(texts))
		batch, err := c.Embed(ctx, texts[start:end], inputType)
		if err != nil {
			return nil, fmt.Errorf("llm: batch embedding texts %d-%d: %w", start, end, err)
		}
		all = append(all, batch...)
	}
	return all, nil
}
