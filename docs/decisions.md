# Decisions

Log of architecture/engineering decisions as they're made, with the reasoning
behind each — this is the evidence trail for "technical judgment" in the
rubric, so entries are added when a decision happens, not reconstructed
after the fact. Newest first.

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
