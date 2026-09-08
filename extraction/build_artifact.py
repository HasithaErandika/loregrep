"""Stage 1 entry point: walks `Ashen_Era_Archive/`, dispatches every source
file to the right parser, and writes the merged artifact Stage 2 loads --
`data/chunks.json`.

Usage:
    python build_artifact.py                  # full corpus
    python build_artifact.py --sample 2        # ~2 files per format per
                                                # category, for a fast
                                                # end-to-end smoke test (see
                                                # README.md -> "What to
                                                # build first")
    python build_artifact.py --skip-ocr        # skip scanned pages/plates
                                                # entirely (fast iteration;
                                                # no OCR chunks in the output)
    python build_artifact.py --archive PATH    # archive root, if not at
                                                # the default ../Ashen_Era_Archive
    python build_artifact.py --out PATH        # output path, if not the
                                                # default ../data/chunks.json

## Output shape

    {
      "meta": {
        "generated_at": "<ISO 8601 UTC>",
        "archive_root": "Ashen_Era_Archive",
        "document_count": int,
        "chunk_count": int,
        "entity_count": int,
        "by_category": {"chronicles": int, "codex": int, ...},
        "by_format": {"pdf": int, "docx": int, "md": int, "txt": int,
                       "scan_pdf": int, "image": int},
        "by_extraction_method": {"pdfplumber": int, "python-docx": int, ...},
        "low_confidence_chunk_count": int,
        "ocr_available": bool
      },
      "chunks": [ ... see common.py's chunk schema docstring ... ]
    }

This is a new artifact -- Stage 2 (`src/`) doesn't exist yet -- so the shape
above is this script's own design decision, not something dictated
elsewhere; see `docs/decisions.md` for the reasoning (a `meta` wrapper
around the chunk array, rather than a bare array, so Stage 2 and anyone
inspecting the file get corpus stats for free).
"""
from __future__ import annotations

import argparse
import datetime
import json
import re
import sys
import time
from collections import Counter, defaultdict
from pathlib import Path

from common import (
    ChunkBuilder,
    EntityMatcher,
    chunk_long_text,
    clean_wiki_markup,
    group_paragraphs,
    load_entities_from_wiki,
)
from ocr_scans import OCRUnavailable, ocr_image, ocr_pdf, ocr_pdf_page
from parse_docx import parse_docx
from parse_pdfs import parse_pdf

DEFAULT_ARCHIVE_ROOT = Path(__file__).resolve().parent.parent / "Ashen_Era_Archive"
DEFAULT_OUTPUT_PATH = Path(__file__).resolve().parent.parent / "data" / "chunks.json"

CATEGORY_DIRS = ("chronicles", "codex", "ephemera", "wiki")

# images/ is a byte-identical duplicate of codex/images/ (verified by diff --
# same filenames, same bytes) -- only codex/images/ is processed, to avoid
# emitting the same OCR chunk twice under two different doc_ids. See
# docs/decisions.md.
IMAGE_DIRS = {"codex/images": "codex", "wiki/images": "wiki"}


# ---------------------------------------------------------------------------
# Markdown / plain-text parsing (no dedicated parser module -- see
# docs/architecture.md: only PDF, DOCX, and OCR get one, since Markdown/txt
# need no real library, just line-based splitting)
# ---------------------------------------------------------------------------

_HEADING_RE = re.compile(r"^#{2,6}\s+(.+)$")
_TABLE_SEP_RE = re.compile(r"^\|?[\s:|-]+\|?$")


