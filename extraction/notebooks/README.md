# Notebooks (exploration only)

`.ipynb` files here are for prototyping and eyeballing output — e.g. "does
`pdfplumber` find this codex table correctly," "what does the OCR output
look like for this scanned ephemera page." They are **not** run as part of
the pipeline and `build_artifact.py` never imports from this folder.

## The rule

Once logic in a notebook works, it moves into the real pipeline as a
function in one of `../parse_pdfs.py`, `../parse_docx.py`, `../ocr_scans.py`,
or `../build_artifact.py`. A notebook is scratch space, not a deliverable —
if `build_artifact.py` needs the logic, it needs to be a `.py` function that
can be imported and unit-tested, not a notebook cell.

## Why not run the pipeline as notebooks

- `.py` diffs cleanly in git and in code review; `.ipynb` is JSON with
  execution counts and (without care) embedded output/image blobs — bad for
  the git-discipline part of the rubric.
- Cell execution order can silently diverge from the file's top-to-bottom
  order, which is exactly the kind of bug you don't want in a script that
  produces the one artifact (`data/chunks.json`) the whole live system
  depends on.
- `build_artifact.py` needs to run non-interactively (`python
  build_artifact.py`) — notebooks aren't built for that.

## Keeping notebook diffs clean

This repo strips notebook outputs on commit via `nbstripout` (registered
in `.gitattributes`). One-time setup after cloning, from inside your Python
env (see `../../README.md` → "Stage 1 environment"):

```
nbstripout --install
```

After that, `git diff`/`git add` on a notebook only shows code+markdown
cell changes — no output noise, no embedded images bloating the repo.
