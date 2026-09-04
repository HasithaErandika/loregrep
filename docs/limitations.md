# Limitations

Honest, living record of what doesn't work or is known to be weak. Update
this as the build progresses — it's graded, and a vague or empty version at
submission time is worse than a short, specific one.

## Known risk areas (from planning, to be confirmed/updated during the build)

- **OCR on scanned ephemera pages** is the least predictable extraction
  step in either language. Tesseract output quality on the simulated scans
  in `Ashen_Era_Archive/ephemera/` is unverified as of this writing; a
  vision-LLM fallback is planned but not yet implemented/evaluated.
- **Cross-reference graph coverage** is seeded only from the ~90 wiki
  articles' entity list, not from every entity mentioned in raw prose across
  all 415 documents. Questions that hinge on an entity that only appears in
  chronicles/codex prose (never in the wiki) may not be reachable via
  `follow_reference`.
- **Agent loop iteration cap** — the planner/tool-router loop is capped at a
  fixed number of iterations (see `docs/decisions.md` / architecture) to
  guarantee termination during a live demo. A question that genuinely needs
  more hops than the cap will get a best-effort partial answer rather than
  a complete one.
- **In-world source reliability** — the corpus deliberately includes
  unreliable narrators (README.txt: "in-world authors are not always
  reliable"). The reliability-tier tagging in `chunks.json` is a heuristic
  set at extraction time, not a guarantee; conflicting-source detection is
  only as good as that tagging.
- **Vector fallback quality** — the semantic-search tool exists for
  conceptual/fuzzy questions but is a brute-force in-memory cosine store
  sized for this corpus only; it is not the primary retrieval path and has
  not been tuned as heavily as keyword/table/graph search.

## To fill in during the build

- [ ] Failure modes actually observed against `sample_questions.json`
- [ ] Extraction accuracy spot-checks (tables, scanned pages) with examples
- [ ] Any question categories the agent consistently gets wrong, and why
- [ ] Performance/latency notes if relevant under demo conditions
