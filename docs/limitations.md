# Limitations

Honest, living record of what doesn't work or is known to be weak. Update
this as the build progresses — it's graded, and a vague or empty version at
submission time is worse than a short, specific one.

## Stage 1 (extraction) — confirmed during the build

- **The corpus is 341 files, not 415** — `find Ashen_Era_Archive -type f`
  counts 8 chronicles, 6 codex, 145 ephemera, 95 wiki articles (254
  text-bearing documents), 85 image plates, `sample_questions.json`, and
  `README.txt`. The "415 documents, ~1,277 pages" figure in `README.md`
  and `docs/architecture.md` is from the original competition brief, not
  re-verified against the actual delivered archive — worth reconciling
  with the team, but Stage 1 processes whatever is actually present rather
  than assuming the brief's numbers.
- **`top-level images/` is a byte-identical duplicate of `codex/images/`**
  (same 15 filenames, `cmp` confirms identical bytes) — only `codex/images/`
  is processed; see `docs/decisions.md`.
- **OCR on scanned ephemera pages (`*.scan.pdf`) is good** in spot checks —
  Tesseract at 300dpi with `--psm 6` scored 90-95+ mean word confidence on
  every scanned page checked, including full stanzas of verse with unusual
  line breaks. This resolves the "unverified as of this writing" risk noted
  during planning.
- **OCR on the corpus's figure plates (`codex/images/plate_*.png`) needed a
  second Tesseract mode to be reliable.** These are bar-chart-style
  infographics (a title, several labeled bars, bold numeric values) — the
  default single-block prose assumption (`--psm 6`) missed most of the
  numbers in a spot check (only 1 of 4 values recovered on one plate);
  `--psm 12` (sparse text) recovered all 4 at 91 mean confidence.
  `ocr_scans.py` now tries both per page/image and keeps whichever scores
  higher, rather than assuming one mode corpus-wide.
- **`wiki/images/` portraits and battle paintings (filenames prefixed
  `atmo_`) carry no embedded text at all** — they're illustrative art, not
  documents. Tesseract doesn't cleanly return empty on them; it hallucinates
  short strings of near-random glyphs at ~30 mean confidence. `ocr_scans.py`
  discards anything below a 35-confidence noise floor so this doesn't
  pollute `chunks.json` with garbage chunks — but it also means **sub-track
  1A questions like "what object is X holding in their portrait" are not
  answerable from Stage 1's output at all**. Answering those needs
  vision-LLM image captioning (OpenRouter, per `docs/architecture.md`'s
  "vision-capable OpenRouter model" note), which is a materially different
  capability from OCR and is not implemented — flagged here as an actual
  gap, not a planned-but-pending item.
- **docx/Markdown/plain-text chunks carry no page number (`page: null`)** —
  by design, not a defect: these formats have no page boundaries before
  rendering. `section` (nearest heading) is the citation anchor instead.
  Only `pdf`/`scan_pdf`/`image` chunks (rendered formats) carry a real page
  number. See `docs/decisions.md` for why a guessed page number was
  rejected outright rather than left as a TODO.
- **Cross-reference graph coverage** is seeded only from the 95 wiki
  articles' entity list (not "~90" — confirmed count), not from every entity
  mentioned in raw prose across chronicles/codex/ephemera. Questions that
  hinge on an entity that only appears in chronicles/codex prose (never in
  the wiki) may not be reachable via `follow_reference`, and the same list
  is what Stage 1 tags every chunk's `entities` field against — an entity
  never named in the wiki is invisible to entity tagging too, not just graph
  traversal.
- **Entity tagging is exact-name string matching** (case-insensitive, whole-
  word/phrase, longest-name-first) against the wiki-derived entity list, not
  coreference resolution — a chunk that refers to "the Warden" or "she"
  without repeating the entity's full name won't be tagged for that entity.

## Known risk areas (Stage 2, not yet built)

- **Agent loop iteration cap** — the planner/tool-router loop is capped at a
  fixed number of iterations (see `docs/decisions.md` / architecture) to
  guarantee termination during a live demo. A question that genuinely needs
  more hops than the cap will get a best-effort partial answer rather than
  a complete one.
- **In-world source reliability** — the corpus deliberately includes
  unreliable narrators (README.txt: "in-world authors are not always
  reliable"). The `reliability_tier` field in `chunks.json` is assigned
  purely by source folder (`ephemera` → `in_world_unreliable`, `codex` →
  `official`, `wiki` → `reference`, `chronicles` → `narrative`) — it's a
  coarse, category-level heuristic, not a per-claim fact-check, so two
  ephemera documents that actually disagree on a number are both tagged the
  same tier; Stage 2's conflicting-source detection is only as good as that
  coarse signal.
- **Vector fallback quality** — the semantic-search tool exists for
  conceptual/fuzzy questions but is a brute-force in-memory cosine store
  sized for this corpus only; it is not the primary retrieval path and has
  not been tuned as heavily as keyword/table/graph search.

## To fill in during the build

- [ ] Failure modes actually observed against `sample_questions.json`
- [ ] Any question categories the agent consistently gets wrong, and why
- [ ] Performance/latency notes if relevant under demo conditions
