# Decisions

Log of architecture/engineering decisions as they're made, with the reasoning
behind each — this is the evidence trail for "technical judgment" in the
rubric, so entries are added when a decision happens, not reconstructed
after the fact. Newest first.

---

## 2026-09-08 — Agent orchestrator + OpenRouter `LLMClient`: complete the planner → tool router → sufficiency check → synthesizer loop

**Decision:** `src/internal/llm/openrouter.go` adds `LLMClient`/`OpenRouterClient`
(chat completions, OpenRouter's `response_format: json_schema` strict mode
for structured output, retry/backoff — same shape as `VoyageClient`, kept
as a separate implementation rather than a shared abstraction since the
two APIs' request/response/error bodies differ enough that sharing would
need its own escape hatches). `src/internal/agent` adds `Orchestrator.Ask`,
implementing `docs/diagrams/agent-loop.md` exactly: plan a tool call →
dispatch it against the real `Toolset` (`keyword_search`, `table_lookup`,
`follow_reference`, `semantic_search` if configured) → sufficiency check →
loop or synthesize, capped at `MaxIterations` (6).

- **Every LLM decision point uses a JSON schema, not free-text parsing.**
  The planner's tool choice, the sufficiency check, and the synthesizer's
  answer/citations/conflicts are each constrained by an `llm.JSONSchema`
  (OpenRouter strict mode). Strict mode requires every schema property to
  be listed in `required` (nullability expressed via `type` unions, e.g.
  citation `page`), so `ToolCall`'s per-tool-optional fields (`query` vs.
  `entity`/`attribute`) are all required in the schema even though only
  some apply to a given tool — the model just returns `""` for the unused
  ones.
- **The synthesizer's system prompt explicitly names the corpus's
  `reliability_tier` categories** (`docs/limitations.md`:
  `in_world_unreliable`/`official`/`reference`/`narrative`) and instructs
  it to report disagreements rather than silently pick a source — this is
  the actual mechanism behind architecture.md's "flags conflicting
  sources," not a separate conflict-detection algorithm.
- **No test files for `llm/openrouter.go` or `src/internal/agent`** — per
  explicit instruction, since both need a real `OPENROUTER_API_KEY` (not
  configured in this environment) to exercise for real, unlike Voyage/the
  three deterministic tools, which were fully unit-testable against
  synthetic fixtures. Verified instead with in-process smoke runs using a
  fake `llm.LLMClient` returning canned schema-shaped JSON, against the
  real `Toolset` built from the real corpus: (1) a normal run
  (`keyword_search` → sufficient → synthesize) produces a correctly parsed
  `Answer` with citations; (2) an always-insufficient fake confirms the
  loop actually stops at `MaxIterations` (6 tool calls, 13 total LLM calls
  = 6 plan + 6 sufficiency + 1 synthesis) and still returns a best-effort
  answer with `HitIterationCap: true`, rather than hanging — the specific
  demo-safety property `docs/limitations.md` calls out.
- **`Toolset.Semantic` may be `nil`** (no embeddings built yet — needs a
  live `VOYAGE_API_KEY`), and the planner is only ever told about tools
  that are actually available (`availableTools`), so a missing
  `semantic_search` degrades gracefully instead of the LLM picking a tool
  that would error.

---

## 2026-09-08 — `semantic_search` tool: Voyage embeddings, brute-force cosine, verified without a live key

**Decision:** `src/internal/llm/voyage.go` adds `EmbeddingClient`/`VoyageClient`
(Voyage AI's embeddings API, retry/backoff, batching above the API's
1,000-input-per-request cap). `src/internal/index/vector.go` adds
`VectorStore` (brute-force cosine similarity — the corpus is small enough
that an ANN index would add complexity without a measurable speed win) and
`SemanticIndex`, the `semantic_search` agent tool.

- **`input_type` is set explicitly** (`"document"` when embedding chunks
  via `BuildVectorStore`, `"query"` when embedding a search query via
  `SemanticIndex.Search`) — Voyage's asymmetric embedding support, which
  produces different (better-matched) vectors for the same model depending
  on which side of a retrieval pair a text is on.
