package agent

import (
	"context"
	"fmt"

	"github.com/HasithaErandika/loregrep/src/internal/llm"
)

// MaxIterations caps the planner/tool-router loop so a live demo can never
// hang on a question that needs more hops than expected. A question that
// genuinely needs more gets a best-effort answer from whatever was found
// instead — see docs/limitations.md and docs/diagrams/agent-loop.md.
const MaxIterations = 6

// Citation is one claim's source, in the doc+page/section form
// docs/architecture.md's synthesizer is meant to produce. Page is nil for
// chunks with no native page boundary (docx/md/txt — see
// docs/decisions.md's page:null decision); Section is the citation anchor
// in that case.
type Citation struct {
	DocID   string `json:"doc_id"`
	Page    *int   `json:"page,omitempty"`
	Section string `json:"section,omitempty"`
}

// Answer is the orchestrator's final output for one question.
type Answer struct {
	Text      string
	Citations []Citation
	// Conflicts flags disagreements between sources the synthesizer
	// noticed (e.g. two ephemera documents giving different numbers for
	// the same fact) rather than silently picking one — see
	// docs/limitations.md's reliability_tier discussion.
	Conflicts []string
	// Trace is every tool call made, in order — the "live reasoning
	// trace" the API is meant to stream per docs/architecture.md.
	Trace []Observation
	// HitIterationCap is true if the loop stopped because MaxIterations
	// was reached rather than because the sufficiency check passed — the
	// answer may be incomplete; see docs/limitations.md.
	HitIterationCap bool
}

// Orchestrator runs the planner -> tool router -> sufficiency check ->
// synthesizer loop from docs/diagrams/agent-loop.md.
type Orchestrator struct {
	Tools Toolset
	LLM   llm.LLMClient
	Model string
}

// New builds an Orchestrator over tools, using client (at model) for the
// planner, sufficiency checker, and synthesizer steps.
func New(tools Toolset, client llm.LLMClient, model string) *Orchestrator {
	return &Orchestrator{Tools: tools, LLM: client, Model: model}
}

// Ask runs the full loop for one question: plan a tool call, execute it,
// check sufficiency, repeat until sufficient or MaxIterations is reached,
// then synthesize a cited answer from everything gathered.
func (o *Orchestrator) Ask(ctx context.Context, question string) (Answer, error) {
	var trace []Observation
	hitCap := true

	for i := 0; i < MaxIterations; i++ {
		call, err := o.plan(ctx, question, trace)
		if err != nil {
			return Answer{}, fmt.Errorf("agent: planning step %d: %w", i, err)
		}

		obs, err := o.Tools.dispatch(ctx, call)
		if err != nil {
			return Answer{}, fmt.Errorf("agent: executing %s (step %d): %w", call.Tool, i, err)
		}
		trace = append(trace, obs)

		sufficient, err := o.sufficient(ctx, question, trace)
		if err != nil {
			return Answer{}, fmt.Errorf("agent: sufficiency check (step %d): %w", i, err)
		}
		if sufficient {
			hitCap = false
			break
		}
	}

	answer, err := o.synthesize(ctx, question, trace)
	if err != nil {
		return Answer{}, fmt.Errorf("agent: synthesizing: %w", err)
	}
	answer.Trace = trace
	answer.HitIterationCap = hitCap
	return answer, nil
}
