"""Tesseract OCR wrapper (the model-dependent half of document extraction).

Why Tesseract over PaddleOCR: the tesseract-ocr Debian package plus the `eng`
traineddata adds ~40 MB to a python:3.12-slim image and installs from apt with
zero native-build risk; pytesseract is a thin subprocess wrapper. PaddleOCR
would pull the paddlepaddle wheel (hundreds of MB, occasionally broken on
slim/musl images) for accuracy we don't need at v1 — the checksummed MRZ, not
raw OCR accuracy, is the trust anchor for passports/ID cards, and the visual
zone heuristics only need workable word boxes.

Everything in this module needs the tesseract binary; unit tests exercise
engine.fields / engine.mrz instead and never import this file.
"""
from __future__ import annotations

import io
from dataclasses import dataclass

from engine.fields import OcrWord


@dataclass
class OcrOutput:
    words: list[OcrWord]
    text: str  # line-preserving full text (for MRZ scanning)


def tesseract_available() -> bool:
    import shutil

    return shutil.which("tesseract") is not None


def _preprocess(image_bytes: bytes):
    """Load, EXIF-orient, grayscale, upscale small images, autocontrast."""
    from PIL import Image, ImageOps

    img = Image.open(io.BytesIO(image_bytes))
    img = ImageOps.exif_transpose(img)
    img = img.convert("L")
    if min(img.size) < 700:  # phone thumbnails OCR poorly at native size
        scale = 700 / min(img.size)
        img = img.resize((round(img.width * scale), round(img.height * scale)))
    return ImageOps.autocontrast(img)


def run_ocr(image_bytes: bytes) -> OcrOutput:
    """OCR an ID-document image into words with confidences and full text.

    Two passes: a general pass for the visual zone, and an MRZ-charset pass
    (whitelist A-Z0-9<) whose text is appended so engine.mrz can find MRZ
    lines that the general pass mangles.
    """
    import pytesseract
    from pytesseract import Output

    img = _preprocess(image_bytes)

    data = pytesseract.image_to_data(img, output_type=Output.DICT)
    words: list[OcrWord] = []
    lines: dict[tuple, list[str]] = {}
    for i, text in enumerate(data["text"]):
        if not text.strip():
            continue
        conf = float(data["conf"][i])
        if conf < 0:  # tesseract uses -1 for non-word boxes
            continue
        key = (data["block_num"][i], data["par_num"][i], data["line_num"][i])
        line_no = list(lines.keys()).index(key) if key in lines else len(lines)
        lines.setdefault(key, []).append(text)
        words.append(OcrWord(text=text, confidence=conf / 100.0, line=line_no))

    general_text = "\n".join(" ".join(ws) for ws in lines.values())

    # MRZ pass: single-charset, treat the image as uniform text blocks.
    mrz_text = pytesseract.image_to_string(
        img, config="-c tessedit_char_whitelist=ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789< --psm 6"
    )

    return OcrOutput(words=words, text=general_text + "\n" + mrz_text)
