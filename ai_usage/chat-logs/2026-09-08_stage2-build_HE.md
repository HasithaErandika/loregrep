# 2026-09-08 — Stage 2 build: loader through agent orchestrator (Claude Code)

**Participant:** HE (wickramasinghe.erandika@gmail.com)
**Tool:** Claude Code (Claude Sonnet 5)
**Topic:** Building Stage 2 (`src/`) piece by piece — corpus loader,
`keyword_search`, `table_lookup`, `follow_reference`, `semantic_search`, the
agent orchestrator, and the OpenRouter/Voyage clients — correcting
`README.md` along the way, and logging this session per the process below.

## Summary (human-written, condensed from the session)

One continuous session, worked in the dependency order loader → indexes →
tools → agent (per `docs/architecture.md`). At each step Claude Code was
asked "what next to build," proposed the next piece with a short tradeoff
(e.g. graph before semantic search, since the graph needs no external API
and semantic search does), and only proceeded after being told which to
build.

**What Claude Code did, by piece:**
- **Corpus loader + `keyword_search`** (`internal/corpus`,
  `internal/index/fulltext.go`) — bleve full-text index, BM25 scoring set
  explicitly per `docs/architecture.md`. Table chunks (where `text` is
  `null`, content lives only in `table`) are flattened into searchable
  text so they're not invisible to keyword search.
- **`table_lookup`** (`internal/index/table.go`) — entity/attribute lookup
  over codex tables, resolving each table's subject via a `Name` row, a
  `Registry:`-prefixed section, or the document title.
- **Cross-reference graph + `follow_reference`** (`internal/graph`) — built
  from entity co-occurrence within `chunks.json`'s already-tagged
  `entities` field, deliberately **not** from the wiki's `[[link]]`
  markup, since that markup is stripped before a chunk is ever written and
  rebuilding it would mean Stage 2 reading raw `Ashen_Era_Archive/` at
  runtime — breaking the extraction/serving seam grading depends on.
- **`semantic_search`** (`internal/llm/voyage.go`,
  `internal/index/vector.go`) — Voyage embeddings client (retry/backoff,
  batching, `input_type` set per Voyage's asymmetric document/query
  embeddings) plus a brute-force cosine `VectorStore`.
- **Agent orchestrator + OpenRouter `LLMClient`** (`internal/agent`,
  `internal/llm/openrouter.go`) — the full plan → dispatch → sufficiency
  check → synthesize loop from `docs/diagrams/agent-loop.md`, with every
  LLM decision point constrained to a JSON schema (OpenRouter strict mode)
  rather than parsed from free text.
- **`README.md` correction** — the "Getting started"/"Running" sections
  still said "no implementation yet" and pointed at a `cmd/server` binary
  that doesn't exist. Rewrote both, added a dependency-ordered status
  table, and fixed the repo-layout tree and `go.mod`'s bleve
  indirect→direct marking (`go mod tidy`).

**Correction/decision points (the actual scored content of this log):**

- **Explicit scope redirection: skip tests for the agent/LLMClient piece.**
  Human said "no need for test, Build theoretically and correctly
  engineered" for the orchestrator and OpenRouter client — a deliberate
  redirection away from the unit-test pattern used for every prior piece
  (corpus, index, graph, Voyage client all have real `*_test.go` files
  against synthetic fixtures or a mocked HTTP server). Followed exactly:
  no test files were added for `internal/agent` or `openrouter.go`.
  Compensated with in-process smoke runs (fake `LLMClient` returning
  canned schema-shaped JSON, real corpus, real `Toolset`) to at least
  confirm the control flow — normal happy path, and the `MaxIterations`
  safety cap actually stopping the loop — without claiming that proves the
  live OpenRouter integration works, since it hasn't been run against a
  real key.
- **Self-caught bug in `table_lookup`'s subject resolution, found before it
  shipped.** The first pass fell back to a table chunk's document title
  only when `section` was `nil`. Testing against real corpus shapes (not
  just hand-written fixtures) showed wiki infobox tables carry
  `section: "Infobox"` — a non-`nil` but still generic value — so every
  wiki table was resolving to the wrong subject ("Infobox" instead of the
  actual entity name). Fixed by also treating the literal string
  `"Infobox"` as non-identifying. Logged per this folder's own rule: a
  self-caught issue is still a correction worth recording, not just
  accepted output.
- **Committing was gated by the human at every step, not delegated.** At
  each piece, Claude Code asked "commit now?" and was told "No, leave
  staged" every time during this session; the loader and `table_lookup`
  commits that *do* exist in git history (`65cadbb`, `c0e4464`) were made
  by the human directly between turns, not by Claude Code inside this
  session. As of this log, the graph/semantic-search/agent work is staged
  but not committed — left for the team to review first.
- **External API shapes were verified against live docs, not assumed from
  memory**, before writing the Voyage and OpenRouter clients — fetched
  each provider's current API reference (endpoint, request/response JSON
  shape, structured-output field names) rather than risk shipping a
  client against a remembered-and-possibly-stale schema.
- **Process note on this log itself:** asked to use "relevant skills"
  from the human's own `dev-codex` skill repo for logging this session.
  Checked that repo's skill list — no skill there is actually about chat
  logging or AI-usage disclosure (`hackathon.md` covers project-management
  phases, not documentation format). The closest applicable guidance was
  `shorekeeper.md`'s session-documentation principles (conversational
  language, no raw transcript dumps, note decisions/corrections, stay
  concise) — applied here, and it already matches this project's own
  `ai_usage/chat-logs/README.md` convention, so no conflict to resolve.

## Outcome

All four agent tools (`keyword_search`, `table_lookup`, `follow_reference`,
`semantic_search`) and the agent orchestrator exist and build/vet cleanly.
`internal/corpus`, `internal/index`, `internal/graph`, and
`internal/llm/voyage.go` have passing unit tests; `internal/agent` and
`internal/llm/openrouter.go` do not, by explicit instruction, and were
instead verified with fake-client smoke runs against the real corpus.
`README.md` and `docs/decisions.md` were kept current at each step. Only
`cmd/server`/`internal/api` remain unbuilt. Nothing from this session's
graph/semantic-search/agent work has been committed yet — staged, pending
team review.
