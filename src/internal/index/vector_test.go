package index

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
	"github.com/HasithaErandika/loregrep/src/internal/llm"
)

func TestCosineSimilarity(t *testing.T) {
	cases := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1},
		{"scale invariant", []float32{1, 1}, []float32{2, 2}, 1},
		{"mismatched lengths", []float32{1, 2}, []float32{1}, 0},
		{"zero vector", []float32{0, 0}, []float32{1, 1}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cosineSimilarity(tc.a, tc.b)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("cosineSimilarity(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func vectorTestChunks() []corpus.Chunk {
	return []corpus.Chunk{
		{ID: "c1", DocTitle: "Ashreach"},
		{ID: "c2", DocTitle: "Blackford"},
		{ID: "c3", DocTitle: "Crookvale"},
	}
}

func TestVectorStore_SearchRanksBySimilarity(t *testing.T) {
	embeddings := map[string][]float32{
		"c1": {1, 0},     // identical to query
		"c2": {0, 1},     // orthogonal
		"c3": {0.9, 0.1}, // close, not identical
	}
	vs := NewVectorStore(vectorTestChunks(), embeddings)
	if vs.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", vs.Len())
	}

	results := vs.Search([]float32{1, 0}, 0)
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[0].Chunk.ID != "c1" {
		t.Errorf("results[0] = %+v, want c1 (identical vector) first", results[0])
	}
	if results[1].Chunk.ID != "c3" {
		t.Errorf("results[1] = %+v, want c3 (close vector) second", results[1])
	}
	if results[2].Chunk.ID != "c2" {
		t.Errorf("results[2] = %+v, want c2 (orthogonal) last", results[2])
	}
	if results[0].Score <= results[1].Score || results[1].Score <= results[2].Score {
		t.Errorf("scores not strictly descending: %v, %v, %v", results[0].Score, results[1].Score, results[2].Score)
	}
}

func TestVectorStore_SearchRespectsLimit(t *testing.T) {
	embeddings := map[string][]float32{
		"c1": {1, 0},
		"c2": {0, 1},
		"c3": {0.9, 0.1},
	}
	vs := NewVectorStore(vectorTestChunks(), embeddings)

	results := vs.Search([]float32{1, 0}, 2)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
}

func TestNewVectorStore_SkipsEmbeddingWithNoMatchingChunk(t *testing.T) {
	embeddings := map[string][]float32{
		"c1":          {1, 0},
		"nonexistent": {0, 1},
	}
	vs := NewVectorStore(vectorTestChunks(), embeddings)
	if vs.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 (nonexistent chunk skipped)", vs.Len())
	}
}

// fakeEmbeddingClient is a deterministic stand-in for llm.EmbeddingClient
// so SemanticIndex.Search is testable without a real Voyage API call.
type fakeEmbeddingClient struct {
	vectors map[string][]float32
	err     error
}

func (f *fakeEmbeddingClient) Embed(ctx context.Context, texts []string, inputType llm.InputType) ([][]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = f.vectors[t]
	}
	return out, nil
}

func TestSemanticIndex_Search(t *testing.T) {
	embeddings := map[string][]float32{
		"c1": {1, 0},
		"c2": {0, 1},
	}
	vs := NewVectorStore(vectorTestChunks()[:2], embeddings)
	client := &fakeEmbeddingClient{vectors: map[string][]float32{
		"what is Ashreach": {1, 0},
	}}
	si := NewSemanticIndex(vs, client)

	results, err := si.Search(context.Background(), "what is Ashreach", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Chunk.ID != "c1" {
		t.Fatalf("results = %+v, want [{c1 ...}]", results)
	}
}

func TestSemanticIndex_Search_ClientError(t *testing.T) {
	vs := NewVectorStore(vectorTestChunks(), nil)
	client := &fakeEmbeddingClient{err: errors.New("voyage unavailable")}
	si := NewSemanticIndex(vs, client)

	_, err := si.Search(context.Background(), "anything", 5)
	if err == nil {
		t.Fatal("Search: want error when embedding client fails, got nil")
	}
}

// fakeBatchEmbeddingClient stands in for llm.BatchEmbeddingClient so
// BuildVectorStore is testable without a real Voyage API call.
type fakeBatchEmbeddingClient struct {
	byText map[string][]float32
}

func (f *fakeBatchEmbeddingClient) BatchEmbed(ctx context.Context, texts []string, inputType llm.InputType) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = f.byText[t]
	}
	return out, nil
}

func TestBuildVectorStore_SkipsChunksWithNoSearchableText(t *testing.T) {
	text := "Sabelle Mournvale stood in the tribunal."
	chunks := []corpus.Chunk{
		{ID: "c1", ChunkType: "text", Text: &text},
		{ID: "c2", ChunkType: "image"}, // no Text, no Table -> SearchText() == ""
	}
	client := &fakeBatchEmbeddingClient{byText: map[string][]float32{text: {1, 2, 3}}}

	vs, err := BuildVectorStore(context.Background(), chunks, client)
	if err != nil {
		t.Fatalf("BuildVectorStore: %v", err)
	}
	if vs.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 (image chunk with no text skipped)", vs.Len())
	}

	results := vs.Search([]float32{1, 2, 3}, 0)
	if len(results) != 1 || results[0].Chunk.ID != "c1" {
		t.Fatalf("results = %+v, want [{c1 ...}]", results)
	}
}
