# loregrep

**SLIIT Codefest 2026 — Sub-track 1C: Searching the Way a Human Does**

An agentic document research assistant that answers questions over the
Ashen Era Archive (415 documents, ~1,277 pages, five formats) by reasoning
through it step by step — planning, choosing a search tool, reading the
result, deciding if it has enough, and repeating until it has a real
answer — rather than one-shot embedding retrieval over top-k chunks.

See the full plan and reasoning in the project brief; key decisions are
logged as they're made in [`docs/decisions.md`](docs/decisions.md).

## Getting started

Stage 1 (extraction) is fully implemented and has been run against the
corpus. Stage 2 (serving) is in progress: all four agent tools
(`keyword_search`, `table_lookup`, `follow_reference`, `semantic_search`)
and the agent orchestrator (planner → tool router → sufficiency check →
synthesizer, per `docs/diagrams/agent-loop.md`) are implemented; only
`cmd/server`/`internal/api` are left — see "Current status" below for
specifics, including which pieces need a live API key to exercise for real
and haven't been yet.

**1. Clone and get the corpus locally**

```
git clone https://github.com/HasithaErandika/loregrep.git
cd loregrep
```

`Ashen_Era_Archive/` is not in the repo (see "Obtaining the corpus" below)
— get it from wherever the team is sharing the competition download and
place it at the repo root, matching the layout in "Repo layout" below.

**2. API keys**

```
cp .env.example .env
```

Fill in `OPENROUTER_API_KEY` (openrouter.ai) and `VOYAGE_API_KEY`
(voyageai.com). Per `docs/decisions.md`'s risk notes, use a separate
OpenRouter account per team member during dev so nobody shares a rate
limit. `.env` is gitignored — never commit real keys.

**3. Stage 1 environment (Python, for `extraction/`)**

Pick whichever matches your setup — both install the same packages from the
same `requirements.txt`, so nobody's env drifts from anyone else's.

<details open>
<summary><strong>venv + pip</strong></summary>

```
cd extraction
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt        # core pipeline deps
pip install -r requirements-dev.txt    # + jupyter, if you'll use notebooks/
```

Tesseract (OCR) is a system package here, not pip-installable:
`sudo apt install tesseract-ocr` (Debian/Ubuntu) or `brew install tesseract`
(macOS). Confirm with `tesseract --version`.

</details>

<details>
<summary><strong>miniforge / mamba</strong></summary>

```
cd extraction
mamba env create -f environment.yml    # or: conda env create -f environment.yml
mamba activate loregrep-extraction
```

`environment.yml` pulls `tesseract` straight from conda-forge, so **no
separate system install is needed** — that's the one real difference from
the venv path above. It also installs `requirements.txt` +
`requirements-dev.txt` via pip inside the conda env, so the actual Python
package set matches the venv path exactly.

If you add or change a pipeline dependency, edit `requirements.txt` (not
`environment.yml` directly) — `environment.yml` just points at it, so both
setup paths stay in sync automatically.

</details>

**One-time, either path:** register the notebook-output-stripping git
filter (see `extraction/notebooks/README.md`):

```
nbstripout --install
```

**4. Stage 2 environment (Go, for `src/`)**

```
cd src
go build ./...
go test ./...
```

`src/go.mod` is already initialized (module
`github.com/HasithaErandika/loregrep/src`, Go 1.25), and its one real
dependency so far (`blevesearch/bleve/v2`, for full-text search) is already
in `go.mod`/`go.sum` — no `go mod tidy` needed to get started.

**5. Current status**

Stage 1 extraction (`extraction/parse_pdfs.py`, `parse_docx.py`,
`ocr_scans.py`, `build_artifact.py`) is implemented and has been run
end-to-end. See `extraction/common.py`'s module docstring for the chunk
schema, and `docs/decisions.md` / `docs/limitations.md` for what was found
running it against the real archive (OCR confidence, entity-tagging
coverage, the `page: null` decision for non-paginated formats, etc.).

