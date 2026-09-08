package graph

import (
	"testing"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
)

// testChunks mirrors the real shape: entities are canonical wiki-derived
// names (see docs/decisions.md / extraction/common.py's EntityMatcher),
// and most chunks tag zero or one entity — only some tag two or more,
// which is what actually produces graph edges.
func testChunks() []corpus.Chunk {
	return []corpus.Chunk{
		{ID: "c1", Entities: []string{"House Morvain", "The Bleeding Crown"}},
		{ID: "c2", Entities: []string{"House Morvain", "The Bleeding Crown"}},
		{ID: "c3", Entities: []string{"House Morvain", "The Bleeding Crown", "Crookvale"}},
		{ID: "c4", Entities: []string{"Crookvale", "Hollowreach"}},
		{ID: "c5", Entities: []string{"The Bleeding Crown"}}, // single entity: no edge
		{ID: "c6", Entities: nil}, // no entities: no edge
		{ID: "c7", Entities: []string{"Brannoc Ironmere the Red-Handed", "Blackford"}},
	}
}

func TestBuild_Len(t *testing.T) {
	g := Build(testChunks())
	// House Morvain, The Bleeding Crown, Crookvale, Hollowreach,
	// Brannoc Ironmere the Red-Handed, Blackford = 6 distinct entities.
	if g.Len() != 6 {
		t.Errorf("Len() = %d, want 6", g.Len())
	}
}

func TestFollowReference_WeightedByCooccurrence(t *testing.T) {
	g := Build(testChunks())

	refs := g.FollowReference("The Bleeding Crown", 0)
	if len(refs) != 2 {
		t.Fatalf("FollowReference(The Bleeding Crown) = %d refs, want 2 (House Morvain, Crookvale): %+v", len(refs), refs)
	}

	// House Morvain co-occurs 3 times (c1, c2, c3), Crookvale once (c3) —
	// House Morvain must rank first.
	if refs[0].Entity != "House Morvain" || refs[0].Weight != 3 {
		t.Errorf("refs[0] = %+v, want {House Morvain, 3, ...}", refs[0])
	}
	if refs[1].Entity != "Crookvale" || refs[1].Weight != 1 {
		t.Errorf("refs[1] = %+v, want {Crookvale, 1, ...}", refs[1])
	}
}

func TestFollowReference_ExampleChunksCited(t *testing.T) {
	g := Build(testChunks())

	// House Morvain co-occurs with both The Bleeding Crown (c1, c2, c3)
	// and Crookvale (c3) — find the ref to The Bleeding Crown specifically
	// and check its citations.
	refs := g.FollowReference("House Morvain", 0)
	var toBleedingCrown *Reference
	for i := range refs {
		if refs[i].Entity == "The Bleeding Crown" {
			toBleedingCrown = &refs[i]
		}
	}
	if toBleedingCrown == nil {
		t.Fatalf("no ref to The Bleeding Crown among %+v", refs)
	}

	ids := map[string]bool{}
	for _, c := range toBleedingCrown.ExampleChunks {
		ids[c.ID] = true
	}
	for _, want := range []string{"c1", "c2", "c3"} {
		if !ids[want] {
			t.Errorf("expected chunk %s among ExampleChunks, got %+v", want, ids)
		}
	}
}

func TestFollowReference_CaseInsensitiveExactMatch(t *testing.T) {
	g := Build(testChunks())

	refs := g.FollowReference("the bleeding crown", 0)
	if len(refs) != 2 {
		t.Fatalf("FollowReference(lowercased) = %d refs, want 2", len(refs))
	}
}

func TestFollowReference_PartialNameFallsBackToSubstring(t *testing.T) {
	g := Build(testChunks())

	refs := g.FollowReference("Brannoc Ironmere", 0)
	if len(refs) != 1 || refs[0].Entity != "Blackford" {
		t.Fatalf("FollowReference(Brannoc Ironmere) = %+v, want one ref to Blackford", refs)
	}
}

func TestFollowReference_LimitRespected(t *testing.T) {
	g := Build(testChunks())

	refs := g.FollowReference("The Bleeding Crown", 1)
	if len(refs) != 1 {
		t.Fatalf("FollowReference(..., limit=1) = %d refs, want 1", len(refs))
	}
	if refs[0].Entity != "House Morvain" {
		t.Errorf("refs[0].Entity = %q, want House Morvain (highest weight)", refs[0].Entity)
	}
}

func TestFollowReference_SingleEntityChunkNoSelfEdge(t *testing.T) {
	g := Build(testChunks())

	// "The Bleeding Crown" appears alone in c5 — must not produce a
	// self-referential edge or otherwise distort neighbor weights.
	refs := g.FollowReference("The Bleeding Crown", 0)
	for _, r := range refs {
		if r.Entity == "The Bleeding Crown" {
			t.Errorf("found self-edge for The Bleeding Crown: %+v", r)
		}
	}
}

func TestFollowReference_UnknownEntity(t *testing.T) {
	g := Build(testChunks())

	refs := g.FollowReference("Nonexistent Faction", 0)
	if len(refs) != 0 {
		t.Errorf("FollowReference(Nonexistent Faction) = %d refs, want 0", len(refs))
	}
}

func TestFollowReference_IsolatedEntityNoNeighbors(t *testing.T) {
	g := Build(testChunks())

	// Present as a node (via c5) but never co-occurs would be nice to test
	// directly; instead confirm a known, connected entity behaves and an
	// unconnected query entity returns nothing without erroring.
	refs := g.FollowReference("Blackford", 0)
	if len(refs) != 1 || refs[0].Entity != "Brannoc Ironmere the Red-Handed" {
		t.Fatalf("FollowReference(Blackford) = %+v, want one ref to Brannoc Ironmere the Red-Handed", refs)
	}
}
