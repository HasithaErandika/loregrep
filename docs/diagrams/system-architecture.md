# System architecture diagram

Two stages, split at the extraction/serving seam — see
[`docs/architecture.md`](../architecture.md) for the full explanation and
[`docs/decisions.md`](../decisions.md) for why the split exists.

```mermaid
flowchart TB
    subgraph S1["STAGE 1 — Offline extraction (Python, build-time only)"]
        direction TB
        corpus["Ashen_Era_Archive/\n415 docs · ~1,277 pages · 5 formats\n(local only, gitignored)"]
        parsePdf["parse_pdfs.py\npdfplumber (tables) + PyMuPDF (text/images)"]
        parseDocx["parse_docx.py\npython-docx"]
        ocr["ocr_scans.py\nTesseract / vision-LLM fallback"]
        build["build_artifact.py"]
        chunks[("data/chunks.json\ntext + tables + page refs +\nentity tags + reliability tier")]

        corpus --> parsePdf --> build
        corpus --> parseDocx --> build
        corpus --> ocr --> build
        build --> chunks
    end

    chunks -. "committed to repo" .-> S2

    subgraph S2["STAGE 2 — Online serving (Go, single binary, live/graded)"]
        direction TB
        loader["Loads chunks.json at startup"]

        subgraph idx["Indexes"]
            direction LR
            bleve["full-text\n(bleve, BM25)"]
            graphidx["cross-ref\ngraph"]
            vec["vector store\n(fallback only)"]
        end

        subgraph agent["Agent orchestrator"]
            direction LR
            planner["planner"] --> router["tool router"] --> suff["sufficiency\ncheck"]
            suff -- "not enough, refine" --> planner
            suff -- "enough" --> synth["synthesizer"]
        end

        api["API + demo UI\nlive reasoning trace + citations"]

        loader --> idx
        idx --> agent
        agent --> api
    end
```

## Notes

- The dashed arrow is the only thing that crosses the stage boundary:
  a single committed JSON file, not a live dependency.
- `vec` (vector store) is drawn inside the indexes but is only ever queried
  by the `semantic_search` tool as a fallback — see
  [`agent-loop.md`](agent-loop.md) for how the agent decides when to use it.