- **No `VOYAGE_API_KEY` is configured in this environment**, so
  `VoyageClient` is verified only via unit tests against an
  `httptest.Server` mock (success, out-of-order response indices, 429
  retry-then-succeed, non-retryable 400, retries-exhausted 500, batch
  splitting) — real, but not proof the live API integration works
  end-to-end. Separately, `VectorStore`/`BuildVectorStore` were smoke-
  tested against the real 3,008-chunk corpus using a deterministic
  hash-based fake embedder (semantically meaningless, but real scale):
  build took ~5ms, a brute-force search over all 3,008 vectors took
  ~0.5ms — confirms brute force is fast enough here, without needing a
  live key to prove it.

---

## 2026-09-08 — Cross-reference graph + `follow_reference(entity)`: co-occurrence, not wiki links

**Decision:** `src/internal/graph` builds the cross-reference graph purely
from `data/chunks.json`'s per-chunk `entities` field — two entities get an
edge whenever Stage 1 tagged both into the same chunk, weighted by how many
chunks co-tag them, with up to 10 example chunks kept per edge for
citation. `FollowReference(entity, limit)` returns an entity's neighbors,
ranked by weight, with the same exact-then-substring entity matching as
`TableIndex.Lookup` — merging edges across every substring match rather
than picking one, for the same reason.

