# 2026-09-08 — Stage 1 extraction implementation (Claude Code)

**Participant:** KD (krishmaldinidu5466@gmail.com)
**Tool:** Claude Code (Claude Sonnet 5)
**Topic:** Implementing Stage 1 extraction (`extraction/`) end-to-end against
the real `Ashen_Era_Archive/` corpus and producing `data/chunks.json`.

## Summary (human-written, condensed from the session)

Human asked Claude Code to refer to `Ashen_Era_Archive/` and `docs/` and
complete Stage 1.

**What Claude Code did:**
- Read `docs/architecture.md`, `docs/decisions.md`, `docs/limitations.md`,
  `README.md`, and `Ashen_Era_Archive/README.txt` for the plan, then
  actually inspected the corpus (opened sample PDFs/DOCX/scans/images with
  `pdfplumber`/`PyMuPDF`/`python-docx`/Tesseract) rather than implementing
  from the docs' description alone.
- Discovered mid-exploration that `codex/images/*.png` "figure plates" are
  data-bearing bar-chart infographics (garrison strength, attunement cost,
  casualties) that sub-track 1A questions directly reference — not just
  scanned document pages — and that `wiki/images/*.png` portraits/paintings
  are pure illustrative art with no embedded text at all. Neither is called
  out explicitly in `docs/architecture.md`, which only names `.scan.pdf` as
  needing OCR.
- Implemented `extraction/common.py` (chunk schema, entity tagging via a
  wiki-derived entity list, text chunking), `parse_pdfs.py` (pdfplumber),
  `parse_docx.py` (python-docx, walking document order to keep tables in
  their section), `ocr_scans.py` (Tesseract via pytesseract), and
  `build_artifact.py` (orchestrator + Markdown/plain-text parsing inline,
  per `docs/architecture.md`'s file list which names a dedicated parser only
  for PDF/DOCX/OCR).

**Correction/decision points (asked, not assumed):**
- **Tesseract wasn't installed on the dev machine.** Rather than silently
  skipping OCR or silently installing software, Claude Code asked the human
  first via a direct question (install now via `winget`, vs. write the code
  to degrade gracefully and let OCR be verified later). Human chose to
  install now (`winget install UB-Mannheim.TesseractOCR`) so OCR could
  actually be spot-checked and run against the real corpus in this session.
- **OCR mode found wrong by spot-checking, not assumed correct.** The first
  pass used Tesseract's default single-block mode (`--psm 6`) for
  everything; testing it against an actual figure plate showed it dropped
  3 of 4 numeric labels (kept "20", lost "55"/"85"/"94"). Rather than
  shipping that, tried multiple PSM modes, found `--psm 12` recovered all
  four at higher confidence, and changed `ocr_scans.py` to try both and keep
  whichever scores higher — a self-caught issue before it reached the human,
  logged here per this folder's rule to record what was corrected, not only
  what was accepted the first time.
- **First full-corpus-shaped test run surfaced two more self-caught issues,
  fixed before considering Stage 1 done:** (1) OCR on purely illustrative
  `wiki/images/` portraits/paintings was hallucinating garbled short strings
  instead of returning nothing (~30 mean confidence vs. 90+ on real text) —
  added a confidence floor to discard those as noise rather than emit
  garbage chunks. (2) PDF document titles for multi-line title pages
  (chronicles/codex) were truncating at the first line break, e.g. "The
  Ashen Chronicles, Volume I: The" instead of "...The Kindling Years", while
  the naive full-first-paragraph fix broke ephemera PDFs (which have no
  title page and start straight into body text) by swallowing the entire
  page as the "title" — settled on: join a short first paragraph, else fall
  back to just the first line, else fall back to the filename.
- Encoding: printed OCR/plain-text output initially showed `�` for curly
  apostrophes/em-dashes; verified by inspecting the actual bytes in
  `data/chunks.json` (not the terminal echo) that this was the terminal's
  own display limitation, not real data corruption — the file correctly
  holds proper Unicode (U+2019 etc.) throughout. Logged as a check performed
  rather than a bug fixed, since nothing needed changing.

**Decisions logged to `docs/decisions.md`** (all made and recorded during
this session, not after the fact): the `{"meta", "chunks"}` artifact shape,
`page: null` for non-paginated formats, deduping `images/` against
`codex/images/`, and the dual-PSM OCR approach.

## Outcome

`extraction/common.py`, `parse_pdfs.py`, `parse_docx.py`, `ocr_scans.py`,
`build_artifact.py` are implemented and were run against the full corpus,
producing `data/chunks.json`. `docs/decisions.md` and `docs/limitations.md`
were updated with what running it against the real archive actually showed
(OCR confidence figures, the portrait-captioning gap for sub-track 1A,
corrected corpus file counts). `README.md`'s "What to build first"/"Running"
sections were updated to reflect that Stage 1 is implemented rather than
pending. Nothing was committed to git by Claude Code in this session — left
for the team to review and commit.
