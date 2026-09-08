package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/HasithaErandika/loregrep/src/internal/llm"
)

// These are the three LLM-backed decision points from
// docs/diagrams/agent-loop.md: plan (what to look up next), sufficient
// (enough to answer, or search again), synthesize (compose the final
// cited answer). Each asks the model for JSON constrained to a specific
// schema (llm.JSONSchema, OpenRouter's structured-output mode) so the
// result is parsed directly rather than scraped out of free text.

func (o *Orchestrator) plan(ctx context.Context, question string, trace []Observation) (ToolCall, error) {
	schema := planSchema(o.Tools.availableTools())
	resp, err := o.LLM.Complete(ctx, llm.CompletionRequest{
		Model: o.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: plannerSystemPrompt(o.Tools.availableTools())},
			{Role: llm.RoleUser, Content: planUserPrompt(question, trace)},
		},
		Temperature: 0,
		JSONSchema:  &schema,
	})
	if err != nil {
		return ToolCall{}, fmt.Errorf("agent: planner completion: %w", err)
	}

	var call ToolCall
	if err := json.Unmarshal([]byte(resp.Content), &call); err != nil {
		return ToolCall{}, fmt.Errorf("agent: parsing planner output %q: %w", resp.Content, err)
	}
	return call, nil
}

type sufficiencyDecision struct {
	Sufficient bool   `json:"sufficient"`
	Reasoning  string `json:"reasoning"`
}

func (o *Orchestrator) sufficient(ctx context.Context, question string, trace []Observation) (bool, error) {
	resp, err := o.LLM.Complete(ctx, llm.CompletionRequest{
		Model: o.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: sufficiencySystemPrompt},
			{Role: llm.RoleUser, Content: sufficiencyUserPrompt(question, trace)},
		},
		Temperature: 0,
		JSONSchema:  &sufficiencySchema,
	})
	if err != nil {
		return false, fmt.Errorf("agent: sufficiency completion: %w", err)
	}

	var decision sufficiencyDecision
	if err := json.Unmarshal([]byte(resp.Content), &decision); err != nil {
		return false, fmt.Errorf("agent: parsing sufficiency output %q: %w", resp.Content, err)
	}
	return decision.Sufficient, nil
}

