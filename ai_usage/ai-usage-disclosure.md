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
| Stage 1 extraction (`extraction/`) | | |
| Stage 2 indexing (`src/internal/index`, `graph`) | | |
| Agent orchestrator (`src/internal/agent`) | | |
| API/UI (`src/internal/api`) | | |

## Chat logs

Session logs live in `ai_usage/chat-logs/` — see that folder's
[`README.md`](chat-logs/README.md) for the naming convention and what to
capture. First entry: `2026-09-04_repo-scaffolding_HE.md`.

## Team contribution notes

- (per-member note on what they used AI for vs. wrote themselves, to support
  the report's team-contributions section)
