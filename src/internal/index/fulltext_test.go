package index

import (
	"testing"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
)

func strPtr(s string) *string { return &s }

func testChunks() []corpus.Chunk {
	return []corpus.Chunk{
		{
			ID:              "chronicles/a.docx::000",
			DocID:           "chronicles/a.docx",
			DocTitle:        "The Ashen Chronicles, Volume I",
			Category:        "chronicles",
			ReliabilityTier: "narrative",
			Section:         strPtr("Chapter 1"),
			ChunkType:       "text",
			Text:            strPtr("Sabelle Mournvale the Twice-Crowned stood beneath the painted vault, High Inquisitor of the Ashen Vanguard."),
			Entities:        []string{"Sabelle Mournvale", "The Ashen Vanguard"},
		},
		{
			ID:              "codex/b.docx::000",
			DocID:           "codex/b.docx",
			DocTitle:        "Codex Vaeloria I",
			Category:        "codex",
			ReliabilityTier: "official",
			Section:         strPtr("Ashreach"),
			ChunkType:       "table",
			Table:           [][]string{{"Region", "The Weeping Marshes"}, {"Garrison strength", "2598"}},
			Entities:        []string{"The Bleeding Crown"},
		},
		{
			ID:              "wiki/c.md::000",
			DocID:           "wiki/c.md",
			DocTitle:        "Gloamreach",
			Category:        "wiki",
			ReliabilityTier: "reference",
			ChunkType:       "text",
			Text:            strPtr("Gloamreach is a garrison town on the coast, frequently at odds with the Ashen Vanguard."),
			Entities:        []string{"Gloamreach", "The Ashen Vanguard"},
		},
		{
			// No text and no table — e.g. OCR discarded below the
			// confidence floor. Must not be indexed or crash indexing.
			ID:        "wiki/images/atmo_01.png::000",
			DocID:     "wiki/images/atmo_01.png",
			DocTitle:  "Portrait",
			Category:  "wiki",
			ChunkType: "image",
		},
	}
}

func TestNewFullText_SkipsEmptyChunks(t *testing.T) {
	ft, err := NewFullText(testChunks())
	if err != nil {
		t.Fatalf("NewFullText: %v", err)
	}
	if ft.Len() != 3 {
		t.Errorf("Len() = %d, want 3 (image chunk with no text/table skipped)", ft.Len())
	}
}

func TestKeywordSearch_MatchesProse(t *testing.T) {
	ft, err := NewFullText(testChunks())
	if err != nil {
		t.Fatalf("NewFullText: %v", err)
	}

	results, err := ft.KeywordSearch("Sabelle Mournvale", 10)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Chunk.ID != "chronicles/a.docx::000" {
		t.Errorf("results[0].Chunk.ID = %q, want chronicles/a.docx::000", results[0].Chunk.ID)
	}
	if results[0].Score <= 0 {
		t.Errorf("results[0].Score = %v, want > 0", results[0].Score)
	}
}

func TestKeywordSearch_MatchesTableText(t *testing.T) {
	ft, err := NewFullText(testChunks())
	if err != nil {
		t.Fatalf("NewFullText: %v", err)
	}

	results, err := ft.KeywordSearch("garrison strength", 10)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Chunk.ID == "codex/b.docx::000" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected codex/b.docx::000 (table chunk) among results for %q, got %+v", "garrison strength", results)
	}
}

func TestKeywordSearch_SharedEntityMatchesBothDocs(t *testing.T) {
	ft, err := NewFullText(testChunks())
	if err != nil {
		t.Fatalf("NewFullText: %v", err)
	}

	results, err := ft.KeywordSearch("Ashen Vanguard", 10)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2 (chronicles + wiki chunks both mention it)", len(results))
	}
}

func TestKeywordSearch_NoMatch(t *testing.T) {
	ft, err := NewFullText(testChunks())
	if err != nil {
		t.Fatalf("NewFullText: %v", err)
	}

	results, err := ft.KeywordSearch("nonexistent-term-xyz", 10)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestKeywordSearch_LimitRespected(t *testing.T) {
	ft, err := NewFullText(testChunks())
	if err != nil {
		t.Fatalf("NewFullText: %v", err)
	}

	results, err := ft.KeywordSearch("Ashen Vanguard", 1)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1 (limit)", len(results))
	}
}