def _parse_markdown(text: str) -> tuple[str, list[dict]]:
    """Parse a wiki article into (title, blocks), where blocks mirror
    `parse_docx.py`'s shape (`heading` / `text` / `table`) so both can be
    consumed by the same downstream chunking logic.

    `## `-level headings become section boundaries; a single leading `# `
    line is the article title, not a section. Pipe-table rows are collected
    into `table` blocks; `[[Entity]]` wiki links are flattened to plain text
    via `common.clean_wiki_markup` before being stored.
    """
    title: str | None = None
    blocks: list[dict] = []
    para_lines: list[str] = []
    table_lines: list[str] = []

    def push_paragraph() -> None:
        if para_lines:
            para = " ".join(line.strip() for line in para_lines).strip()
            if para:
                blocks.append({"type": "text", "text": clean_wiki_markup(para)})
            para_lines.clear()

    def push_table() -> None:
        if table_lines:
            rows = []
            for line in table_lines:
                if _TABLE_SEP_RE.match(line):
                    continue
                cells = [clean_wiki_markup(c.strip()) for c in line.strip().strip("|").split("|")]
                if any(cells):
                    rows.append(cells)
            if rows:
                blocks.append({"type": "table", "rows": rows})
            table_lines.clear()

    for raw_line in text.splitlines():
        line = raw_line.strip()
        if line.startswith("# ") and title is None:
            title = line[2:].strip()
            continue
        heading_match = _HEADING_RE.match(line)
        if heading_match:
            push_paragraph()
            push_table()
            blocks.append({"type": "heading", "text": heading_match.group(1).strip()})
            continue
        if line.startswith("|"):
            push_paragraph()
            table_lines.append(line)
            continue
        push_table()
        if not line:
            push_paragraph()
        else:
            para_lines.append(line)

    push_table()
    push_paragraph()
    return title or "Untitled", blocks


def _parse_txt(text: str) -> tuple[str | None, list[dict]]:
    """Parse a plain-text ephemera file into (title, blocks).

    Ephemera `.txt` files use a Markdown-style `# Title` first line despite
    the `.txt` extension (observed across the corpus) -- honored if present.
    Paragraphs are split on blank lines, since these files use them
    consistently, and each becomes its own `text` block.
    """
    lines = text.splitlines()
    title = None
    if lines and lines[0].strip().startswith("# "):
        title = lines[0].strip()[2:].strip()
        text = "\n".join(lines[1:])
    paragraphs = [p.strip() for p in re.split(r"\n\s*\n", text) if p.strip()]
    return title, [{"type": "text", "text": p} for p in paragraphs]


# ---------------------------------------------------------------------------
# Block -> chunk emission
# ---------------------------------------------------------------------------


def _emit_sectioned_blocks(builder: ChunkBuilder, blocks: list[dict], extraction_method: str) -> None:
    """Consume `heading`/`text`/`table` blocks (docx or markdown/txt shape)
    into chunks: consecutive `text` blocks are grouped under the nearest
    preceding heading into ~MAX_CHUNK_CHARS chunks; a table flushes any
    pending group first so section attribution stays accurate."""
    section: str | None = None
    pending: list[str] = []

    def flush() -> None:
        nonlocal pending
        for piece in group_paragraphs(pending):
            builder.add_text(piece, page=None, section=section, extraction_method=extraction_method)
        pending = []

    for block in blocks:
        if block["type"] == "heading":
            flush()
            section = block["text"]
        elif block["type"] == "text":
            pending.append(block["text"])
        elif block["type"] == "table":
            flush()
            builder.add_table(block["rows"], page=None, section=section, extraction_method=extraction_method)
    flush()


def _emit_pdf_blocks(builder: ChunkBuilder, blocks: list[dict], path: Path, skip_ocr: bool, warn) -> None:
    for block in blocks:
        if block["type"] == "text":
            for piece in chunk_long_text(block["text"]):
                builder.add_text(piece, page=block["page"], section=None, extraction_method="pdfplumber")
        elif block["type"] == "table":
            builder.add_table(block["rows"], page=block["page"], section=None, extraction_method="pdfplumber")
        elif block["type"] == "needs_ocr":
            if skip_ocr:
                continue
            try:
                result = ocr_pdf_page(path, block["page"])
            except OCRUnavailable as exc:
                warn(exc)
                continue
            if result:
                builder.add_text(
                    result["text"],
                    page=block["page"],
                    section=None,
                    extraction_method="tesseract-ocr",
                    extraction_confidence=result["confidence"],
                )


def _emit_ocr_pdf(builder: ChunkBuilder, path: Path, warn) -> None:
    try:
        blocks = ocr_pdf(path)
    except OCRUnavailable as exc:
        warn(exc)
        return
    for block in blocks:
        for piece in chunk_long_text(block["text"]):
            builder.add_text(
                piece,
                page=block["page"],
                section=None,
                extraction_method="tesseract-ocr",
                extraction_confidence=block["confidence"],
            )


# ---------------------------------------------------------------------------
# Per-document / per-image processing
# ---------------------------------------------------------------------------


def _title_from_filename(stem: str) -> str:
    return stem.replace("_", " ").replace("-", " ").strip().title()


