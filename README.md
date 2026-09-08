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

The repo currently has structure and docs but no implementation yet — this
is the day-1 setup for anyone starting on it.

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
go mod tidy   # once dependencies (bleve etc.) are added to code
go build ./...
```

`src/go.mod` is already initialized (module
`github.com/HasithaErandika/loregrep/src`, Go 1.25).

**5. What to build first**

Stage 1 extraction (`extraction/parse_pdfs.py`, `parse_docx.py`,
`ocr_scans.py`, `build_artifact.py`) is implemented and has been run
end-to-end over the full corpus — `data/chunks.json` is committed and
current. See `extraction/common.py`'s module docstring for the chunk
schema, and `docs/decisions.md` / `docs/limitations.md` for what was found
running it against the real archive (OCR confidence, entity-tagging
coverage, the `page: null` decision for non-paginated formats, etc.).

To re-run it (only needed if you change extraction logic, or don't yet have
a committed `chunks.json` to work from):

```
cd extraction
python build_artifact.py                # full corpus
python build_artifact.py --sample 3      # ~3 files per format per
                                          # category, for a fast smoke test
python build_artifact.py --skip-ocr      # skip OCR entirely (faster
                                          # iteration; no OCR chunks in
                                          # the output)
```

Next up is Stage 2 — the skeleton (`src/cmd/server/main.go` +
`src/internal/agent`) can be built directly against the real
`data/chunks.json` now — see `docs/diagrams/agent-loop.md` for the loop to
build toward, and `Ashen_Era_Archive/sample_questions.json` for realistic
test questions.

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

*(The `src/` side — `go build ./cmd/server` above — isn't implemented yet;
that's the next stage. Stage 1's `build_artifact.py` command above is real
and produces the committed `data/chunks.json`.)*

## Docs

- [`docs/architecture.md`](docs/architecture.md) — two-stage system, agent
  tools, data flow
- [`docs/decisions.md`](docs/decisions.md) — key engineering decisions and
  why, logged as they happen
- [`docs/limitations.md`](docs/limitations.md) — known weak spots, kept
  honest and current
- [`ai_usage/ai-usage-disclosure.md`](ai_usage/ai-usage-disclosure.md) — AI
  collaboration disclosure
