"""Stage 1 parser: text-based PDF -> raw block list.

Text and tables are extracted with `pdfplumber`, which -- unlike PyMuPDF --
understands table structure well enough to return actual rows/columns
rather than just a flattened text stream; that's the whole reason it's the
primary tool here (see `docs/decisions.md`). `*.scan.pdf` files (scanned,
no text layer) never reach this module -- `build_artifact.py` routes those
straight to `ocr_scans.py`. This module exists for the ordinary `.pdf`
files in `chronicles/` and `codex/`.
"""
from __future__ import annotations

import pdfplumber


def parse_pdf(path) -> list[dict]:
    """Parse one text-based PDF into an ordered list of raw blocks.

    Each block is one of:
      {"type": "text", "page": <1-indexed int>, "text": str}
      {"type": "table", "page": <1-indexed int>, "rows": list[list[str]]}
      {"type": "needs_ocr", "page": <1-indexed int>}

    `needs_ocr` covers the edge case of an ordinary `.pdf` (not suffixed
    `.scan.pdf`) that turns out to have a page with no extractable text but
    an embedded image -- i.e. it's actually a scan someone forgot to name
    accordingly. `build_artifact.py` hands those pages to
    `ocr_scans.ocr_pdf_page` as a fallback.

    Table rows are returned in addition to the page's plain text (which
    also contains the table's cell values inline) -- some duplication, but
    keeping both means `keyword_search` still finds table content in prose
    context and `table_lookup` still gets clean structured rows.
    """
    blocks: list[dict] = []
    with pdfplumber.open(path) as pdf:
        for page_number, page in enumerate(pdf.pages, start=1):
            tables = page.extract_tables() or []
            text = (page.extract_text() or "").strip()

            if not text and not tables:
                if page.images:
                    blocks.append({"type": "needs_ocr", "page": page_number})
                continue

            if text:
                blocks.append({"type": "text", "page": page_number, "text": text})

            for rows in tables:
                cleaned = [[(cell or "").strip() for cell in row] for row in rows]
                if any(any(cell for cell in row) for row in cleaned):
                    blocks.append({"type": "table", "page": page_number, "rows": cleaned})

    return blocks
