"""Shared helpers for the Stage 1 extraction pipeline.

Used by `parse_pdfs.py`, `parse_docx.py`, `ocr_scans.py`, and
`build_artifact.py` so chunk shape, entity tagging, text chunking, and
reliability tiering stay consistent no matter which parser produced the raw
text. This module is plumbing, not a parser itself -- `docs/architecture.md`
lists one parser per source format (`parse_pdfs.py`, `parse_docx.py`,
`ocr_scans.py`) plus `build_artifact.py` as the merge step; `common.py` is
the shared code all four of those import from.

## Chunk schema

Every entry `build_artifact.py` writes to `data/chunks.json` has this shape:

    {
      "id": "wiki/ashreach.md::002",       # f"{doc_id}::{index:03d}"
      "doc_id": "wiki/ashreach.md",        # path relative to Ashen_Era_Archive/
      "doc_title": "Ashreach",
      "category": "wiki",                  # chronicles | codex | ephemera | wiki
      "format": "md",                      # pdf | docx | md | txt | image
      "reliability_tier": "reference",     # see RELIABILITY_BY_CATEGORY below
      "page": null,                        # 1-indexed page number, or null when
                                            # the source format has no native
                                            # pagination (docx/md/txt/image)
      "section": "History",                # nearest heading, or null
      "chunk_type": "text",                # text | table
      "text": "...",                       # present when chunk_type == "text"
      "table": [["Field", "Value"], ...],  # present when chunk_type == "table"
      "entities": ["Ashreach", "The Bleeding Crown"],
      "extraction_method": "markdown",     # markdown | plain-text | python-docx
                                            # | pdfplumber | tesseract-ocr
      "extraction_confidence": null,       # 0-100 mean OCR word confidence;
                                            # null for non-OCR chunks
      "low_confidence": false,             # true when an OCR chunk's
                                            # confidence < LOW_CONFIDENCE_THRESHOLD
      "char_count": 512
    }

`page: null` for docx/md/txt/image is a deliberate choice, not an oversight:
python-docx (and plain text/Markdown) don't carry real page boundaries --
pagination only exists once a document is *rendered*, which Stage 1 never
does for those formats. Fabricating a page number by guessing characters-
per-page would be more misleading than omitting it. `section` is the
citation anchor for those formats instead. See `docs/limitations.md`.
"""
from __future__ import annotations

import re
from pathlib import Path

# ---------------------------------------------------------------------------
# Reliability tiers
# ---------------------------------------------------------------------------

# Ashen_Era_Archive/README.txt is explicit that ephemera's in-world authors
# "are not always reliable" -- codex/wiki are the closest things the corpus
# has to a maintained reference, chronicles are narrative prose (accurate to
# the setting but told in a storytelling voice), and ephemera is first-person
# in-world material that the agent should treat with more skepticism.
RELIABILITY_BY_CATEGORY = {
    "codex": "official",
    "wiki": "reference",
    "chronicles": "narrative",
    "ephemera": "in_world_unreliable",
}

CATEGORIES = tuple(RELIABILITY_BY_CATEGORY)

# OCR mean word confidence (0-100, from Tesseract's image_to_data) below
# which a chunk is flagged low_confidence. Shared with ocr_scans.py, which
# is the only producer of chunks that set extraction_confidence.
LOW_CONFIDENCE_THRESHOLD = 60

# ---------------------------------------------------------------------------
# Text chunking
# ---------------------------------------------------------------------------

MAX_CHUNK_CHARS = 1600


def group_paragraphs(paragraphs: list[str], max_chars: int = MAX_CHUNK_CHARS) -> list[str]:
    """Greedily pack a list of paragraph strings into ~max_chars chunks.

    Never splits a paragraph itself, so a single very long paragraph becomes
    its own oversized chunk rather than being cut mid-sentence.
    """
    chunks: list[str] = []
    buf: list[str] = []
    buf_len = 0
    for para in paragraphs:
        para = para.strip()
        if not para:
            continue
        if buf and buf_len + len(para) + 1 > max_chars:
            chunks.append("\n".join(buf))
            buf, buf_len = [], 0
        buf.append(para)
        buf_len += len(para) + 1
    if buf:
        chunks.append("\n".join(buf))
    return chunks


def chunk_long_text(text: str, max_chars: int = MAX_CHUNK_CHARS) -> list[str]:
    """Split one block of raw text (a whole PDF/OCR page, a .txt file) into
    ~max_chars chunks on the best available boundary.

    PDF and OCR page text often has no blank-line paragraph breaks (just a
    newline per visual line), unlike Markdown/docx source -- so this tries
    blank-line breaks first, falls back to single newlines, and falls back
    again to sentence-ending punctuation, rather than assuming any one
    format's line convention.
    """
    text = text.strip()
    if not text:
        return []
    if len(text) <= max_chars:
        return [text]

    units = [u.strip() for u in text.split("\n\n") if u.strip()]
    if len(units) < 2:
        units = [u.strip() for u in text.split("\n") if u.strip()]
    if len(units) < 2:
        units = [u.strip() for u in re.split(r"(?<=[.!?])\s+", text) if u.strip()]
    return group_paragraphs(units, max_chars=max_chars)