_MAX_TITLE_CHARS = 120


def _pdf_title(blocks: list[dict]) -> str | None:
    """Best-effort title from a PDF's first text block.

    A dedicated title page (chronicles/codex covers) is just the title,
    wrapped across 2-3 lines with no other content before the first blank
    line -- collapsing whitespace joins those back into one readable title.
    Ephemera PDFs mostly have no title page at all (page 1 is straight into
    body text with no early blank line), so that same first-paragraph
    heuristic would swallow the whole page; falling back to just the first
    line, and discarding anything still too long to plausibly be a title,
    keeps this from ever producing a paragraph-length "title".
    """
    if not blocks or blocks[0]["type"] != "text":
        return None
    text = blocks[0]["text"]
    candidate = re.sub(r"\s+", " ", text.split("\n\n")[0]).strip()
    if len(candidate) <= _MAX_TITLE_CHARS:
        return candidate
    first_line = text.splitlines()[0].strip()
    return first_line if first_line and len(first_line) <= _MAX_TITLE_CHARS else None


def _process_document(
    path: Path, archive_root: Path, category: str, matcher: EntityMatcher, skip_ocr: bool, warn
) -> tuple[list[dict], str]:
    """Returns (chunks, format_label) for one source document."""
    rel = path.relative_to(archive_root).as_posix()
    name_lower = path.name.lower()

    if name_lower.endswith(".scan.pdf"):
        title = _title_from_filename(path.name[: -len(".scan.pdf")])
        builder = ChunkBuilder(rel, title, category, "scan_pdf", matcher)
        if not skip_ocr:
            _emit_ocr_pdf(builder, path, warn)
        return builder.chunks(), "scan_pdf"

    suffix = path.suffix.lower()

    if suffix == ".pdf":
        blocks = parse_pdf(path)
        title = _pdf_title(blocks)
        title = title or _title_from_filename(path.stem)
        builder = ChunkBuilder(rel, title, category, "pdf", matcher)
        _emit_pdf_blocks(builder, blocks, path, skip_ocr, warn)
        return builder.chunks(), "pdf"

    if suffix == ".docx":
        blocks = parse_docx(path)
        title = next((b["text"] for b in blocks if b["type"] == "heading"), None) or _title_from_filename(path.stem)
        builder = ChunkBuilder(rel, title, category, "docx", matcher)
        _emit_sectioned_blocks(builder, blocks, extraction_method="python-docx")
        return builder.chunks(), "docx"

    if suffix == ".md":
        text = path.read_text(encoding="utf-8")
        title, blocks = _parse_markdown(text)
        builder = ChunkBuilder(rel, title, category, "md", matcher)
        _emit_sectioned_blocks(builder, blocks, extraction_method="markdown")
        return builder.chunks(), "md"

    if suffix == ".txt":
        text = path.read_text(encoding="utf-8")
        title, blocks = _parse_txt(text)
        title = title or _title_from_filename(path.stem)
        builder = ChunkBuilder(rel, title, category, "txt", matcher)
        _emit_sectioned_blocks(builder, blocks, extraction_method="plain-text")
        return builder.chunks(), "txt"

    return [], "unknown"


def _process_image(
    path: Path, archive_root: Path, category: str, matcher: EntityMatcher, skip_ocr: bool, warn
) -> list[dict]:
    rel = path.relative_to(archive_root).as_posix()
    title = _title_from_filename(path.stem)
    builder = ChunkBuilder(rel, title, category, "image", matcher)
    if not skip_ocr:
        try:
            result = ocr_image(path)
        except OCRUnavailable as exc:
            warn(exc)
            result = None
        if result:
            builder.add_text(
                result["text"],
                page=None,
                section=None,
                extraction_method="tesseract-ocr",
                extraction_confidence=result["confidence"],
            )
    return builder.chunks()


# ---------------------------------------------------------------------------
# Corpus walking
# ---------------------------------------------------------------------------


