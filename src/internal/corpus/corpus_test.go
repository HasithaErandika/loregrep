package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chunks.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func TestLoad(t *testing.T) {
	path := writeFixture(t, `{
		"meta": {
			"generated_at": "2026-09-08T00:00:00Z",
			"archive_root": "Ashen_Era_Archive",
			"document_count": 2,
			"chunk_count": 2,
			"entity_count": 1,
			"by_category": {"codex": 1, "wiki": 1},
			"by_format": {"docx": 2},
			"by_extraction_method": {"python-docx": 2},
			"low_confidence_chunk_count": 0,
			"ocr_available": true,
			"sample_mode": null
		},
		"chunks": [
			{
				"id": "codex/a.docx::000",
				"doc_id": "codex/a.docx",
				"doc_title": "Codex A",
				"category": "codex",
				"format": "docx",
				"reliability_tier": "official",
				"page": null,
				"section": "Ashreach",
				"chunk_type": "table",
				"text": null,
				"table": [["Region", "The Weeping Marshes"], ["Garrison strength", "2598"]],
				"entities": ["The Bleeding Crown"],
				"extraction_method": "python-docx",
				"extraction_confidence": null,
				"low_confidence": false,
				"char_count": 40
			},
			{
				"id": "wiki/b.md::000",
				"doc_id": "wiki/b.md",
				"doc_title": "Wiki B",
				"category": "wiki",
				"format": "md",
				"reliability_tier": "reference",
				"page": null,
				"section": null,
				"chunk_type": "text",
				"text": "Sabelle Mournvale led the Ashen Vanguard.",
				"table": null,
				"entities": ["Sabelle Mournvale"],
				"extraction_method": "markdown",
				"extraction_confidence": null,
				"low_confidence": false,
				"char_count": 42
			}
		]
	}`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if c.Meta.DocumentCount != 2 {
		t.Errorf("Meta.DocumentCount = %d, want 2", c.Meta.DocumentCount)
	}
	if len(c.Chunks) != 2 {
		t.Fatalf("len(Chunks) = %d, want 2", len(c.Chunks))
	}

	table := c.Chunks[0]
	if table.Page != nil {
		t.Errorf("table chunk Page = %v, want nil", *table.Page)
	}
	if got := table.SearchText(); got != "Region The Weeping Marshes\nGarrison strength 2598\n" {
		t.Errorf("table chunk SearchText() = %q", got)
	}

	text := c.Chunks[1]
	if text.Section != nil {
		t.Errorf("text chunk Section = %v, want nil", *text.Section)
	}
	if got, want := text.SearchText(), "Sabelle Mournvale led the Ashen Vanguard."; got != want {
		t.Errorf("text chunk SearchText() = %q, want %q", got, want)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json")); err == nil {
		t.Fatal("Load: want error for missing file, got nil")
	}
}
