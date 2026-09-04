# Chat logs

Raw evidence for the human-AI collaboration grading criteria. This is where
corrections and rejected suggestions live, not just accepted output — the
disclosure doc (`../ai-usage-disclosure.md`) summarizes; this folder proves
it.

## Naming convention

```
YYYY-MM-DD_<short-topic>_<author-initials>.md
```

Example: `2026-09-04_repo-scaffolding_HE.md`.

One file per meaningful session (not per message) — a session that covers
several small back-and-forth exchanges on one topic is one file.

## What to capture

- What was asked and why (one line of context is enough)
- What the AI proposed or produced
- **Any correction, rejection, or redirection the human gave** — this is
  the part that's actually being scored, since "accepted the first
  suggestion every time" and "caught and fixed a bad suggestion" look
  identical if only the final output is kept
- The outcome

A tool's native export (e.g. a Claude Code transcript, a ChatGPT share
link saved as a file) is fine to drop in directly. A short human-written
summary in the format above is also fine — it doesn't need to be a verbatim
transcript, but it does need to be honest about what was corrected.

## Commit attribution note

Commits made with Claude Code in this repo carry:

```
Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
Claude-Session: <session URL>
```

That trailer is itself part of the audit trail — it links a commit back to
the exact session that produced it — but it doesn't replace a log entry
here when the session involved a real decision or correction worth
recording for the report.