- **Not built from the wiki's `[[link]]` markup**, even though that's the
  more obvious "cross-reference" signal and `docs/architecture.md` calls
  the graph "seeded from the wiki's ~90 articles." `extraction/common.py`'s
  `clean_wiki_markup` strips `[[Entity Name]]` down to plain prose before a
  chunk is ever written to `chunks.json` — link structure isn't preserved
  in the artifact at all. Rebuilding it would mean Stage 2 reading raw
  `Ashen_Era_Archive/` markdown at runtime, which breaks the entire
  extraction/serving seam (`docs/decisions.md`'s Python/Go split decision):
  the raw corpus isn't committed and grading only runs `go build &&
  ./server`. Co-occurrence within a chunk's already-computed `entities`
  list needs nothing beyond the committed artifact and is a reasonable
  proxy — two entities named in the same paragraph/table row are almost
  always actually related in this corpus.
- **Verified against the real corpus, not just synthetic fixtures:** 59
  entities, 246 co-occurring pairs. `FollowReference("The Bleeding Crown")`
  top hit is "House Morvain" at weight 133 — matches an independent
  co-occurrence count run directly against `chunks.json` outside the Go
  code, before trusting the implementation.
- **Graph quality inherits Stage 1's entity-tagging limitations exactly**
  (`docs/limitations.md`): exact-name string matching, no coreference
  resolution, so an entity referred to only as "the Warden" or "she" in a
  chunk that also names another entity won't produce an edge there. Not a
  Stage 2 defect — there's no additional information Stage 2 could recover
  that Stage 1 didn't already extract.

---

## 2026-09-08 — `table_lookup(entity, attribute)` tool: subject resolution over codex tables

**Decision:** `src/internal/index/table.go` adds `TableIndex`, indexing
every table chunk by the entity/subject its rows describe, for the
`table_lookup` agent tool from `docs/architecture.md`.

- **Subject resolution is a three-step fallback**, discovered by inspecting
  actual table chunks rather than assumed up front: (1) an explicit `Name`
  row if the table has one — most character/relic/creature tables do; (2)
  the chunk's `section` with a `Registry:` prefix stripped — codex
  biography entries are sectioned `Registry: <name>`; (3) the document
  title — needed for wiki infobox tables, whose `section` is either `nil`
  or the **literal string `"Infobox"`**, neither of which names the entity.
  Missing case (2) initially: the first pass fell back to `doc_title` only
  when `section == nil`, so every wiki infobox table (`section: "Infobox"`)
  resolved to the wrong subject — caught by testing against real corpus
  shapes (`wiki/aldous_wrenfield_the_last_warden.md`,
  `wiki/ashfall_colossus.md`) before this was ever exercised at runtime,
  not by a downstream failure.
- **A useful side effect of the `Registry:`-stripping and `Name`-row rules:**
  the same entity's codex biography table and wiki infobox table resolve to
  the identical subject key (both just the person/place's name), so a
  single `table_lookup` call merges rows from both sources rather than
  requiring the caller to know which document to ask. Verified against the
  real corpus: `Lookup("Aldous Wrenfield the Last Warden", "")` returns rows
  from both `codex/the_annals_of_the_ashen_era.*` and
  `wiki/aldous_wrenfield_the_last_warden.md`.
- **Entity and attribute matching are both exact-first, substring-fallback**
  (only falling back when there's no exact match), so a precise query on a
  common short name or label isn't diluted by unrelated partial matches —
  e.g. `Lookup("Aldous Wrenfield", "wields")` still resolves correctly via
  the fallback even though no table's subject is literally "Aldous
  Wrenfield".
- **Duplicate/corroborating rows across chunks are all returned, not
  deduped** — e.g. `codex_vaeloria_ii...docx` and the `.pdf` re-export of
  the same document both produce an "Attunement cost" row for the same
  relic. Deduping is left to the caller (the synthesizer), since two
  sources agreeing is itself a useful signal the agent may want to
  preserve, not noise to hide.

---

## 2026-09-08 — Stage 2 started: corpus loader + full-text index (`keyword_search`)

**Decision:** `src/internal/corpus` loads `data/chunks.json` into typed
`Chunk`/`Meta` structs (nullable fields stay pointers rather than being
defaulted, matching Stage 1's `page: null` decision below). `src/internal/index`
wraps an in-memory `bleve` index (`bleve.NewMemOnly` — no on-disk index
file, since the source artifact is already a committed build output) over
every chunk's searchable text, and exposes `KeywordSearch`, the `keyword_search`
agent tool from `docs/architecture.md`.

- **Table chunks are indexed too, via a flattened text rendering**
  (`Chunk.SearchText()` joins each row with spaces). `chunks.json` stores
  `text: null` for `chunk_type: "table"` chunks — table content lives only
  in the `table` field — so without this, `keyword_search("garrison
  strength")` would silently miss every codex table row and only the agent's
  `table_lookup` tool would ever see them. Confirmed against the real
  corpus: `keyword_search("garrison strength")` now returns a `plate_*`
  table chunk alongside prose hits.
- **Scoring model is explicitly set to BM25** (`IndexMapping.ScoringModel =
  "bm25"`) rather than left at bleve's tf-idf default, since
  `docs/architecture.md` names BM25 specifically for this tool.
- **Chunks with no searchable text are skipped at index time, not indexed
  as empty documents** — e.g. `wiki/images/atmo_*` chunks that never made it
  past Stage 1's OCR confidence floor (`docs/limitations.md`). Verified this
  doesn't silently drop real content: `NewFullText` skips exactly the chunks
  where `SearchText()` is empty, and a test asserts the skipped count.
- Verified against the actual artifact, not just synthetic fixtures: loads
  all 3,008 real chunks in ~40ms, indexes in ~2.7s, and returns sane
  BM25-ranked hits for both prose and table queries — see the package tests
  for the synthetic-fixture coverage that runs in CI.

**Not yet decided:** where the loader resolves `chunks.json`'s path from at
server startup (flag vs. hardcoded relative path) — deferred until
`cmd/server` exists.

---

## 2026-09-08 — Stage 1 extraction implemented: chunk schema, `chunks.json` shape, and a handful of corpus-driven calls

**Decision:** `extraction/common.py`, `parse_pdfs.py`, `parse_docx.py`,
`ocr_scans.py`, and `build_artifact.py` are implemented end-to-end and have
been run over the full corpus (see `docs/limitations.md` for the resulting
counts). A few concrete choices, made against what the actual corpus turned
out to contain rather than decided up front:

- **`chunks.json` top-level shape is `{"meta": {...}, "chunks": [...]}`**,
  not a bare array — `meta` carries `generated_at`, per-category/format/
  extraction-method counts, entity count, and whether OCR ran, so Stage 2
  (or anyone inspecting the file) gets corpus stats without scanning every
  chunk. Nothing in `docs/architecture.md` dictated this either way since
  Stage 2 doesn't exist yet — see `common.py`'s and `build_artifact.py`'s
  module docstrings for the full chunk schema.
- **`page` is `null` for docx/md/txt/image chunks, never a guessed value.**
  Those formats have no native page boundaries before rendering — python-docx
  paragraphs, Markdown, and plain text don't carry page breaks. Estimating a
  page number from a characters-per-page heuristic would look precise while
  being fabricated. `section` (nearest heading) is the citation anchor for
  those chunks instead; `page` stays populated and real for `pdf`/`scan_pdf`.
- **`images/` (top-level) is skipped; only `codex/images/` is processed.**
  `diff` plus a byte comparison confirmed `images/` is an exact duplicate of
  `codex/images/` (same 15 filenames, identical bytes) — processing both
  would double-count every plate's OCR chunk under two different `doc_id`s
  for no benefit.
- **`wiki/images/` (character portraits, battle paintings — filenames
  prefixed `atmo_`) get OCR attempted but mostly produce nothing, correctly.**
  These are illustrative art with no embedded text, unlike `codex/images/`'s
  `plate_*` files, which are bar-chart-style infographics with real numeric
  labels (garrison strength, attunement cost, casualties). Tesseract on pure
  artwork doesn't cleanly return empty — it hallucinates short strings of
  near-random glyphs at ~30 mean word confidence, versus 90+ on the corpus's
  real document/plate text in spot checks. `ocr_scans.py` discards anything
  below a 35-confidence noise floor rather than emitting it as a chunk. This
  also means the sub-track 1A question style "what object is X holding in
  their portrait" is **not answerable from Stage 1's output** — that needs
  vision-LLM captioning, not OCR. See `docs/limitations.md`.
- **Every figure plate is OCR'd with two Tesseract page-segmentation modes
  (`--psm 6` and `--psm 12`), keeping whichever gets higher mean word
  confidence**, rather than one fixed mode. `--psm 6` (single prose block)
  is right for scanned ephemera pages; the bar-chart plates' bold numeric
  labels are laid out too sparsely for it and `--psm 6` silently dropped
  most of the numbers in a spot check (e.g. only "20" of four labels on one
  plate) while `--psm 12` (sparse text + orientation detection) caught all
  four at 91 mean confidence. Trying both and picking by confidence avoids
  hard-coding a mode per source folder while still getting the numbers that
  matter for sub-track 1A's "what's the attunement cost" style questions.

**Why not decided differently:** a fixed characters-per-page estimate for
docx page numbers was considered and rejected — a wrong-but-confident page
citation is worse for the agent's answer quality than an honest `null` with
a section name, since a judge can spot-check a page reference that's often
off by one document but consistently structured never gets caught as wrong.

---

## 2026-09-04 — Notebooks are exploration-only; the pipeline is plain `.py`, output-stripped via nbstripout

**Decision:** `extraction/notebooks/` holds `.ipynb` files for prototyping
parsers against sample documents only. `build_artifact.py` and the other
Stage 1 scripts never import from that folder — working notebook logic gets
rewritten as a function in the real `.py` module. Notebook outputs are
stripped on commit via `nbstripout` (`.gitattributes` + one-time
`nbstripout --install` per clone), so `.ipynb` diffs stay text-only.

Both `venv`/`pip` and `miniforge`/`mamba` are supported for the Python side
— `extraction/requirements.txt` is the single source of truth for pipeline
deps, `requirements-dev.txt` adds notebook tooling, and
`extraction/environment.yml` (conda) just installs both via pip inside the
conda env so the two paths can't drift apart. The one substantive
difference: `environment.yml` pulls `tesseract` from conda-forge, so
miniforge users skip the system package install that venv/pip users still
need.

**Why:**
- Notebooks are good for the actual exploration work (does this parser get
  this table right, what does this OCR output look like) but bad as the
  pipeline itself: cells can execute out of order and silently diverge from
  the file's visual top-to-bottom order, and `build_artifact.py` needs to
  run non-interactively — `python build_artifact.py`, not "run all cells."
- Un-stripped `.ipynb` diffs (execution counts, embedded output/image
  blobs) are exactly the kind of noisy, unreviewable commit the git-
  discipline grading criteria (30% of the rubric) penalizes. Stripping
  outputs before commit keeps notebook diffs as readable as any other file.
- The team isn't standardized on one Python environment manager — requiring
  everyone to install extra system packages differently (or not supporting
  conda at all) would slow down day-1 setup. A single `requirements.txt`
  referenced from both paths avoids two dependency lists silently drifting.

---

## 2026-09-04 — Tool-first agent, not vector-RAG-first

**Decision:** The system is built around an agent that iteratively chooses
from a small set of search tools (exact/fuzzy keyword search, structured
table lookup, cross-reference graph traversal, semantic search) and decides
after each result whether it has enough to answer. Vector/semantic search is
one tool among four, used only as a fallback for genuinely conceptual or
fuzzy questions — it is not the primary retrieval mechanism.

**Why:**
- Anthropic removed vector search from Claude Code in favor of direct,
  agent-driven search tools and found it outperformed embedding-based
  retrieval — an LLM driving search iteratively can refine its query, follow
  references, and self-correct in a way one-shot embedding retrieval can't.
- A 2026 AAAI benchmark found agentic keyword search beat traditional vector
  RAG by 6 points on answer correctness on a table-heavy, structured-document
  benchmark — the same document profile as the Ashen Era Archive (tables,
  cross-references, conflicting in-world sources).
- The strongest current systems are hybrid: semantic search combined with
  exact/structured search beats either alone. So semantic search is kept,
  just demoted to a fallback tool rather than the backbone.

**Alternative considered:** standard RAG (embed everything, retrieve top-k,
hand to LLM). Rejected because it treats every chunk as equally trustworthy
(can't flag conflicting sources) and can only retrieve what "looks similar"
to the question, not what a multi-step chain of reasoning would actually
need to follow — both of which are named failure modes in the sub-track 1C
problem statement itself.

---

## 2026-09-04 — Python for offline extraction, Go for online serving

**Decision:** Document extraction (`extraction/`) is Python, run at build
time only, output committed as `data/chunks.json`. The live, graded system
(`src/`) is a single Go binary that loads that artifact and does not depend
on Python at runtime.

**Why:**
- Python's document-parsing ecosystem (`pdfplumber` for tables, `PyMuPDF`
  for speed/images, `python-docx`) is more mature than Go's equivalent —
  Go's most complete option (UniDoc) is commercially licensed, and the
  open-source alternatives are noticeably thinner, especially for table
  structure extraction.
- Neither language solves OCR for scanned pages natively — both need an
  external Tesseract install or a vision-LLM call. Not a language
  differentiator, so it isn't treated as one.
- Go's advantages — goroutine concurrency, static typing, single-binary
  deployment, no dependency-resolution risk at grading time — apply
  specifically to the serving/reasoning layer, which is live, repeated, and
  graded every time a judge interacts with it. That's where reliability
  matters most.
- Splitting at the extraction/serving seam (rather than forcing one language
  across both) gets Python's extraction quality and Go's single-binary
  reproducibility simultaneously, with neither compromised. Grading only
  requires `go build && ./server`.

**Alternative considered:** all-Go (extraction + serving in one language, one
toolchain). Rejected — the extraction-quality gap (esp. table structure)
was judged a bigger risk to answer correctness than the operational
convenience of a single language.

**Alternative considered:** all-Python. Rejected for the serving layer
specifically because of dependency-resolution risk at grading time and
weaker concurrency ergonomics for a live, repeatedly-queried agent loop.

---

## 2026-09-04 — Raw corpus (`Ashen_Era_Archive/`) is gitignored, not committed

*Supersedes the same-day decision below to commit it — reversed before any
commit had actually happened, on reconsideration of git best practice for
large, non-authored input data.*

**Decision:** `Ashen_Era_Archive/` (the provided source corpus) is
gitignored. `data/chunks.json`, the artifact Stage 1 produces from it,
remains committed and is the only thing the live Go server (and grading)
actually needs.

**Why:**
- It's ~122MB of binary/media files (PDFs, DOCX, PNGs) that no one on the
  team authored — it's provided input, not project output. Standard
  practice is to keep large provided/generated input data out of version
  control and instead commit the derived artifact plus the pipeline that
  produces it, so every clone/fetch/CI checkout doesn't keep paying to
  transfer it.
- Nothing about reproducibility is lost where it matters: `chunks.json` is
  still committed, versioned, and regenerable, and `extraction/` +
  `docs/architecture.md` document exactly how it was produced. What's lost
  is one specific thing — regenerating `chunks.json` from scratch requires
  a local copy of the archive — which is an acceptable tradeoff since the
  competition distributes that archive separately from this repo anyway.
- See `README.md` → "Obtaining the corpus" for where team members get a
  local copy.

<details>
<summary>Original (superseded) reasoning for committing it</summary>

It's ~122MB total with no individual file over a few MB — well within
normal git limits, no LFS needed. Committing it would keep the extraction
pipeline (`extraction/build_artifact.py`) reproducible from a fresh clone
alone. Reversed above: reproducibility of the *derived artifact* (which is
what grading and the live system depend on) doesn't require committing the
*raw* input too, and 122MB of binary assets that nobody on the team
authored is exactly the kind of thing git best practice says to keep out
of history.
</details>