**The committed `data/chunks.json` is currently a sample run, not the full
corpus** — its own `meta.sample_mode` is `3` and `meta.document_count` is
33 (of the corpus's real ~341 files, see `docs/limitations.md`). To
regenerate it (needs a local `Ashen_Era_Archive/` — see "Obtaining the
corpus" below):

```
cd extraction
python build_artifact.py                # full corpus
python build_artifact.py --sample 3      # ~3 files per format per
                                          # category, for a fast smoke test
python build_artifact.py --skip-ocr      # skip OCR entirely (faster
                                          # iteration; no OCR chunks in
                                          # the output)
```

Stage 2 (`src/`), in dependency order — loader → indexes → tools → agent →
API:

| Piece | Status | Where |
|---|---|---|
| Corpus loader | done | `src/internal/corpus` |
| `keyword_search` (full-text, bleve/BM25) | done | `src/internal/index/fulltext.go` |
| `table_lookup` | done | `src/internal/index/table.go` |
| Cross-reference graph + `follow_reference` | done | `src/internal/graph` |
| `semantic_search` (vector fallback) | done*, needs `VOYAGE_API_KEY` | `src/internal/index/vector.go`, `src/internal/llm/voyage.go` |
| `LLMClient` (OpenRouter) | done*, needs `OPENROUTER_API_KEY` | `src/internal/llm/openrouter.go` |
| Agent orchestrator (planner/router/sufficiency/synthesizer) | done* | `src/internal/agent` |
| Server + API | not started | `src/cmd/server`, `src/internal/api` |

\* These three depend on live API keys this environment doesn't have
configured. `VoyageClient`/`OpenRouterClient` are unit-tested against a
mocked HTTP server (retry/backoff, error handling, request/response
shapes), and `VectorStore`/the agent loop are smoke-tested against the
real corpus using fake, in-process stand-ins for the two clients — real
scale, real tool dispatch, but not a real model's judgment. Once
`.env`'s `OPENROUTER_API_KEY`/`VOYAGE_API_KEY` are filled in (see "2. API
keys" above) they should work as designed, but that's genuinely
unverified — worth an early smoke test with real keys before relying on it.

There's no runnable server yet — `go test ./...` from `src/` is currently
the way to exercise what's built (the agent package itself has no test
files — see `docs/decisions.md` for why). Note that `follow_reference` is
built from each chunk's already-tagged `entities` field (co-occurrence),
not from the wiki's `[[link]]` markup — that markup isn't preserved in
`chunks.json`, and rebuilding it from raw `Ashen_Era_Archive/` at runtime
would break the extraction/serving seam grading depends on (`go build &&
./server`, no Python) — see `docs/decisions.md`. See
`Ashen_Era_Archive/sample_questions.json` for realistic test questions.

**Notebooks vs. scripts:** prototype parser behavior (e.g. "does
`pdfplumber` get this table right") in `extraction/notebooks/`, then move
working logic into the real `.py` modules above — notebooks are never
imported by the pipeline. See `extraction/notebooks/README.md` for why and
for the `nbstripout` diff-cleanliness setup.

## Repo layout

```
├── Ashen_Era_Archive/    # provided source corpus (gitignored — see below)
├── extraction/            # Stage 1 — Python, build-time document parsing
│   ├── notebooks/           # exploration only, never imported by the pipeline
│   ├── requirements.txt     # core pipeline deps (venv + pip, or via environment.yml)
│   ├── requirements-dev.txt # + jupyter/nbstripout, for notebooks/
│   └── environment.yml      # miniforge/mamba env spec
├── data/
│   └── chunks.json         # committed artifact Stage 2 loads (regenerable)
├── src/                     # Stage 2 — Go, runtime agent + server
│   ├── cmd/server/            # not implemented yet
│   └── internal/
│       ├── corpus/              # chunks.json loader (done)
│       ├── index/                 # keyword_search, table_lookup, vector store (done)
│       ├── graph/                   # follow_reference / cross-reference graph (done)
│       ├── agent/                     # planner, tool router, sufficiency check, synthesizer (done)
│       ├── llm/                         # OpenRouter + Voyage clients (done, needs real API keys to run live)
│       └── api/                           # HTTP handlers, trace streaming (not started)
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

**Regenerate the extraction artifact** (needs a local `Ashen_Era_Archive/`
— see "Obtaining the corpus" above; the committed `data/chunks.json` is
currently a `--sample 3` run, not the full corpus — see "Current status"):

```
cd extraction
python build_artifact.py   # → ../data/chunks.json
```

**Exercise Stage 2** (no Python required — loads the committed
`data/chunks.json` directly):

```
cd src
go build ./...
go test ./...
```

*There is no `cmd/server` yet, so nothing is runnable as a live server or
API — `go test ./...` above is the current way to verify what's built
(corpus loading, `keyword_search`, `table_lookup`). `go build ./cmd/server
&& ./server` will become the real run command once the server and agent
orchestrator exist.*

## Docs

- [`docs/architecture.md`](docs/architecture.md) — two-stage system, agent
  tools, data flow
- [`docs/decisions.md`](docs/decisions.md) — key engineering decisions and
  why, logged as they happen
- [`docs/limitations.md`](docs/limitations.md) — known weak spots, kept
  honest and current
- [`ai_usage/ai-usage-disclosure.md`](ai_usage/ai-usage-disclosure.md) — AI
  collaboration disclosure
