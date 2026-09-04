# Architecture

Sub-track 1C — agentic document research assistant over the Ashen Era Archive
(415 documents, ~1,277 pages, five formats).

## Two-stage system

Extraction (offline, build-time) is separated from serving (online, graded)
at a natural seam: the processed corpus is a versioned artifact committed to
the repo, not something the live system regenerates. At grading time the
only required step is `go build && ./server` — zero Python involved.

```
┌─────────────────────────────────────────────────────────┐
│  STAGE 1 — Offline extraction (Python, build-time only)   │
│                                                             │
│  Document corpus (415 docs, 1,277 pages, 5 formats)        │
│         │                                                   │
│         ▼                                                   │
│  Parse: pdfplumber (tables) · PyMuPDF (text/images)         │
│         · python-docx · Tesseract/vision-LLM (scans)         │
│         │                                                   │
│         ▼                                                   │
│  Structured artifact: chunks.json                            │
│  (text, tables, page refs, entity tags, reliability tier)     │
└─────────────────────────────────┬───────────────────────────┘
                                    │  committed to repo
                                    ▼
┌─────────────────────────────────────────────────────────┐
│  STAGE 2 — Online serving (Go, single binary, live/graded)  │
│                                                             │
│  Loads chunks.json at startup                                │
│         │                                                   │
│  ┌─────────────────┐    ┌──────────────────┐                │
│  │ Indexes:          │    │ Agent orchestrator │              │
│  │ - full-text (bleve)│───▶│ - planner          │              │
│  │ - cross-ref graph  │    │ - tool router       │              │
│  │ - vector store      │    │ - sufficiency check │              │
│  │   (fallback only)    │    │ - synthesizer        │              │
│  └─────────────────┘    └──────────────────┘                │
│                                    │                          │
│                                    ▼                          │
│                        API + demo UI                          │
│                (live reasoning trace + citations)               │
└─────────────────────────────────────────────────────────┘
```

## Stage 1 — extraction (`extraction/`)

Python. Parses all five source formats (PDF, DOCX, Markdown, scanned image,
JSON) into a single structured artifact.

- `parse_pdfs.py` — text + table extraction via `pdfplumber`, page images via
  `PyMuPDF`
- `parse_docx.py` — `python-docx`
- `ocr_scans.py` — Tesseract CLI, or a vision-capable OpenRouter model when
  Tesseract output is unreliable
- `build_artifact.py` — merges all parsed sources into `data/chunks.json`;
  this script is the single source of truth for the artifact and should be
  re-run and re-committed whenever extraction logic changes

Each chunk carries: source text, table structure (where applicable), source
document + page reference, entity tags, and a reliability tier (e.g. official
codex vs. in-world ephemera of unknown reliability), so the agent can reason
about conflicting sources rather than just concatenating them.

## Stage 2 — serving (`src/`)

Go, single binary, loads `data/chunks.json` at startup.

**Indexes** (`src/internal/index`, `src/internal/graph`)
- Full-text index — `blevesearch/bleve`, BM25-ranked, exact/fuzzy keyword
  search
- Cross-reference graph — entity relationships seeded from the wiki's ~90
  articles
- In-memory vector store — brute-force cosine similarity, sized for this
  corpus; fallback tool only

**Agent orchestrator** (`src/internal/agent`)
- Planner — decides what to look up next
- Tool router — dispatches to one of the four tools below
- Sufficiency checker — after each result, decides "enough to answer, or
  search again"
- Synthesizer — composes the final cited answer, flags conflicting sources

**Agent tools**

| Tool | Purpose | Used when |
|---|---|---|
| `keyword_search(query)` | Exact/fuzzy full-text search (bleve, BM25) | Default first choice — named entities, specific terms |
| `table_lookup(entity, attribute)` | Structured queries against extracted codex tables | Question needs a specific data point, not prose |
| `follow_reference(entity)` | Traverses the cross-reference graph | Question needs "what else connects to X" |
| `semantic_search(query)` | Vector similarity fallback | Question is conceptual/fuzzy, other tools return nothing useful |

The loop is a real decision, not a fixed number of passes: plan → pick tool
→ read result → decide (answer / refine and search again) → repeat, capped
at a hard iteration limit (see `docs/limitations.md` and the risk table in
the master plan) so the live demo never hangs.

**LLM access** (`src/internal/llm`) — OpenRouter behind an internal
`LLMClient` interface with retry/exponential backoff. Voyage AI
(`voyage-4-lite`/`voyage-4`) powers the semantic-search fallback tool only.

**API** (`src/internal/api`) — HTTP handlers plus trace streaming, so the
reasoning trace is visible live rather than returned only at the end.

## Diagrams

- [`diagrams/system-architecture.md`](diagrams/system-architecture.md) —
  the two-stage pipeline above, as a Mermaid diagram
- [`diagrams/agent-loop.md`](diagrams/agent-loop.md) — the planner → tool →
  read → sufficiency-check → repeat loop, including the iteration cap

## Why this shape

See `docs/decisions.md` for the reasoning behind the tool-first (vs.
vector-RAG-first) architecture and the Python/Go language split.
