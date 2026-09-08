# AI Usage Disclosure

Sub-track 1C submission — SLIIT Codefest 2026.

This document discloses how AI tools were used during development, per the
competition's human-AI collaboration grading criteria (30% of the rubric).
Update it as work happens; keep the corrections and rejected suggestions
alongside the accepted ones, since that's what's actually being scored.

## Operating principle

**Every architecture and technical decision in this project is made by the
team**, not by an AI tool. Claude Code is used to *maximize idea and coding
output* — drafting options quickly, writing boilerplate and docs, exploring
tradeoffs, producing first-pass implementations — but the team reviews,
corrects, and decides. `docs/decisions.md` records team decisions; where
Claude Code's proposal was accepted as-is, changed, or rejected outright is
what the chat logs in `ai_usage/chat-logs/` exist to show.

## Tools used

- **Claude Code** (Claude Sonnet 5) — repo scaffolding, drafting
  architecture/decision docs, implementation assistance throughout Stage 1
  and Stage 2
- (add others as used, e.g. ChatGPT, GitHub Copilot)

## Commit attribution

Commits authored with Claude Code in this repo carry:

```
Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: <session URL>
```

This is an automatic, per-commit audit trail — it links a commit straight
back to the session that produced it, which is stronger evidence than a
manually-kept log. A commit with **no** such trailer means it was written
by a team member without AI assistance; don't add the trailer to those.

## How AI was used, by area

Filled in as the build progresses — see `ai_usage/chat-logs/` for the
detail behind each row (what was proposed, what was corrected, why).

| Area | AI involvement | Human review/correction |
|---|---|---|
| Repo scaffolding (`docs/`, `.gitignore`, folder structure) | Claude Code drafted the folder structure, `docs/architecture.md`, `docs/decisions.md`, diagrams, and `.gitignore` content from the team's plan — as working-tree files only | Team corrected the initial `.gitignore`/decision to commit the raw `Ashen_Era_Archive/` corpus — redirected to gitignore it as large, non-authored input data (see `docs/decisions.md`, entry superseding the original). The actual `git init`, review, commit, and push to GitHub were done by the team directly, not by Claude Code — commit `af31ba5` carries no `Co-Authored-By` trailer, correctly. |
| Stage 1 extraction (`extraction/`) | Claude Code implemented `common.py`/`parse_pdfs.py`/`parse_docx.py`/`ocr_scans.py`/`build_artifact.py` end-to-end, inspected the actual corpus (not just the docs) to make several concrete calls: the `chunks.json` `{"meta", "chunks"}` shape, `page: null` for non-paginated formats, deduping `images/` against `codex/images/`, a dual-Tesseract-PSM-mode OCR heuristic, and a noise-confidence floor to drop hallucinated OCR on illustrative art — logged in `docs/decisions.md` and `docs/limitations.md`. Installed Tesseract (`winget install UB-Mannheim.TesseractOCR`) after asking, and ran `build_artifact.py` against the full corpus. | See `ai_usage/chat-logs/2026-09-08_stage1-extraction_KD.md` for the session detail. |
| Stage 2 indexing (`src/internal/corpus`, `index`, `graph`) | Claude Code implemented the corpus loader, `keyword_search` (bleve/BM25), `table_lookup`, the cross-reference graph + `follow_reference` (entity co-occurrence, not wiki link markup — see `docs/decisions.md`), and the `semantic_search` vector store (Voyage embeddings + brute-force cosine), each verified against the real corpus as it landed. Also corrected `README.md`'s stale "no implementation yet" setup instructions. | Caught its own subject-resolution bug in `table_lookup` (wiki tables' `section: "Infobox"` was wrongly treated as identifying) by testing against real corpus shapes before shipping it, not just hand-written fixtures — logged in the chat log below rather than silently fixed. Commits were reviewed and made by the human between turns, not delegated to Claude Code. See `ai_usage/chat-logs/2026-09-08_stage2-build_HE.md`. |
| Agent orchestrator + LLM clients (`src/internal/agent`, `src/internal/llm`) | Claude Code implemented the OpenRouter `LLMClient` and the full planner → tool router → sufficiency check → synthesizer loop (`docs/diagrams/agent-loop.md`), with every LLM decision point constrained to a JSON schema rather than parsed from free text. API request/response shapes were verified against each provider's live docs before writing the clients. | Explicitly directed to skip unit tests for this piece (no live `OPENROUTER_API_KEY`/`VOYAGE_API_KEY` configured to exercise it against) — a deliberate scope redirection from the test-per-package pattern used everywhere else in Stage 2. Verified instead with in-process smoke runs (fake LLM client, real corpus, real tools) confirming control flow, not real model judgment. See chat log. |
| API/UI (`src/internal/api`) | | |

## Chat logs

Session logs live in `ai_usage/chat-logs/` — see that folder's
[`README.md`](chat-logs/README.md) for the naming convention and what to
capture:

- `2026-09-04_repo-scaffolding_HE.md` — initial repo/docs scaffolding
- `2026-09-08_stage1-extraction_KD.md` — Stage 1 extraction pipeline
- `2026-09-08_stage2-build_HE.md` — Stage 2 loader through agent
  orchestrator, plus a `README.md` correction

## Team contribution notes

- (per-member note on what they used AI for vs. wrote themselves, to support
  the report's team-contributions section)
