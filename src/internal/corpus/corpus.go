// Package corpus loads and represents the Stage 1 extraction artifact
// (data/chunks.json) that Stage 2 indexes and serves. See
// docs/decisions.md for the chunk schema's provenance.
package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Chunk is one unit of the extracted corpus: either prose text or a table,
// never both. Pointer fields are nullable in the source JSON and stay
// nullable here rather than being defaulted to zero values, since e.g.
// page:null (docx/md/txt) is meaningfully different from page:0.
type Chunk struct {
	ID                   string     `json:"id"`
	DocID                string     `json:"doc_id"`
	DocTitle             string     `json:"doc_title"`
	Category             string     `json:"category"`
	Format               string     `json:"format"`
	ReliabilityTier      string     `json:"reliability_tier"`
	Page                 *int       `json:"page"`
	Section              *string    `json:"section"`
	ChunkType            string     `json:"chunk_type"`
	Text                 *string    `json:"text"`
	Table                [][]string `json:"table"`
	Entities             []string   `json:"entities"`
	ExtractionMethod     string     `json:"extraction_method"`
	ExtractionConfidence *float64   `json:"extraction_confidence"`
	LowConfidence        bool       `json:"low_confidence"`
	CharCount            int        `json:"char_count"`
}

// SearchText returns the chunk's searchable body: its prose text, or a
// flattened rendering of its table when chunk_type is "table" (text is
// null there). Returns "" for a chunk with neither (e.g. discarded
// below-confidence OCR).
func (c Chunk) SearchText() string {
	if c.Text != nil {
		return *c.Text
	}
	if c.Table != nil {
		var b strings.Builder
		for _, row := range c.Table {
			b.WriteString(strings.Join(row, " "))
			b.WriteString("\n")
		}
		return b.String()
	}
	return ""
}

// Meta carries the corpus-level stats build_artifact.py computed while
// producing chunks.json, so Stage 2 doesn't need to rescan every chunk to
// answer "how many documents/chunks/entities are there."
type Meta struct {
	GeneratedAt             string         `json:"generated_at"`
	ArchiveRoot             string         `json:"archive_root"`
	DocumentCount           int            `json:"document_count"`
	ChunkCount              int            `json:"chunk_count"`
	EntityCount             int            `json:"entity_count"`
	ByCategory              map[string]int `json:"by_category"`
	ByFormat                map[string]int `json:"by_format"`
	ByExtractionMethod      map[string]int `json:"by_extraction_method"`
	LowConfidenceChunkCount int            `json:"low_confidence_chunk_count"`
	OCRAvailable            bool           `json:"ocr_available"`
	SampleMode              *int           `json:"sample_mode"`
}

// Corpus is the in-memory form of data/chunks.json.
type Corpus struct {
	Meta   Meta
	Chunks []Chunk
}

// Load reads and parses a chunks.json artifact from path.
func Load(path string) (*Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("corpus: reading %s: %w", path, err)
	}

	var raw struct {
		Meta   Meta    `json:"meta"`
		Chunks []Chunk `json:"chunks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("corpus: parsing %s: %w", path, err)
	}

	return &Corpus{Meta: raw.Meta, Chunks: raw.Chunks}, nil
}