func (o *Orchestrator) synthesize(ctx context.Context, question string, trace []Observation) (Answer, error) {
	resp, err := o.LLM.Complete(ctx, llm.CompletionRequest{
		Model: o.Model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: synthesizerSystemPrompt},
			{Role: llm.RoleUser, Content: synthesizerUserPrompt(question, trace)},
		},
		Temperature: 0.2,
		JSONSchema:  &synthesisSchema,
	})
	if err != nil {
		return Answer{}, fmt.Errorf("agent: synthesis completion: %w", err)
	}

	var parsed struct {
		Answer    string     `json:"answer"`
		Citations []Citation `json:"citations"`
		Conflicts []string   `json:"conflicts"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return Answer{}, fmt.Errorf("agent: parsing synthesis output %q: %w", resp.Content, err)
	}

	return Answer{
		Text:      parsed.Answer,
		Citations: parsed.Citations,
		Conflicts: parsed.Conflicts,
	}, nil
}

// --- Prompts -----------------------------------------------------------

func plannerSystemPrompt(tools []ToolName) string {
	var b strings.Builder
	b.WriteString("You are the planner in a document research agent over the Ashen Era Archive. ")
	b.WriteString("On each turn you pick exactly one search tool and its arguments to move toward answering the user's question. ")
	b.WriteString("Prefer the most specific tool for what's needed; keyword_search is the default first choice. Available tools:\n")
	for _, t := range tools {
		b.WriteString("- ")
		b.WriteString(toolDescriptions[t])
		b.WriteString("\n")
	}
	return b.String()
}

func planUserPrompt(question string, trace []Observation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n\n", question)
	if len(trace) == 0 {
		b.WriteString("No searches have been run yet. Pick the first tool to call.")
		return b.String()
	}
	b.WriteString("Searches so far:\n")
	writeTrace(&b, trace)
	b.WriteString("\nPick the next tool to call. Don't repeat a search that already returned nothing useful with the same arguments.")
	return b.String()
}

const sufficiencySystemPrompt = "You check whether an agent researching a question in the Ashen Era Archive " +
	"has gathered enough information to answer it well, or needs to search again. " +
	"Say insufficient if a claim central to the answer is still unsupported by any search result so far."

func sufficiencyUserPrompt(question string, trace []Observation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n\nSearches so far:\n", question)
	writeTrace(&b, trace)
	b.WriteString("\nIs this enough to answer the question well?")
	return b.String()
}

const synthesizerSystemPrompt = "You compose the final answer for a document research agent over the Ashen Era Archive. " +
	"Answer only from the search results provided — never from outside knowledge. Cite every claim with its source " +
	"document (doc_id) and page or section. The corpus deliberately includes unreliable in-world narrators " +
	"(reliability_tier: in_world_unreliable for ephemera, official for codex, reference for wiki, narrative for chronicles) " +
	"— when two sources disagree on a fact, state both and note the disagreement in conflicts rather than silently " +
	"picking one. If the search results don't actually support an answer, say so plainly instead of guessing."

func synthesizerUserPrompt(question string, trace []Observation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n\nSearch results:\n", question)
	writeTrace(&b, trace)
	return b.String()
}

// writeTrace renders every observation's chunks (with reliability tier and
// citation info) and tool-specific notes, so each LLM step reads the same
// evidence.
func writeTrace(b *strings.Builder, trace []Observation) {
	for i, obs := range trace {
		fmt.Fprintf(b, "\n[%d] %s(%s)\n", i+1, obs.Call.Tool, callArgs(obs.Call))
		if len(obs.Chunks) == 0 && len(obs.Notes) == 0 {
			b.WriteString("  (no results)\n")
			continue
		}
		for _, n := range obs.Notes {
			fmt.Fprintf(b, "  - %s\n", n)
		}
		for _, c := range obs.Chunks {
			page := "no page"
			if c.Page != nil {
				page = fmt.Sprintf("p.%d", *c.Page)
			}
			section := ""
			if c.Section != nil {
				section = " § " + *c.Section
			}
			text := c.SearchText()
			if len(text) > 500 {
				text = text[:500] + "…"
			}
			fmt.Fprintf(b, "  - [%s, %s%s, %s] %s\n", c.DocID, page, section, c.ReliabilityTier, text)
		}
	}
}

func callArgs(call ToolCall) string {
	switch call.Tool {
	case ToolKeywordSearch, ToolSemanticSearch:
		return fmt.Sprintf("%q", call.Query)
	case ToolTableLookup:
		return fmt.Sprintf("%q, %q", call.Entity, call.Attribute)
	case ToolFollowReference:
		return fmt.Sprintf("%q", call.Entity)
	default:
		return ""
	}
}

// --- JSON schemas --------------------------------------------------------

func planSchema(tools []ToolName) llm.JSONSchema {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = string(t)
	}
	return llm.JSONSchema{
		Name: "tool_call",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool":      map[string]any{"type": "string", "enum": names},
				"query":     map[string]any{"type": "string"},
				"entity":    map[string]any{"type": "string"},
				"attribute": map[string]any{"type": "string"},
				"reasoning": map[string]any{"type": "string"},
			},
			"required":             []string{"tool", "query", "entity", "attribute", "reasoning"},
			"additionalProperties": false,
		},
	}
}

var sufficiencySchema = llm.JSONSchema{
	Name: "sufficiency_decision",
	Schema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"sufficient": map[string]any{"type": "boolean"},
			"reasoning":  map[string]any{"type": "string"},
		},
		"required":             []string{"sufficient", "reasoning"},
		"additionalProperties": false,
	},
}

var synthesisSchema = llm.JSONSchema{
	Name: "synthesized_answer",
	Schema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
			"citations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"doc_id":  map[string]any{"type": "string"},
						"page":    map[string]any{"type": []string{"integer", "null"}},
						"section": map[string]any{"type": "string"},
					},
					"required":             []string{"doc_id", "page", "section"},
					"additionalProperties": false,
				},
			},
			"conflicts": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required":             []string{"answer", "citations", "conflicts"},
		"additionalProperties": false,
	},
}