# ---------------------------------------------------------------------------
# Entity tagging
# ---------------------------------------------------------------------------

WIKI_LINK_RE = re.compile(r"\[\[([^\]|#]+)(?:[^\]]*)?\]\]")


def clean_wiki_markup(text: str) -> str:
    """Turn `[[Entity Name]]` / `[[Entity Name|display]]` wiki links into
    plain prose (`Entity Name`) so chunk text reads naturally and entity
    tagging can match against it directly."""
    return WIKI_LINK_RE.sub(lambda m: m.group(1).strip(), text)


def load_entities_from_wiki(archive_root: Path) -> list[str]:
    """Build the canonical entity list from each wiki article's `# Title`
    line -- this is the "cross-reference graph seeded from the wiki's ~90
    articles" from docs/architecture.md, and the same list doubles as the
    entity vocabulary Stage 1 tags every chunk (of any format) against."""
    entities: list[str] = []
    wiki_dir = archive_root / "wiki"
    for path in sorted(wiki_dir.glob("*.md")):
        text = path.read_text(encoding="utf-8")
        match = re.search(r"^#\s+(.+)$", text, re.MULTILINE)
        if match:
            entities.append(match.group(1).strip())
    return entities


class EntityMatcher:
    """Case-insensitive, whole-word/phrase matcher over a fixed entity list.

    Longest names are checked first so e.g. "The Bleeding Crown" is tagged
    as one entity rather than any shorter name that happened to be a
    substring of it.
    """

    def __init__(self, entities: list[str]):
        uniq = sorted(set(entities), key=len, reverse=True)
        self._patterns = [
            (name, re.compile(r"(?<!\w)" + re.escape(name) + r"(?!\w)", re.IGNORECASE))
            for name in uniq
        ]

    def tag(self, text: str, limit: int = 25) -> list[str]:
        if not text:
            return []
        found = []
        for name, pattern in self._patterns:
            if pattern.search(text):
                found.append(name)
                if len(found) >= limit:
                    break
        return found

    def __len__(self) -> int:
        return len(self._patterns)


# ---------------------------------------------------------------------------
# Chunk construction
# ---------------------------------------------------------------------------


class ChunkBuilder:
    """Accumulates chunks for a single source document and assigns each one
    a stable, sequential id (`f"{doc_id}::{index:03d}"`)."""

    def __init__(self, doc_id: str, doc_title: str, category: str, fmt: str, matcher: EntityMatcher):
        self.doc_id = doc_id
        self.doc_title = doc_title
        self.category = category
        self.format = fmt
        self.matcher = matcher
        self.reliability_tier = RELIABILITY_BY_CATEGORY[category]
        self._chunks: list[dict] = []

    def add_text(
        self,
        text: str,
        page: int | None,
        section: str | None,
        extraction_method: str,
        extraction_confidence: float | None = None,
    ) -> None:
        text = text.strip()
        if not text:
            return
        self._chunks.append(
            {
                "id": f"{self.doc_id}::{len(self._chunks):03d}",
                "doc_id": self.doc_id,
                "doc_title": self.doc_title,
                "category": self.category,
                "format": self.format,
                "reliability_tier": self.reliability_tier,
                "page": page,
                "section": section,
                "chunk_type": "text",
                "text": text,
                "table": None,
                "entities": self.matcher.tag(text),
                "extraction_method": extraction_method,
                "extraction_confidence": extraction_confidence,
                "low_confidence": (
                    extraction_confidence is not None
                    and extraction_confidence < LOW_CONFIDENCE_THRESHOLD
                ),
                "char_count": len(text),
            }
        )

    def add_table(
        self,
        rows: list[list[str]],
        page: int | None,
        section: str | None,
        extraction_method: str,
    ) -> None:
        rows = [[(cell or "").strip() for cell in row] for row in rows]
        if not any(any(cell for cell in row) for row in rows):
            return
        flat_text = "\n".join(" | ".join(row) for row in rows)
        self._chunks.append(
            {
                "id": f"{self.doc_id}::{len(self._chunks):03d}",
                "doc_id": self.doc_id,
                "doc_title": self.doc_title,
                "category": self.category,
                "format": self.format,
                "reliability_tier": self.reliability_tier,
                "page": page,
                "section": section,
                "chunk_type": "table",
                "text": None,
                "table": rows,
                "entities": self.matcher.tag(flat_text),
                "extraction_method": extraction_method,
                "extraction_confidence": None,
                "low_confidence": False,
                "char_count": len(flat_text),
            }
        )

    def chunks(self) -> list[dict]:
        return self._chunks
