"""Stage 1 parser: OCR for scanned PDFs (`*.scan.pdf`) and standalone image
plates (`codex/images/`, `wiki/images/`), via Tesseract.

Two Tesseract page-segmentation modes are tried for every page/image --
`--psm 6` (assume one uniform block of prose text; the common case for
scanned ephemera pages) and `--psm 12` (sparse text with orientation
detection, which reads bar-chart-style figure plates far more reliably --
their bold numeric labels are laid out nothing like a paragraph). Whichever
mode gets the higher mean per-word confidence from `image_to_data` is kept,
so one heuristic covers both content shapes without hard-coding a mode by
source folder. See `docs/limitations.md` for the spot-check this was based
on and its known misses.

A vision-LLM fallback for pages Tesseract still garbles badly (per
`docs/architecture.md`'s "Tesseract CLI, or a vision-capable OpenRouter
model when Tesseract output is unreliable") is planned but not implemented
here -- see `docs/limitations.md`.
"""
from __future__ import annotations

import io
import shutil
import subprocess
from pathlib import Path

import fitz  # PyMuPDF -- used here only to rasterize scan.pdf pages to images
import pytesseract
from PIL import Image

PSM_CANDIDATES = (6, 12)

# Below this mean word confidence, treat the result as noise rather than
# degraded-but-real text and drop it entirely (distinct from
# common.LOW_CONFIDENCE_THRESHOLD, which flags a kept chunk for review
# rather than discarding it). Needed because purely illustrative art --
# wiki/images/ character portraits and battle paintings, which have no
# embedded text at all -- makes Tesseract hallucinate short strings of
# near-random glyphs at ~30 mean confidence instead of returning nothing;
# genuine document text in this corpus scored 90+ in spot checks, so 35
# cleanly separates the two. See docs/limitations.md.
NOISE_FLOOR_CONFIDENCE = 35

# Common per-OS Tesseract install locations, checked only if the binary
# isn't already on PATH. Tesseract is a system package (README.md -> "Stage
# 1 environment"), so where it lands differs by OS/package manager -- this
# list is deliberately generous rather than assuming one platform.
_TESSERACT_CANDIDATES = (
    r"C:\Program Files\Tesseract-OCR\tesseract.exe",
    r"C:\Program Files (x86)\Tesseract-OCR\tesseract.exe",
    "/usr/bin/tesseract",
    "/usr/local/bin/tesseract",
    "/opt/homebrew/bin/tesseract",
)

_configured = False


class OCRUnavailable(RuntimeError):
    """Raised when no working Tesseract binary can be found.

    `build_artifact.py` catches this once, warns, and continues the run
    without OCR chunks rather than aborting the whole extraction -- see
    `docs/limitations.md`.
    """


def _configure_tesseract() -> None:
    """Point `pytesseract` at a working `tesseract` binary, once per process.

    `pytesseract` shells out to the `tesseract` CLI rather than being an OCR
    engine itself, so it needs the binary's path if it isn't already
    resolvable on PATH.
    """
    global _configured
    if _configured:
        return

    on_path = shutil.which("tesseract")
    if on_path:
        pytesseract.pytesseract.tesseract_cmd = on_path
        _configured = True
        return

    for candidate in _TESSERACT_CANDIDATES:
        if Path(candidate).exists():
            pytesseract.pytesseract.tesseract_cmd = candidate
            _configured = True
            return

    try:
        subprocess.run(["tesseract", "--version"], capture_output=True, check=True)
        _configured = True
        return
    except Exception:
        pass

    raise OCRUnavailable(
        "No Tesseract binary found on PATH or in common install locations. "
        "Install it (see README.md -> 'Stage 1 environment') and re-run."
    )


def _best_ocr(image: Image.Image) -> dict:
    """Run every candidate PSM mode against `image` and keep the one with
    the highest mean word confidence."""
    _configure_tesseract()
    best = None
    for psm in PSM_CANDIDATES:
        config = f"--psm {psm}"
        data = pytesseract.image_to_data(image, config=config, output_type=pytesseract.Output.DICT)
        confidences = [int(c) for c in data["conf"] if c not in ("-1", -1)]
        mean_confidence = (sum(confidences) / len(confidences)) if confidences else -1.0
        text = pytesseract.image_to_string(image, config=config).strip()
        candidate = {
            "text": text,
            "confidence": mean_confidence,
            "psm": psm,
            "n_tokens": len(confidences),
        }
        if best is None or candidate["confidence"] > best["confidence"]:
            best = candidate
    return best


def _is_real_text(result: dict | None) -> bool:
    return bool(result) and bool(result["text"]) and result["confidence"] >= NOISE_FLOOR_CONFIDENCE


def ocr_pdf(path, dpi: int = 300) -> list[dict]:
    """OCR every page of a `*.scan.pdf` file.

    Returns one block per non-empty page:
      {"type": "text", "page": <1-indexed int>, "text": str,
       "confidence": float, "psm": int}
    """
    blocks: list[dict] = []
    with fitz.open(path) as doc:
        for page_number, page in enumerate(doc, start=1):
            pixmap = page.get_pixmap(dpi=dpi)
            image = Image.open(io.BytesIO(pixmap.tobytes("png"))).convert("L")
            result = _best_ocr(image)
            if _is_real_text(result):
                blocks.append({"type": "text", "page": page_number, **result})
    return blocks


def ocr_pdf_page(path, page_number: int, dpi: int = 300) -> dict | None:
    """OCR a single page of an ordinary (non-`.scan.pdf`) PDF.

    Used only for the rare `needs_ocr` block `parse_pdfs.py` can emit when a
    normal `.pdf` has an image-only page it wasn't expected to have.
    `page_number` is 1-indexed to match `parse_pdfs.py`'s block numbering.
    """
    with fitz.open(path) as doc:
        page = doc[page_number - 1]
        pixmap = page.get_pixmap(dpi=dpi)
        image = Image.open(io.BytesIO(pixmap.tobytes("png"))).convert("L")
        result = _best_ocr(image)
    return result if _is_real_text(result) else None


def ocr_image(path, upscale: int = 2) -> dict | None:
    """OCR a single standalone image plate (a figure/chart or portrait, not
    a document page).

    Upscaled 2x before OCR by default -- Tesseract's default-resolution pass
    reliably missed the small bold numeric labels on the corpus's bar-chart
    style figure plates in spot checks; 2x fixed it (see
    `docs/limitations.md`). Purely illustrative art (character portraits,
    heraldry) has no embedded text at all and correctly OCRs to nothing --
    callers should treat an empty/near-empty result as "no caption to
    extract," not a failure.
    """
    image = Image.open(path).convert("L")
    if upscale and upscale != 1:
        width, height = image.size
        image = image.resize((width * upscale, height * upscale))
    result = _best_ocr(image)
    return result if _is_real_text(result) else None
