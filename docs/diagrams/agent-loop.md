# Agent decision loop

The core of the product (see `docs/decisions.md` → "Tool-first agent, not
vector-RAG-first"): the agent doesn't run a fixed number of retrieval
passes, it decides after every result whether to answer or search again,
capped so a live demo can never hang.

## Flow

```mermaid
flowchart TD
    start(["User question"]) --> plan["Planner:\nwhat do I need to find out first?"]
    plan --> pick{"Tool router:\npick a tool"}

    pick -->|"named entity / specific term\n(default first choice)"| kw["keyword_search(query)\nexact/fuzzy, BM25"]
    pick -->|"needs a specific data point"| tbl["table_lookup(entity, attribute)\nstructured codex tables"]
    pick -->|"needs 'what else connects to X'"| ref["follow_reference(entity)\ncross-reference graph"]
    pick -->|"conceptual/fuzzy,\nother tools came up empty"| sem["semantic_search(query)\nvector similarity — fallback only"]

    kw --> read["Read the result"]
    tbl --> read
    ref --> read
    sem --> read

    read --> suff{"Sufficiency check:\nenough to answer?"}
    suff -->|"no — refine using\nwhat was just learned"| cap{"Iteration cap\nreached?"}
    cap -->|no| plan
    cap -->|"yes — stop, don't hang"| synth
    suff -->|yes| synth["Synthesizer:\ncompose cited answer,\nflag conflicting sources"]

    synth --> answer(["Answer + citations\n(doc + page per claim)"])
```

## Why this shape, not top-k retrieval

- Each loop iteration is informed by what the *previous* result actually
  said — the planner can follow a cross-reference it just discovered, or
  re-query more narrowly after a keyword search returns too much.
- The tool choice is itself a decision, not "always embed and search":
  `semantic_search` is explicitly the last resort, used only when the
  question is genuinely conceptual or the structured tools found nothing.
- The iteration cap (see `docs/limitations.md`) exists purely for demo
  safety — a question that needs more hops than the cap gets a best-effort
  partial answer instead of the process hanging live in front of judges.