def _sample_paths(directory: Path, n: int) -> list[Path]:
    """Up to `n` files per distinct format (`.pdf`, `.docx`, `.md`, `.txt`,
    and `.scan.pdf` counted separately from `.pdf`), for a fast, format-
    diverse smoke-test subset -- per README.md's "What to build first"."""
    buckets: dict[str, list[Path]] = defaultdict(list)
    for path in sorted(directory.iterdir()):
        if not path.is_file() or path.name.startswith("."):
            continue
        key = "scan_pdf" if path.name.lower().endswith(".scan.pdf") else path.suffix.lower()
        if len(buckets[key]) < n:
            buckets[key].append(path)
    return sorted(p for paths in buckets.values() for p in paths)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--archive", type=Path, default=DEFAULT_ARCHIVE_ROOT, help="Path to Ashen_Era_Archive/")
    parser.add_argument("--out", type=Path, default=DEFAULT_OUTPUT_PATH, help="Output path for chunks.json")
    parser.add_argument("--sample", type=int, default=None, metavar="N", help="Process only ~N files per format per category (smoke test)")
    parser.add_argument("--skip-ocr", action="store_true", help="Skip OCR entirely (scanned pages/plates produce no chunks)")
    args = parser.parse_args()

    archive_root = args.archive.resolve()

    if not archive_root.is_dir():
        print(f"error: archive root not found: {archive_root}", file=sys.stderr)
        print("See README.md -> 'Obtaining the corpus'.", file=sys.stderr)
        return 1

    matcher = EntityMatcher(load_entities_from_wiki(archive_root))
    print(f"Loaded {len(matcher)} entities from wiki/ for tagging.")

    warned = {"done": False, "skip_ocr": args.skip_ocr}

    def warn(exc: OCRUnavailable) -> None:
        if not warned["done"]:
            print(f"warning: {exc}", file=sys.stderr)
            print("warning: continuing without OCR for the remainder of this run.", file=sys.stderr)
            warned["done"] = True
        warned["skip_ocr"] = True

    all_chunks: list[dict] = []
    by_category: Counter = Counter()
    by_format: Counter = Counter()
    by_extraction_method: Counter = Counter()
    document_count = 0
    low_confidence_count = 0
    start = time.time()

    for category in CATEGORY_DIRS:
        category_dir = archive_root / category
        if not category_dir.is_dir():
            continue
        paths = _sample_paths(category_dir, args.sample) if args.sample else sorted(
            p for p in category_dir.iterdir() if p.is_file() and not p.name.startswith(".")
        )
        for path in paths:
            chunks, fmt = _process_document(path, archive_root, category, matcher, warned["skip_ocr"], warn)
            if not chunks and fmt == "unknown":
                continue
            document_count += 1
            by_category[category] += len(chunks)
            by_format[fmt] += len(chunks)
            all_chunks.extend(chunks)

    for image_dir, category in IMAGE_DIRS.items():
        directory = archive_root / image_dir
        if not directory.is_dir():
            continue
        paths = (
            _sample_paths(directory, args.sample)
            if args.sample
            else sorted(p for p in directory.iterdir() if p.is_file() and not p.name.startswith("."))
        )
        for path in paths:
            chunks = _process_image(path, archive_root, category, matcher, warned["skip_ocr"], warn)
            document_count += 1
            by_category[category] += len(chunks)
            by_format["image"] += len(chunks)
            all_chunks.extend(chunks)

    for chunk in all_chunks:
        by_extraction_method[chunk["extraction_method"]] += 1
        if chunk["low_confidence"]:
            low_confidence_count += 1

    meta = {
        "generated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "archive_root": str(archive_root.name),
        "document_count": document_count,
        "chunk_count": len(all_chunks),
        "entity_count": len(matcher),
        "by_category": dict(by_category),
        "by_format": dict(by_format),
        "by_extraction_method": dict(by_extraction_method),
        "low_confidence_chunk_count": low_confidence_count,
        "ocr_available": not warned["done"],
        "sample_mode": args.sample,
    }

    args.out.parent.mkdir(parents=True, exist_ok=True)
    with args.out.open("w", encoding="utf-8") as f:
        json.dump({"meta": meta, "chunks": all_chunks}, f, ensure_ascii=False, indent=1)

    elapsed = time.time() - start
    print(f"Wrote {len(all_chunks)} chunks from {document_count} documents -> {args.out} ({elapsed:.1f}s)")
    print(f"By category: {dict(by_category)}")
    print(f"By format: {dict(by_format)}")
    print(f"By extraction method: {dict(by_extraction_method)}")
    if low_confidence_count:
        print(f"Low-confidence OCR chunks: {low_confidence_count}")
    if warned["done"]:
        print("OCR was unavailable during this run -- see the warning above.", file=sys.stderr)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
