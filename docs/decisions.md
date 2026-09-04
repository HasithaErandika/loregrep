# Decisions

Log of architecture/engineering decisions as they're made, with the reasoning
behind each — this is the evidence trail for "technical judgment" in the
rubric, so entries are added when a decision happens, not reconstructed
after the fact. Newest first.

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
