# loregrep

**SLIIT Codefest 2026 — Sub-track 1C: Searching the Way a Human Does**

An agentic document research assistant that answers questions over the
Ashen Era Archive (415 documents, ~1,277 pages, five formats) by reasoning
through it step by step — planning, choosing a search tool, reading the
result, deciding if it has enough, and repeating until it has a real
answer — rather than one-shot embedding retrieval over top-k chunks.

See the full plan and reasoning in the project brief; key decisions are
logged as they're made in [`docs/decisions.md`](docs/decisions.md).

## Repo layout

```
├── Ashen_Era_Archive/    # provided source corpus (gitignored — see below)
├── extraction/            # Stage 1 — Python, build-time document parsing
├── data/
│   └── chunks.json         # committed artifact Stage 2 loads (regenerable)
├── src/                     # Stage 2 — Go, runtime agent + server
│   ├── cmd/server/
│   └── internal/
│       ├── index/            # bleve full-text + vector store
│       ├── graph/             # cross-reference graph
│       ├── agent/               # planner, tool router, sufficiency check, synthesizer
│       ├── llm/                   # OpenRouter client interface
│       └── api/                     # HTTP handlers, trace streaming
├── docs/
│   ├── architecture.md
│   ├── decisions.md
│   ├── limitations.md
│   └── diagrams/
│       ├── system-architecture.md
│       └── agent-loop.md
└── ai_usage/
    ├── ai-usage-disclosure.md
    └── chat-logs/
```

## Obtaining the corpus

`Ashen_Era_Archive/` is **not** committed to this repo — it's ~122MB of
provided input data (PDFs, DOCX, images) that nobody on the team authored,
so it's gitignored per git best practice for large input assets (see
`docs/decisions.md`). Only the derived `data/chunks.json` artifact is
committed, and that's all the live system needs.

To regenerate `chunks.json` from scratch, place the competition-provided
`Ashen_Era_Archive/` folder at the repo root (same layout as this README's
tree above) — it's excluded via `.gitignore`, not deleted, so this is just
a local, un-tracked copy each team member keeps.

## Running

**Regenerate the extraction artifact** (only needed if extraction logic
changes, or `Ashen_Era_Archive/` is present locally — `data/chunks.json` is
committed and up to date otherwise):

```
cd extraction
python build_artifact.py   # → ../data/chunks.json
```

**Run the live system** (no Python required):

```
cd src
go build ./cmd/server
./server
```

## Docs

- [`docs/architecture.md`](docs/architecture.md) — two-stage system, agent
  tools, data flow
- [`docs/decisions.md`](docs/decisions.md) — key engineering decisions and
  why, logged as they happen
- [`docs/limitations.md`](docs/limitations.md) — known weak spots, kept
  honest and current
- [`ai_usage/ai-usage-disclosure.md`](ai_usage/ai-usage-disclosure.md) — AI
  collaboration disclosure
