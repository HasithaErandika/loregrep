# 2026-09-04 — Repo scaffolding (Claude Code)

**Participant:** HE (wickramasinghe.erandika@gmail.com)
**Tool:** Claude Code (Claude Sonnet 5)
**Topic:** Turning the master plan into an actual repo — folder structure,
`docs/`, `.gitignore`, diagrams, AI usage disclosure.

## Summary (human-written, condensed from the session)

Human pasted the full sub-track 1C master plan (problem statement,
architecture, tech stack, repo structure, timeline) and asked Claude Code to
set up the repo to match: correct `.gitignore`, the folder structure from
the plan's section 6, and a `docs/` folder.

**What Claude Code did:**
- Created the full directory tree (`extraction/`, `data/`, `src/cmd/server`,
  `src/internal/{index,graph,agent,llm,api}`, `docs/`, `ai_usage/`)
- Wrote `docs/architecture.md` and `docs/decisions.md` from the plan's
  reasoning (tool-first vs. vector-RAG-first, Python/Go split)
- Wrote a starter `docs/limitations.md` and `ai_usage/ai-usage-disclosure.md`
- Wrote `.gitignore`

**Correction #1 (caught before anything was committed):** Claude Code's
first `.gitignore` pass *committed* the raw `Ashen_Era_Archive/` corpus
(~122MB) to git, reasoning that no single file exceeded a few MB so it was
"safe to track directly," and logged that as a decision in
`docs/decisions.md`.

Human pushed back: *"since the data is large the best practice is to make
that gitignore right"* — i.e. large, non-authored provided input data
shouldn't go into version control at all, regardless of whether it happens
to be under GitHub's hard size limits. Claude Code reversed the decision:
gitignored `Ashen_Era_Archive/`, kept only the derived `data/chunks.json`
artifact tracked, added a "superseded" entry to `docs/decisions.md`
explaining the reversal instead of silently rewriting history, and added an
"Obtaining the corpus" section to `README.md` so team members know how to
get a local copy for re-running Stage 1.

**Correction #2:** Human clarified that architecture and technical
decisions are made **by the team**, and Claude Code's role is to *maximize
idea and coding output* (drafting, exploring options, writing code/docs
quickly) — not to be the decision-maker of record. `ai_usage/ai-usage-
disclosure.md` was rewritten with this framing as an explicit "Operating
principle" section, and the per-commit `Co-Authored-By` / `Claude-Session`
trailer was documented as the mechanism that distinguishes AI-assisted
commits from team-only ones.

**Requested next:** real Mermaid diagrams in `docs/diagrams/`
(`system-architecture.md`, `agent-loop.md`) and a working `ai_usage/`
folder (this log + its `README.md`) instead of empty placeholders.

## Outcome

All of the above applied to the working tree. Claude Code did not run any
`git init`/`commit`/`push` commands that ended up in the project's history —
**the human performed the actual git init, review, commit, and publish to
`github.com/HasithaErandika/loregrep` directly**, outside of AI tool calls.
That commit (`af31ba5`, "docs: init project basics") carries no
`Co-Authored-By` trailer, correctly reflecting that it's human-authored, not
AI-authored — see the "Commit attribution" convention in
`../ai-usage-disclosure.md`.
