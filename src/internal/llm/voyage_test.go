package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, server *httptest.Server) *VoyageClient {
	t.Helper()
	return &VoyageClient{
		apiKey:      "test-key",
		model:       "voyage-4-lite",
		endpoint:    server.URL,
		httpClient:  server.Client(),
		maxRetries:  2,
		baseBackoff: time.Millisecond,
		maxBatch:    3,
	}
}

func TestEmbed_Success(t *testing.T) {
	var gotAuth, gotInputType string
	var gotBody voyageRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		gotInputType = string(gotBody.InputType)

		// Return embeddings deliberately out of order to verify the
		// client places them by their `index` field, not response order.
		resp := voyageResponse{Data: []voyageEmbeddingDatum{
			{Embedding: []float32{0.2}, Index: 1},
			{Embedding: []float32{0.1}, Index: 0},
		}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := testClient(t, server)
	vecs, err := c.Embed(context.Background(), []string{"a", "b"}, InputTypeQuery)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want Bearer test-key", gotAuth)
	}
	if gotInputType != "query" {
		t.Errorf("input_type = %q, want query", gotInputType)
	}
	if len(vecs) != 2 || vecs[0][0] != 0.1 || vecs[1][0] != 0.2 {
		t.Fatalf("vecs = %+v, want [[0.1] [0.2]] (placed by index, not response order)", vecs)
	}
}

func TestEmbed_EmptyInputNoRequest(t *testing.T) {
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
	}))
	defer server.Close()

	c := testClient(t, server)
	vecs, err := c.Embed(context.Background(), nil, InputTypeQuery)
	if err != nil || vecs != nil {
		t.Fatalf("Embed(nil) = %v, %v, want nil, nil", vecs, err)
	}
	if called.Load() != 0 {
		t.Error("expected no HTTP call for empty input")
	}
}

func TestEmbed_RetriesOn429ThenSucceeds(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(voyageErrorResponse{Detail: "rate limited"})
			return
		}
		json.NewEncoder(w).Encode(voyageResponse{Data: []voyageEmbeddingDatum{{Embedding: []float32{1}, Index: 0}}})
	}))
	defer server.Close()

	c := testClient(t, server)
	vecs, err := c.Embed(context.Background(), []string{"x"}, InputTypeDocument)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 1 {
		t.Fatalf("vecs = %+v, want 1 embedding", vecs)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
}

func TestEmbed_NonRetryable400StopsImmediately(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(voyageErrorResponse{Detail: "bad model"})
	}))
	defer server.Close()

	c := testClient(t, server)
	_, err := c.Embed(context.Background(), []string{"x"}, InputTypeDocument)
	if err == nil {
		t.Fatal("Embed: want error for 400, got nil")
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 400)", attempts.Load())
	}
}

func TestEmbed_PersistentServerErrorExhaustsRetries(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := testClient(t, server)
	_, err := c.Embed(context.Background(), []string{"x"}, InputTypeDocument)
	if err == nil {
		t.Fatal("Embed: want error after exhausting retries, got nil")
	}
	if got, want := attempts.Load(), int32(c.maxRetries+1); got != want {
		t.Errorf("attempts = %d, want %d (initial + maxRetries)", got, want)
	}
}

func TestEmbed_OverBatchLimitRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Embed should not make an HTTP call when over its batch limit")
	}))
	defer server.Close()

	c := testClient(t, server) // maxBatch: 3
	_, err := c.Embed(context.Background(), []string{"a", "b", "c", "d"}, InputTypeDocument)
	if err == nil {
		t.Fatal("Embed: want error for over-limit batch, got nil")
	}
}

func TestBatchEmbed_SplitsAndConcatenatesInOrder(t *testing.T) {
	var requestSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req voyageRequest
		json.NewDecoder(r.Body).Decode(&req)
		requestSizes = append(requestSizes, len(req.Input))

		data := make([]voyageEmbeddingDatum, len(req.Input))
		for i, text := range req.Input {
			data[i] = voyageEmbeddingDatum{Embedding: []float32{float32(len(text))}, Index: i}
		}
		json.NewEncoder(w).Encode(voyageResponse{Data: data})
	}))
	defer server.Close()

	c := testClient(t, server) // maxBatch: 3
	texts := []string{"a", "bb", "ccc", "dddd", "eeeee", "f", "gg"}
	vecs, err := c.BatchEmbed(context.Background(), texts, InputTypeDocument)
	if err != nil {
		t.Fatalf("BatchEmbed: %v", err)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("len(vecs) = %d, want %d", len(vecs), len(texts))
	}
	for i, text := range texts {
		if vecs[i][0] != float32(len(text)) {
			t.Errorf("vecs[%d] = %v, want embedding derived from %q", i, vecs[i], text)
		}
	}
	if len(requestSizes) != 3 { // 7 texts, batch of 3 -> 3,3,1
		t.Errorf("made %d requests (sizes %v), want 3", len(requestSizes), requestSizes)
	}
}
