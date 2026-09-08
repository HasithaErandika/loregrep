package index

import (
	"testing"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
)

// tableTestChunks mirrors real shapes found in data/chunks.json (see
// docs/decisions.md): a location with two separate table chunks and no
// "Name" row (subject comes from section), a codex biography sectioned
// "Registry: <name>", the same character's wiki infobox (section
// "Infobox", subject falls back to doc title) sharing some but not all
// attribute labels with the codex version, and a relic with duplicate
// "Attunement cost" rows across two chunks.
func tableTestChunks() []corpus.Chunk {
	return []corpus.Chunk{
		{
			ID:        "codex/gazetteer.docx::000",
			DocID:     "codex/gazetteer.docx",
			DocTitle:  "Codex Vaeloria I: Gazetteer of the Sundered Realms",
			ChunkType: "table",
			Section:   strPtr("Ashreach"),
			Table: [][]string{
				{"Classification", "Record"},
				{"Region", "The Weeping Marshes"},
				{"Status", "contested"},
				{"Ruling power", "The Bleeding Crown"},
				{"Garrison strength", "2598"},
			},
		},
		{
			ID:        "codex/gazetteer.docx::001",
			DocID:     "codex/gazetteer.docx",
			DocTitle:  "Codex Vaeloria I: Gazetteer of the Sundered Realms",
			ChunkType: "table",
			Section:   strPtr("Ashreach"),
			Table: [][]string{
				{"Gazetteer Note", "Determination"},
				{"Settlement character", "Marsh-bound, bleak, and unsettled"},
			},
		},
		{
			ID:        "codex/annals.docx::100",
			DocID:     "codex/annals.docx",
			DocTitle:  "The Annals of the Ashen Era",
			ChunkType: "table",
			Section:   strPtr("Registry: Aldous Wrenfield the Last Warden"),
			Table: [][]string{
				{"Field", "Record"},
				{"Name", "Aldous Wrenfield the Last Warden"},
				{"Role", "Reliquary Keeper"},
				{"Birth", "315 AS"},
			},
		},
		{
			ID:        "wiki/aldous.md::000",
			DocID:     "wiki/aldous_wrenfield_the_last_warden.md",
			DocTitle:  "Aldous Wrenfield the Last Warden",
			ChunkType: "table",
			Section:   strPtr("Infobox"),
			Table: [][]string{
				{"Field", "Value"},
				{"Role", "Reliquary Keeper"},
				{"Wields", "The Silent Psalter since 341 AS"},
			},
		},
		{
			ID:        "codex/armory.docx::010",
			DocID:     "codex/armory.docx",
			DocTitle:  "Codex Vaeloria II: Armory of Relics and Bestiary",
			ChunkType: "table",
			Section:   strPtr("Chalice of Ashdeep"),
			Table: [][]string{
				{"Classification", "Record"},
				{"Name", "Chalice of Ashdeep"},
				{"Attunement cost", "27"},
			},
		},
		{
			ID:        "codex/armory.docx::011",
			DocID:     "codex/armory.docx",
			DocTitle:  "Codex Vaeloria II: Armory of Relics and Bestiary",
			ChunkType: "table",
			Section:   strPtr("Chalice of Ashdeep"),
			Table: [][]string{
				{"Custodial Note", "Entry"},
				{"Required identification", "Chalice of Ashdeep"},
				{"Attunement cost", "27"},
			},
		},
		{
			// A prose chunk must never be treated as a table.
			ID:        "chronicles/x.docx::000",
			DocID:     "chronicles/x.docx",
			DocTitle:  "Some Chronicle",
			ChunkType: "text",
			Text:      strPtr("Attunement cost is mentioned here only in prose."),
		},
	}
}

func TestTableIndex_ExactEntityAndAttribute(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	results := ti.Lookup("Ashreach", "garrison strength")
	if len(results) != 1 {
		t.Fatalf("Lookup(Ashreach, garrison strength) = %d results, want 1: %+v", len(results), results)
	}
	if results[0].Value != "2598" {
		t.Errorf("Value = %q, want 2598", results[0].Value)
	}
	if results[0].Entity != "Ashreach" {
		t.Errorf("Entity = %q, want Ashreach", results[0].Entity)
	}
}

