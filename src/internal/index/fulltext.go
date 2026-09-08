// Package index builds and queries the full-text search index (bleve,
// BM25) over the extracted corpus — the agent's default first-choice tool
// per docs/architecture.md.
package index

import (
	"fmt"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
)

// indexDoc is the flattened shape actually handed to bleve. bleve indexes
// exported struct fields via reflection, and corpus.Chunk's pointer/table
// fields (nullable page, table vs. text) don't map onto that directly, so
// each chunk is projected into this shape at index time.
type indexDoc struct {
	DocTitle        string
	Section         string
	Text            string
	Category        string
	ReliabilityTier string
	Entities        []string
}

// FullText is an in-memory (build-artifact-sized, not persisted to disk)
// bleve index over the corpus, plus the chunk data needed to turn a hit ID
// back into a full Chunk for the caller.
type FullText struct {
	idx    bleve.Index
	chunks map[string]corpus.Chunk
}

// SearchResult pairs a matched chunk with its BM25 score.
type SearchResult struct {
	Chunk corpus.Chunk
	Score float64
}

func buildMapping() mapping.IndexMapping {
	m := bleve.NewIndexMapping()
	// bm25 (vs. bleve's tf-idf default) per docs/architecture.md's
	// "full-text (bleve, BM25)" — see bleve's docs/scoring.md.
	m.ScoringModel = "bm25"
	return m
}

// NewFullText builds an in-memory full-text index over chunks. Chunks with
// no searchable text (e.g. OCR output discarded below the confidence
// floor, see docs/decisions.md) are skipped.
func NewFullText(chunks []corpus.Chunk) (*FullText, error) {
	idx, err := bleve.NewMemOnly(buildMapping())
	if err != nil {
		return nil, fmt.Errorf("index: creating full-text index: %w", err)
	}

	ft := &FullText{idx: idx, chunks: make(map[string]corpus.Chunk, len(chunks))}

	batch := idx.NewBatch()
	for _, c := range chunks {
		text := c.SearchText()
		if text == "" {
			continue
		}

		section := ""
		if c.Section != nil {
			section = *c.Section
		}

		doc := indexDoc{
			DocTitle:        c.DocTitle,
			Section:         section,
			Text:            text,
			Category:        c.Category,
			ReliabilityTier: c.ReliabilityTier,
			Entities:        c.Entities,
		}
		if err := batch.Index(c.ID, doc); err != nil {
			return nil, fmt.Errorf("index: batching chunk %s: %w", c.ID, err)
		}
		ft.chunks[c.ID] = c
	}

	if err := idx.Batch(batch); err != nil {
		return nil, fmt.Errorf("index: writing batch: %w", err)
	}

	return ft, nil
}

// KeywordSearch is the agent's keyword_search(query) tool: exact/fuzzy
// full-text search over chunk prose and table text, BM25-ranked, capped at
// limit results.
func (ft *FullText) KeywordSearch(query string, limit int) ([]SearchResult, error) {
	q := bleve.NewQueryStringQuery(query)
	req := bleve.NewSearchRequestOptions(q, limit, 0, false)

	res, err := ft.idx.Search(req)
	if err != nil {
		return nil, fmt.Errorf("index: searching %q: %w", query, err)
	}

	results := make([]SearchResult, 0, len(res.Hits))
	for _, hit := range res.Hits {
		chunk, ok := ft.chunks[hit.ID]
		if !ok {
			continue
		}
		results = append(results, SearchResult{Chunk: chunk, Score: hit.Score})
	}

	return results, nil
}

// Len returns the number of chunks actually indexed (i.e. excluding those
// skipped for having no searchable text).
func (ft *FullText) Len() int {
	return len(ft.chunks)
}
