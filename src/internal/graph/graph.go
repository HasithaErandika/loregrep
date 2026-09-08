// Package graph builds and queries the cross-reference graph over the
// corpus's tagged entities, for the agent's follow_reference(entity) tool
// per docs/architecture.md — used when a question needs "what else
// connects to X."
package graph

import (
	"sort"
	"strings"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
)

// maxCitationsPerEdge caps how many example chunks are kept per
// entity-pair, so a very common pair (e.g. a faction and its home region)
// doesn't drag hundreds of chunk copies through the graph.
const maxCitationsPerEdge = 10

// edge accumulates the co-occurrence weight and example chunks for one
// unordered pair of entities.
type edge struct {
	weight int
	chunks []corpus.Chunk
}

// Graph is the cross-reference graph: an entity is a node, and two
// entities are connected whenever Stage 1 tagged both of them into the
// same chunk. Built entirely from data/chunks.json's per-chunk `entities`
// field — no raw corpus or wiki markup access needed at runtime. Coverage
// is exactly as good as Stage 1's entity tagging, no better — see
// docs/limitations.md ("entity tagging is exact-name string matching
// against the wiki-derived entity list," not coreference resolution).
type Graph struct {
	edges map[string]map[string]*edge // normalized entity -> normalized neighbor -> edge
	names map[string]string           // normalized entity -> canonical display name
}

// Reference is one neighbor of a queried entity: how it connects, how
// often, and example chunks to cite.
type Reference struct {
	Entity        string
	Weight        int
	ExampleChunks []corpus.Chunk
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Build indexes every chunk's co-tagged entity pairs into a graph. A chunk
// tagging three or more entities contributes an edge between every pair,
// not just adjacent ones, since "mentioned in the same chunk" is symmetric
// and has no inherent order.
func Build(chunks []corpus.Chunk) *Graph {
	g := &Graph{
		edges: make(map[string]map[string]*edge),
		names: make(map[string]string),
	}

	for _, c := range chunks {
		ents := c.Entities
		for _, e := range ents {
			g.names[normalize(e)] = e
		}
		for i := 0; i < len(ents); i++ {
			for j := i + 1; j < len(ents); j++ {
				g.link(ents[i], ents[j], c)
			}
		}
	}

	return g
}

func (g *Graph) link(a, b string, c corpus.Chunk) {
	na, nb := normalize(a), normalize(b)
	if na == nb {
		return
	}
	g.connect(na, nb, c)
	g.connect(nb, na, c)
}

func (g *Graph) connect(from, to string, c corpus.Chunk) {
	if g.edges[from] == nil {
		g.edges[from] = make(map[string]*edge)
	}
	e := g.edges[from][to]
	if e == nil {
		e = &edge{}
		g.edges[from][to] = e
	}
	e.weight++
	if len(e.chunks) < maxCitationsPerEdge {
		e.chunks = append(e.chunks, c)
	}
}

// FollowReference is the follow_reference(entity) agent tool: every other
// entity co-tagged with entity in at least one chunk, ranked by
// co-occurrence weight (most connected first, then alphabetically for
// determinism), capped at limit (no cap if limit <= 0).
//
// Entity matching tries an exact (normalized) match first, falling back to
// substring matching in either direction — and merging every match's
// edges — only when there's no exact match. Same shape as
// TableIndex.Lookup in src/internal/index/table.go, for the same reason: a
// precise query on a common short name shouldn't silently pick one of
// several plausible matches.
func (g *Graph) FollowReference(entity string, limit int) []Reference {
	keys := g.matchEntities(entity)
	if len(keys) == 0 {
		return nil
	}

	agg := make(map[string]*Reference)
	for _, key := range keys {
		for nk, e := range g.edges[key] {
			r := agg[nk]
			if r == nil {
				r = &Reference{Entity: g.names[nk]}
				agg[nk] = r
			}
			r.Weight += e.weight
			r.ExampleChunks = append(r.ExampleChunks, e.chunks...)
		}
	}

	refs := make([]Reference, 0, len(agg))
	for _, r := range agg {
		if len(r.ExampleChunks) > maxCitationsPerEdge {
			r.ExampleChunks = r.ExampleChunks[:maxCitationsPerEdge]
		}
		refs = append(refs, *r)
	}

	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Weight != refs[j].Weight {
			return refs[i].Weight > refs[j].Weight
		}
		return refs[i].Entity < refs[j].Entity
	})

	if limit > 0 && len(refs) > limit {
		refs = refs[:limit]
	}
	return refs
}

func (g *Graph) matchEntities(entity string) []string {
	key := normalize(entity)
	if _, ok := g.names[key]; ok {
		return []string{key}
	}

	var matched []string
	for nk := range g.names {
		if strings.Contains(nk, key) || strings.Contains(key, nk) {
			matched = append(matched, nk)
		}
	}
	return matched
}

// Len returns the number of distinct entities (nodes) in the graph.
func (g *Graph) Len() int {
	return len(g.names)
}
