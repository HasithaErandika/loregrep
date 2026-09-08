// Package agent implements the planner -> tool router -> sufficiency
// check -> synthesizer loop from docs/diagrams/agent-loop.md: the agent
// iteratively picks a search tool, reads the result, and decides whether
// it has enough to answer, rather than running a fixed number of
// retrieval passes.
package agent

import (
	"context"
	"fmt"

	"github.com/HasithaErandika/loregrep/src/internal/corpus"
	"github.com/HasithaErandika/loregrep/src/internal/graph"
	"github.com/HasithaErandika/loregrep/src/internal/index"
)

// DefaultResultLimit caps how many chunks a single tool call feeds back
// into the loop, so one broad query can't blow out the context the
// planner/synthesizer have to read.
const DefaultResultLimit = 5

// ToolName identifies one of the four agent tools from
// docs/architecture.md's tool table.
type ToolName string

const (
	ToolKeywordSearch   ToolName = "keyword_search"
	ToolTableLookup     ToolName = "table_lookup"
	ToolFollowReference ToolName = "follow_reference"
	ToolSemanticSearch  ToolName = "semantic_search"
)

// toolDescriptions documents each tool for the planner prompt (steps.go),
// matching docs/architecture.md's "Used when" column so the LLM gets the
// same guidance a human reading the docs would.
var toolDescriptions = map[ToolName]string{
	ToolKeywordSearch:   `keyword_search(query): exact/fuzzy full-text search (BM25). Default first choice for named entities and specific terms.`,
	ToolTableLookup:     `table_lookup(entity, attribute): structured lookup against extracted codex tables. Use when the question needs a specific data point (e.g. a number, a status), not prose.`,
	ToolFollowReference: `follow_reference(entity): traverses the cross-reference graph to find what else connects to an entity. Use when the question needs "what else relates to X."`,
	ToolSemanticSearch:  `semantic_search(query): vector similarity search. Last resort — use only when the question is conceptual/fuzzy and the other tools found nothing useful.`,
}

// Toolset bundles the four search tools the planner can choose between.
// Semantic may be nil when no embeddings have been built (no Voyage API
// key configured) — the planner is only ever told about tools that are
// actually available (see availableTools in steps.go), so a nil Semantic
// naturally keeps the LLM from picking it rather than needing a runtime
// check on every call.
type Toolset struct {
	FullText *index.FullText
	Table    *index.TableIndex
	Graph    *graph.Graph
	Semantic *index.SemanticIndex
}

func (ts Toolset) availableTools() []ToolName {
	tools := []ToolName{ToolKeywordSearch, ToolTableLookup, ToolFollowReference}
	if ts.Semantic != nil {
		tools = append(tools, ToolSemanticSearch)
	}
	return tools
}

// ToolCall is the planner's decision for one iteration: which tool to run
// and with what arguments. Query is used by keyword_search/semantic_search;
// Entity/Attribute by table_lookup/follow_reference (Attribute unused by
// follow_reference).
type ToolCall struct {
	Tool      ToolName `json:"tool"`
	Query     string   `json:"query,omitempty"`
	Entity    string   `json:"entity,omitempty"`
	Attribute string   `json:"attribute,omitempty"`
	Reasoning string   `json:"reasoning,omitempty"`
}

// Observation is what one tool call returned: chunks worth reading, plus
// tool-specific notes a chunk dump alone doesn't capture (table_lookup's
// resolved attribute/value, follow_reference's co-occurrence weight).
type Observation struct {
	Call   ToolCall
	Chunks []corpus.Chunk
	Notes  []string
}

// dispatch executes call against the toolset and normalizes the result
// into an Observation. Pure Go, no LLM call — only the planner,
// sufficiency check, and synthesizer steps (steps.go) talk to an
// llm.LLMClient.
func (ts Toolset) dispatch(ctx context.Context, call ToolCall) (Observation, error) {
	switch call.Tool {
	case ToolKeywordSearch:
		if ts.FullText == nil {
			return Observation{}, fmt.Errorf("agent: keyword_search: not configured")
		}
		results, err := ts.FullText.KeywordSearch(call.Query, DefaultResultLimit)
		if err != nil {
			return Observation{}, fmt.Errorf("agent: keyword_search: %w", err)
		}
		return observationFromSearchResults(call, results), nil

	case ToolTableLookup:
		if ts.Table == nil {
			return Observation{}, fmt.Errorf("agent: table_lookup: not configured")
		}
		results := ts.Table.Lookup(call.Entity, call.Attribute)
		obs := Observation{Call: call}
		for _, r := range results {
			obs.Chunks = append(obs.Chunks, r.Chunk)
			obs.Notes = append(obs.Notes, fmt.Sprintf("%s — %s: %s", r.Entity, r.Attribute, r.Value))
		}
		return obs, nil

	case ToolFollowReference:
		if ts.Graph == nil {
			return Observation{}, fmt.Errorf("agent: follow_reference: not configured")
		}
		refs := ts.Graph.FollowReference(call.Entity, DefaultResultLimit)
		obs := Observation{Call: call}
		for _, r := range refs {
			obs.Chunks = append(obs.Chunks, r.ExampleChunks...)
			obs.Notes = append(obs.Notes, fmt.Sprintf("%s connects to %s (co-occurs in %d chunks)", call.Entity, r.Entity, r.Weight))
		}
		return obs, nil

	case ToolSemanticSearch:
		if ts.Semantic == nil {
			return Observation{}, fmt.Errorf("agent: semantic_search: not configured (no embeddings/API key)")
		}
		results, err := ts.Semantic.Search(ctx, call.Query, DefaultResultLimit)
		if err != nil {
			return Observation{}, fmt.Errorf("agent: semantic_search: %w", err)
		}
		return observationFromSearchResults(call, results), nil

	default:
		return Observation{}, fmt.Errorf("agent: unknown tool %q", call.Tool)
	}
}

func observationFromSearchResults(call ToolCall, results []index.SearchResult) Observation {
	obs := Observation{Call: call}
	for _, r := range results {
		obs.Chunks = append(obs.Chunks, r.Chunk)
	}
	return obs
}
