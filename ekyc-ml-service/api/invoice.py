"""POST /v1/invoice/extract — invoice field extraction from a PDF or image.

Input routing:
  * PDF with a text layer  → words + bboxes straight from PyMuPDF (engine
    "pdf-text"); no OCR, confidence is the text layer's (0.98/word).
  * PDF without text (scan) → pages 1–2 rendered at 200 dpi and OCR'd with
    the same engine selection as /v1/document/extract (ppocr | tesseract).
  * Image (PNG/JPEG/…)      → OCR directly.

Error contract mirrors the document endpoint: unreadable input → 422, no OCR
engine when one is needed → 503, empty upload → 400. Never fabricate fields.
"""
from __future__ import annotations

from fastapi import APIRouter, File, HTTPException, UploadFile

from api.document import choose_ocr_engine
from engine.fields import OcrWord

router = APIRouter()

_PDF_RENDER_DPI = 200
_MAX_PAGES = 2
_TEXT_LAYER_MIN_WORDS = 15  # fewer than this → treat as a scan and OCR
_TEXT_CONF = 0.98


def _is_pdf(data: bytes) -> bool:
    return data.lstrip()[:5] == b"%PDF-"


def pdf_text_words(doc) -> list[OcrWord]:
    """Text-layer words of the first pages as OcrWords, grouped into visual
    lines by vertical overlap (so two-column headers stay on one line, same
    as the OCR engines' line clustering) and ordered top→bottom, left→right."""
    words: list[OcrWord] = []
    line_no = 0
    for pno in range(min(_MAX_PAGES, doc.page_count)):
        raw = doc[pno].get_text("words")  # x0,y0,x1,y1,word,block,line,wordno
        raw = [r for r in raw if r[4].strip()]
        raw.sort(key=lambda r: ((r[1] + r[3]) / 2, r[0]))
        lines: list[list] = []
        for r in raw:
            cy = (r[1] + r[3]) / 2
            h = max(1.0, r[3] - r[1])
            if lines:
                last = lines[-1]
                lcy = sum((w[1] + w[3]) / 2 for w in last) / len(last)
                if abs(cy - lcy) <= 0.5 * h:
                    last.append(r)
                    continue
            lines.append([r])
        for ln in lines:
            ln.sort(key=lambda r: r[0])
            for r in ln:
                words.append(OcrWord(text=r[4], confidence=_TEXT_CONF, line=line_no))
            line_no += 1
    return words


def _render_pages(doc) -> list[bytes]:
    import fitz  # PyMuPDF

    zoom = _PDF_RENDER_DPI / 72.0
    out = []
    for pno in range(min(_MAX_PAGES, doc.page_count)):
        pix = doc[pno].get_pixmap(matrix=fitz.Matrix(zoom, zoom), alpha=False)
        out.append(pix.tobytes("png"))
    return out


def _ocr_images(images: list[bytes]) -> tuple[list[OcrWord], str, str]:
    from engine import ocr, ppocr

    engine_name = choose_ocr_engine()
    run = ppocr.run_ocr if engine_name == "ppocr" else ocr.run_ocr
    words: list[OcrWord] = []
    texts: list[str] = []
    offset = 0
    for img in images:
        out = run(img)
        for w in out.words:
            words.append(OcrWord(w.text, w.confidence, w.line + offset))
        offset += (max((w.line for w in out.words), default=-1) + 1)
        texts.append(out.text)
    return words, "\n".join(texts), engine_name


@router.post("/invoice/extract")
async def extract_invoice_endpoint(file: UploadFile = File(...)):
    from engine.invoice import extract_invoice

    data = await file.read()
    if not data:
        raise HTTPException(400, "empty file")

    if _is_pdf(data):
        try:
            import fitz
        except ImportError as e:  # pragma: no cover - deployment error
            raise HTTPException(503, "PDF support (PyMuPDF) not installed") from e
        try:
            doc = fitz.open(stream=data, filetype="pdf")
            if doc.page_count == 0:
                raise ValueError("PDF has no pages")
            words = pdf_text_words(doc)
        except Exception as e:
            raise HTTPException(422, f"could not read PDF: {e}") from e

        if len(words) >= _TEXT_LAYER_MIN_WORDS:
            engine_name = "pdf-text"
            text = "\n".join(
                " ".join(w.text for w in words if w.line == ln)
                for ln in sorted({w.line for w in words})
            )
        else:
            try:
                images = _render_pages(doc)
            except Exception as e:
                raise HTTPException(422, f"could not render PDF: {e}") from e
            engine_name = choose_ocr_engine()  # 503 before any heavy work
            try:
                words, text, engine_name = _ocr_images(images)
            except HTTPException:
                raise
            except Exception as e:
                raise HTTPException(422, f"could not OCR PDF pages: {e}") from e
    else:
        choose_ocr_engine()
        try:
            words, text, engine_name = _ocr_images([data])
        except HTTPException:
            raise
        except Exception as e:
            raise HTTPException(422, f"could not OCR image: {e}") from e

    if not words:
        raise HTTPException(422, "no text could be read from the document")

    return extract_invoice(words, text, engine=engine_name).to_dict()