func TestTableIndex_MergesRowsAcrossChunksForSameEntity(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	// "Region" and "Settlement character" live in two different chunks
	// both sectioned "Ashreach" — an empty attribute must return both.
	results := ti.Lookup("Ashreach", "")
	if len(results) < 6 {
		t.Fatalf("Lookup(Ashreach, \"\") = %d rows, want at least 6 (both chunks' rows): %+v", len(results), results)
	}
}

func TestTableIndex_RegistryPrefixStripped(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	// Codex ("Registry: ...") and wiki ("Infobox") both carry a "Role" row
	// for the same person and agree on the value — both are expected back,
	// same as the duplicate "Attunement cost" case below; deduping
	// corroborating sources is the caller's job, not the index's.
	results := ti.Lookup("Aldous Wrenfield the Last Warden", "role")
	if len(results) != 2 {
		t.Fatalf("Lookup(Aldous Wrenfield the Last Warden, role) = %d results, want 2: %+v", len(results), results)
	}
	for _, r := range results {
		if r.Value != "Reliquary Keeper" {
			t.Errorf("Value = %q, want Reliquary Keeper", r.Value)
		}
	}
}

func TestTableIndex_SameSubjectMergesAcrossCodexAndWiki(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	// "Registry: Aldous Wrenfield the Last Warden" (codex) and doc title
	// "Aldous Wrenfield the Last Warden" (wiki infobox) must resolve to
	// the same subject key, so a lookup by full name pulls rows from both.
	results := ti.Lookup("Aldous Wrenfield the Last Warden", "")
	sources := map[string]bool{}
	for _, r := range results {
		sources[r.Chunk.DocID] = true
	}
	if !sources["codex/annals.docx"] || !sources["wiki/aldous_wrenfield_the_last_warden.md"] {
		t.Errorf("expected rows from both codex and wiki sources, got sources: %+v", sources)
	}
}

func TestTableIndex_PartialEntityNameFallsBackToSubstring(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	results := ti.Lookup("Aldous Wrenfield", "birth")
	if len(results) != 1 || results[0].Value != "315 AS" {
		t.Fatalf("Lookup(Aldous Wrenfield, birth) = %+v, want one result with value 315 AS", results)
	}
}

func TestTableIndex_DuplicateAttributeAcrossChunksBothReturned(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	results := ti.Lookup("Chalice of Ashdeep", "Attunement cost")
	if len(results) != 2 {
		t.Fatalf("Lookup(Chalice of Ashdeep, Attunement cost) = %d results, want 2: %+v", len(results), results)
	}
	for _, r := range results {
		if r.Value != "27" {
			t.Errorf("Value = %q, want 27", r.Value)
		}
	}
}

func TestTableIndex_AttributeSubstringFallback(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	// No table has an attribute labeled exactly "cost", only "Attunement
	// cost" — must still match via substring fallback.
	results := ti.Lookup("Chalice of Ashdeep", "cost")
	if len(results) != 2 {
		t.Fatalf("Lookup(Chalice of Ashdeep, cost) = %d results, want 2 via substring fallback: %+v", len(results), results)
	}
}

func TestTableIndex_ProseChunkNeverMatched(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	results := ti.Lookup("Some Chronicle", "attunement cost")
	if len(results) != 0 {
		t.Errorf("Lookup on a prose-only doc title = %d results, want 0: %+v", len(results), results)
	}
}

func TestTableIndex_UnknownEntity(t *testing.T) {
	ti := NewTableIndex(tableTestChunks())

	results := ti.Lookup("Nonexistent Place", "garrison strength")
	if len(results) != 0 {
		t.Errorf("Lookup(Nonexistent Place, ...) = %d results, want 0", len(results))
	}
}
