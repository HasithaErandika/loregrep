package index

import (
	"strings"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
)

// TableIndex indexes table chunks by the entity/subject each row-set
// describes, for the agent's table_lookup(entity, attribute) tool —
// structured queries against extracted codex tables, as distinct from
// keyword_search's free-text search over those same tables' flattened text.
type TableIndex struct {
	bySubject map[string][]corpus.Chunk // key: normalized subject
}

// TableResult is one matched (entity, attribute, value) row, with its
// source chunk kept for citation (doc_id, page/section, reliability_tier).
type TableResult struct {
	Entity    string
	Attribute string
	Value     string
	Chunk     corpus.Chunk
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// subject resolves the entity a table chunk's rows describe: an explicit
// "Name" row if the table has one (most character/relic/creature tables
// do), else the chunk's section with a "Registry:" prefix stripped (codex
// biography entries are sectioned "Registry: <name>"), else the document
// title. The document-title fallback also covers wiki infobox tables,
// whose section is either nil or the generic literal "Infobox" — neither
// identifies the entity — since one wiki page's table is always about that
// page's subject.
func subject(c corpus.Chunk) string {
	for _, row := range c.Table {
		if len(row) >= 2 && normalize(row[0]) == "name" {
			return strings.TrimSpace(row[1])
		}
	}
	if c.Section != nil && normalize(*c.Section) != "infobox" {
		if rest, ok := strings.CutPrefix(*c.Section, "Registry:"); ok {
			return strings.TrimSpace(rest)
		}
		return *c.Section
	}
	return c.DocTitle
}

// NewTableIndex indexes every table chunk in chunks by its resolved
// subject. Non-table chunks (and table chunks with no rows) are ignored.
func NewTableIndex(chunks []corpus.Chunk) *TableIndex {
	ti := &TableIndex{bySubject: make(map[string][]corpus.Chunk)}
	for _, c := range chunks {
		if c.ChunkType != "table" || len(c.Table) == 0 {
			continue
		}
		key := normalize(subject(c))
		ti.bySubject[key] = append(ti.bySubject[key], c)
	}
	return ti
}

// Lookup is the table_lookup(entity, attribute) agent tool: structured
// queries against extracted codex tables, returning every matching row
// across every table chunk about entity. An empty attribute returns every
// row for the entity, e.g. for "what do we know about X."
//
// Entity matching tries an exact (normalized) subject match first, falling
// back to substring matching in either direction (e.g. "Aldous Wrenfield"
// matches subject "Aldous Wrenfield the Last Warden") only when no exact
// match exists, so a precise query on a common short name isn't diluted by
// unrelated partial matches. Attribute matching follows the same
// exact-first, substring-fallback shape, since the corpus uses different
// labels for the same concept across documents (e.g. "Garrison strength"
// vs. "Former garrison strength").
func (ti *TableIndex) Lookup(entity, attribute string) []TableResult {
	chunks := ti.matchEntity(entity)
	attrKey := normalize(attribute)

	var exact, contains []TableResult
	for _, c := range chunks {
		subj := subject(c)
		for _, row := range c.Table {
			if len(row) < 2 {
				continue
			}
			label, value := row[0], row[1]

			if attribute == "" {
				exact = append(exact, TableResult{Entity: subj, Attribute: label, Value: value, Chunk: c})
				continue
			}

			labelKey := normalize(label)
			switch {
			case labelKey == attrKey:
				exact = append(exact, TableResult{Entity: subj, Attribute: label, Value: value, Chunk: c})
			case strings.Contains(labelKey, attrKey) || strings.Contains(attrKey, labelKey):
				contains = append(contains, TableResult{Entity: subj, Attribute: label, Value: value, Chunk: c})
			}
		}
	}

	if len(exact) > 0 {
		return exact
	}
	return contains
}

func (ti *TableIndex) matchEntity(entity string) []corpus.Chunk {
	key := normalize(entity)
	if chunks, ok := ti.bySubject[key]; ok {
		return chunks
	}

	var matched []corpus.Chunk
	for subj, chunks := range ti.bySubject {
		if strings.Contains(subj, key) || strings.Contains(key, subj) {
			matched = append(matched, chunks...)
		}
	}
	return matched
}
