package index

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
	"github.com/HasithaErandika/loregrep/src/internal/llm"
)

// VectorStore is an in-memory, brute-force cosine-similarity index over
// precomputed chunk embeddings — the semantic_search fallback tool per
// docs/architecture.md ("vector similarity — fallback only," used when a
// question is conceptual/fuzzy and keyword/table/graph tools found nothing
// useful). Brute force because the corpus is small enough (thousands, not
// millions, of chunks) that an ANN index would add complexity without a
// measurable speed win — see docs/architecture.md's own "sized for this
// corpus" framing.
type VectorStore struct {
	ids        []string
	embeddings [][]float32
	chunks     map[string]corpus.Chunk
}

// NewVectorStore builds a store from chunk IDs and their precomputed
// embeddings (see BuildVectorStore for how those get produced). An
// embedding whose id has no matching chunk is skipped rather than
// erroring — harmless, since it just means it can never be returned.
func NewVectorStore(chunks []corpus.Chunk, embeddings map[string][]float32) *VectorStore {
	byID := make(map[string]corpus.Chunk, len(chunks))
	for _, c := range chunks {
		byID[c.ID] = c
	}

	vs := &VectorStore{chunks: make(map[string]corpus.Chunk, len(embeddings))}
	for id, vec := range embeddings {
		c, ok := byID[id]
		if !ok {
			continue
		}
		vs.ids = append(vs.ids, id)
		vs.embeddings = append(vs.embeddings, vec)
		vs.chunks[id] = c
	}
	return vs
}

// Search returns up to limit chunks whose embeddings are most
// cosine-similar to query, highest similarity first (limit <= 0 means no
// cap).
func (vs *VectorStore) Search(query []float32, limit int) []SearchResult {
	type scored struct {
		idx   int
		score float64
	}
	scores := make([]scored, len(vs.embeddings))
	for i, vec := range vs.embeddings {
		scores[i] = scored{idx: i, score: cosineSimilarity(query, vec)}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })

	if limit > 0 && limit < len(scores) {
		scores = scores[:limit]
	}

	results := make([]SearchResult, len(scores))
	for i, s := range scores {
		results[i] = SearchResult{Chunk: vs.chunks[vs.ids[s.idx]], Score: s.score}
	}
	return results
}

// Len returns the number of embedded chunks in the store.
func (vs *VectorStore) Len() int {
	return len(vs.ids)
}

// cosineSimilarity returns 0 for mismatched-length or zero vectors rather
// than erroring — a defensive default since embeddings from a live API
// are outside this package's control, and a zero score just sorts a
// malformed entry to the bottom instead of ranking it.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// BuildVectorStore embeds every chunk's searchable text (see
// corpus.Chunk.SearchText — prose or a flattened table rendering) via
// client at input_type "document", and builds a VectorStore from the
// results. This is the expensive, once-per-corpus-version step — an
// OpenRouter/Voyage API key is required, so it isn't run automatically;
// see docs/decisions.md.
func BuildVectorStore(ctx context.Context, chunks []corpus.Chunk, client llm.BatchEmbeddingClient) (*VectorStore, error) {
	ids := make([]string, 0, len(chunks))
	texts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		t := c.SearchText()
		if t == "" {
			continue
		}
		ids = append(ids, c.ID)
		texts = append(texts, t)
	}

	vecs, err := client.BatchEmbed(ctx, texts, llm.InputTypeDocument)
	if err != nil {
		return nil, fmt.Errorf("index: embedding corpus: %w", err)
	}
	if len(vecs) != len(ids) {
		return nil, fmt.Errorf("index: embedding corpus: got %d embeddings for %d chunks", len(vecs), len(ids))
	}

	embeddings := make(map[string][]float32, len(ids))
	for i, id := range ids {
		embeddings[id] = vecs[i]
	}
	return NewVectorStore(chunks, embeddings), nil
}

// SemanticIndex is the semantic_search(query) agent tool: embeds the query
// via an EmbeddingClient (at input_type "query", for Voyage's asymmetric
// document/query embeddings) and finds the most cosine-similar chunks in a
// VectorStore. Kept separate from VectorStore itself so the pure
// similarity math stays testable without any network dependency.
type SemanticIndex struct {
	store  *VectorStore
	client llm.EmbeddingClient
}

// NewSemanticIndex pairs a prebuilt VectorStore with the client used to
// embed queries against it.
func NewSemanticIndex(store *VectorStore, client llm.EmbeddingClient) *SemanticIndex {
	return &SemanticIndex{store: store, client: client}
}

// Search embeds query and returns up to limit most similar chunks.
func (si *SemanticIndex) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	vecs, err := si.client.Embed(ctx, []string{query}, llm.InputTypeQuery)
	if err != nil {
		return nil, fmt.Errorf("index: embedding query %q: %w", query, err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("index: embedding query %q: no embedding returned", query)
	}
	return si.store.Search(vecs[0], limit), nil
}
