"""PP-OCR (PaddleOCR-lineage) engine run as plain ONNX — no paddlepaddle.

Primary OCR engine when its model files are provisioned (`OCR_ENGINE=auto`,
the default, prefers it; Tesseract remains the fallback and can be forced
with `OCR_ENGINE=tesseract`). The original Tesseract-over-PaddleOCR decision
was about install footprint — the paddlepaddle wheel is hundreds of MB and
historically breaks on slim images — not accuracy. This module takes the
RapidOCR route instead: PaddleOCR's shipped detection/recognition networks
exported to ONNX (~14 MB total) executed by onnxruntime, the same
small-ONNX-file pattern as YuNet/SFace/MiniFASNet. PP-OCR is markedly better
than Tesseract on real-world phone photos of documents (glare, skew, low
contrast), which is exactly the visual-zone gap the current-state audit
flagged.

Pipeline (reference: PaddleOCR tools/infer/predict_{det,rec}.py, RapidOCR):

  1. DB text detection (`ch_PP-OCRv4_det_infer.onnx`, language-agnostic):
     image resized to ≤960 long side on multiples of 32, ImageNet-normalized;
     output probability map is thresholded (0.3), contoured, scored (mean
     prob inside the box, floor 0.5) and unclipped (×1.6) back to full-image
     quad boxes — one box per printed text line.
  2. CRNN/SVTR recognition (`en_PP-OCRv3_rec_infer.onnx`): each box is
     perspective-cropped, resized to 48×320 keep-AR, normalized to [-1, 1];
     the T×C output is greedy-CTC-decoded against the PaddleOCR `en_dict.txt`
     charset (95 printable ASCII chars — including `<`, so MRZ lines are
     representable — plus CTC blank at index 0 and, when the model's class
     count asks for it, the appended space class). Line confidence = mean
     probability of the kept characters.
  3. Boxes are clustered into visual lines by vertical overlap and emitted in
     reading order as `OcrWord`s (one per whitespace token, carrying the line
     confidence) plus line-preserving full text for the MRZ scanner.

Hybrid MRZ safety net: when the tesseract binary is also present, its
MRZ-charset pass (whitelist `A-Z0-9<`, psm 6) is appended to the text exactly
as in `engine.ocr` — the checksummed MRZ stays the trust anchor and we keep
the strongest of both readings. Absent tesseract, PP-OCR's own text is used.

Model paths (ops drop-ins, same pattern as the face/liveness models):

    PPOCR_DET_MODEL  (default /app/models/ppocr_det.onnx)
    PPOCR_REC_MODEL  (default /app/models/ppocr_rec.onnx)
    PPOCR_REC_DICT   (default /app/models/ppocr_keys_en.txt)

There is deliberately NO capped-fallback here: OCR either runs a real engine
or the endpoint fails 503 (fail closed) — a fabricated field list has no
safe cap the way a face-match score does.
"""
from __future__ import annotations

import os
from dataclasses import dataclass

from engine.fields import OcrWord
from engine.ocr import OcrOutput

# Detection (DB postprocess) — PaddleOCR defaults.
_DET_LIMIT_SIDE = 960
_DET_THRESH = 0.3  # pixel binarization threshold on the probability map
_DET_BOX_THRESH = 0.5  # minimum mean probability inside a candidate box
_DET_UNCLIP = 1.6  # box expansion ratio (text lines are detected shrunk)
_DET_MIN_SIDE = 3  # px; discard degenerate boxes

# Recognition — en_PP-OCRv3/v4 input geometry.
_REC_H = 48
_REC_MAX_W = 320
_MIN_LINE_CONF = 0.05  # drop unreadable garbage lines outright


def det_model_path() -> str:
    return os.getenv("PPOCR_DET_MODEL", "/app/models/ppocr_det.onnx")


def rec_model_path() -> str:
    return os.getenv("PPOCR_REC_MODEL", "/app/models/ppocr_rec.onnx")


def rec_dict_path() -> str:
    return os.getenv("PPOCR_REC_DICT", "/app/models/ppocr_keys_en.txt")


def models_available() -> bool:
    """True when all three model files exist and onnxruntime imports."""
    if not all(
        os.path.isfile(p)
        for p in (det_model_path(), rec_model_path(), rec_dict_path())
    ):
        return False
    try:
        import onnxruntime  # noqa: F401
    except ImportError:
        return False
    return True


# ─── pure logic (unit-tested without model files or cv2) ─────────────────────


def load_charset(dict_text: str) -> list[str]:
    """Charset from a PaddleOCR dict file's text: one character per line,
    order significant, blank lines at the end ignored."""
    return [line for line in dict_text.splitlines() if line != ""]


def charset_for_classes(charset: list[str], num_classes: int) -> list[str]:
    """Map the rec model's class count onto the charset.

    PaddleOCR prepends the CTC blank (index 0) and, when exported with
    use_space_char, appends a space class:
      num_classes == len(charset) + 2  ->  [blank] + charset + [' ']
      num_classes == len(charset) + 1  ->  [blank] + charset
    Anything else means the dict file doesn't belong to the model.
    """
    if num_classes == len(charset) + 2:
        return charset + [" "]
    if num_classes == len(charset) + 1:
        return list(charset)
    raise ValueError(
        f"rec model has {num_classes} classes but dict has {len(charset)} "
        "characters — PPOCR_REC_DICT does not match PPOCR_REC_MODEL"
    )


def ctc_greedy_decode(probs, charset: list[str]) -> tuple[str, float]:
    """Greedy CTC decode of a (T, C) probability matrix.

    Repeats are collapsed, blanks (class 0) dropped; charset[i-1] is the
    character for class i. Returns (text, mean probability of kept chars) —
    ("", 0.0) when nothing survives.
    """
    import numpy as np

    ids = np.argmax(probs, axis=1)
    confs = probs[np.arange(len(ids)), ids]
    chars: list[str] = []
    kept: list[float] = []
    prev = -1
    for i, c in zip(ids, confs):
        if i != prev and i != 0:
            chars.append(charset[i - 1])
            kept.append(float(c))
        prev = i
    if not chars:
        return "", 0.0
    return "".join(chars), float(sum(kept) / len(kept))


def cluster_lines(boxes: list[tuple]) -> list[list[int]]:
    """Group det box indices into visual lines by vertical-centre proximity.

    ``boxes`` are (x_min, y_min, x_max, y_max) tuples. Two boxes share a line
    when either's y-centre falls inside the other's vertical span. Returns
    line groups top-to-bottom, each group's indices sorted left-to-right.
    """
    order = sorted(range(len(boxes)), key=lambda i: (boxes[i][1], boxes[i][0]))
    lines: list[list[int]] = []
    for i in order:
        x0, y0, x1, y1 = boxes[i]
        cy = (y0 + y1) / 2
        for group in lines:
            gx0, gy0, gx1, gy1 = boxes[group[0]]
            gcy = (gy0 + gy1) / 2
            if gy0 <= cy <= gy1 or y0 <= gcy <= y1:
                group.append(i)
                break
        else:
            lines.append([i])
    for group in lines:
        group.sort(key=lambda i: boxes[i][0])
    lines.sort(key=lambda g: min(boxes[i][1] for i in g))
    return lines


# ─── ONNX pipeline ───────────────────────────────────────────────────────────

_sessions: dict | None = None


def _get_sessions():
    """Lazily created, process-cached onnxruntime sessions + charset."""
    global _sessions
    if _sessions is None:
        import onnxruntime as ort

        opts = ort.SessionOptions()
        opts.log_severity_level = 3
        with open(rec_dict_path(), encoding="utf-8") as fh:
            charset = load_charset(fh.read())
        _sessions = {
            "det": ort.InferenceSession(
                det_model_path(), opts, providers=["CPUExecutionProvider"]
            ),
            "rec": ort.InferenceSession(
                rec_model_path(), opts, providers=["CPUExecutionProvider"]
            ),
            "charset": charset,
        }
    return _sessions


def _decode_bgr(image_bytes: bytes):
    import cv2
    import numpy as np

    img = cv2.imdecode(np.frombuffer(image_bytes, np.uint8), cv2.IMREAD_COLOR)
    if img is None:
        raise ValueError("could not decode image")
    return img


def _det_boxes(img) -> list:
    """Run DB detection; returns 4-point float boxes in original-image coords."""
    import cv2
    import numpy as np

    sess = _get_sessions()["det"]
    h, w = img.shape[:2]
    scale = min(1.0, _DET_LIMIT_SIDE / max(h, w))
    rh = max(32, int(round(h * scale / 32)) * 32)
    rw = max(32, int(round(w * scale / 32)) * 32)
    resized = cv2.resize(img, (rw, rh)).astype(np.float32) / 255.0
    resized = (resized - (0.485, 0.456, 0.406)) / (0.229, 0.224, 0.225)
    blob = resized.transpose(2, 0, 1)[np.newaxis].astype(np.float32)

    prob = sess.run(None, {sess.get_inputs()[0].name: blob})[0][0, 0]
    binary = (prob > _DET_THRESH).astype(np.uint8)
    contours, _ = cv2.findContours(binary, cv2.RETR_LIST, cv2.CHAIN_APPROX_SIMPLE)

    sx, sy = w / rw, h / rh
    boxes = []
    for cnt in contours:
        rect = cv2.minAreaRect(cnt)
        (cx, cy), (bw, bh), angle = rect
        if min(bw, bh) < _DET_MIN_SIDE:
            continue
        # score: mean probability inside the contour polygon (the reference
        # box_score_fast) — a bounding-rect mean gets diluted by background
        # noise and drops genuine lines on degraded photos
        x, y, ww, hh = cv2.boundingRect(cnt)
        mask = np.zeros((hh, ww), dtype=np.uint8)
        cv2.fillPoly(mask, [cnt.reshape(-1, 2) - (x, y)], 1)
        if cv2.mean(prob[y : y + hh, x : x + ww], mask)[0] < _DET_BOX_THRESH:
            continue
        # unclip: DB predicts shrunk regions — expand by area/perimeter ratio
        offset = (bw * bh * _DET_UNCLIP) / (2 * (bw + bh))
        expanded = ((cx, cy), (bw + 2 * offset, bh + 2 * offset), angle)
        pts = cv2.boxPoints(expanded)
        pts[:, 0] = np.clip(pts[:, 0] * sx, 0, w - 1)
        pts[:, 1] = np.clip(pts[:, 1] * sy, 0, h - 1)
        boxes.append(pts.astype(np.float32))
    return boxes


def _order_quad(pts):
    """Order 4 points tl, tr, br, bl."""
    import numpy as np

    s = pts.sum(axis=1)
    d = np.diff(pts, axis=1).reshape(-1)
    return np.array(
        [pts[np.argmin(s)], pts[np.argmin(d)], pts[np.argmax(s)], pts[np.argmax(d)]],
        dtype=np.float32,
    )


def _rotate_crop(img, quad):
    """Perspective-crop a quad to an upright text-line image (PaddleOCR's
    get_rotate_crop_image, including the tall-box 90° rotation)."""
    import cv2
    import numpy as np

    quad = _order_quad(quad)
    cw = int(max(np.linalg.norm(quad[0] - quad[1]), np.linalg.norm(quad[2] - quad[3])))
    ch = int(max(np.linalg.norm(quad[0] - quad[3]), np.linalg.norm(quad[1] - quad[2])))
    if cw < 1 or ch < 1:
        return None
    dst = np.array([[0, 0], [cw, 0], [cw, ch], [0, ch]], dtype=np.float32)
    m = cv2.getPerspectiveTransform(quad, dst)
    crop = cv2.warpPerspective(img, m, (cw, ch), flags=cv2.INTER_CUBIC)
    if ch >= cw * 1.5:  # vertical text line — stand it up
        crop = np.rot90(crop)
    return crop


def _recognize(img_line) -> tuple[str, float]:
    import cv2
    import numpy as np

    sess = _get_sessions()["rec"]
    charset = _get_sessions()["charset"]
    h, w = img_line.shape[:2]
    rw = min(_REC_MAX_W, max(8, int(round(w * _REC_H / h))))
    resized = cv2.resize(img_line, (rw, _REC_H)).astype(np.float32)
    resized = (resized / 255.0 - 0.5) / 0.5
    padded = np.zeros((_REC_H, _REC_MAX_W, 3), dtype=np.float32)
    padded[:, :rw] = resized
    blob = padded.transpose(2, 0, 1)[np.newaxis]

    out = sess.run(None, {sess.get_inputs()[0].name: blob})[0][0]  # (T, C)
    if out.min() < 0.0 or out.max() > 1.0:  # logits — some exports skip softmax
        e = np.exp(out - out.max(axis=1, keepdims=True))
        out = e / e.sum(axis=1, keepdims=True)
    return ctc_greedy_decode(out, charset_for_classes(charset, out.shape[1]))


def run_ocr(image_bytes: bytes) -> OcrOutput:
    """PP-OCR an ID-document image into the same OcrOutput contract as
    engine.ocr.run_ocr: words with per-word confidence + line index, and
    line-preserving full text (with the Tesseract MRZ pass appended when the
    binary is available — hybrid trust-anchor reading)."""
    img = _decode_bgr(image_bytes)
    quads = _det_boxes(img)

    read: list[tuple[tuple, str, float]] = []  # (aabb, text, conf)
    for quad in quads:
        crop = _rotate_crop(img, quad)
        if crop is None:
            continue
        text, conf = _recognize(crop)
        if not text.strip() or conf < _MIN_LINE_CONF:
            continue
        aabb = (
            float(quad[:, 0].min()),
            float(quad[:, 1].min()),
            float(quad[:, 0].max()),
            float(quad[:, 1].max()),
        )
        read.append((aabb, text, conf))

    words: list[OcrWord] = []
    text_lines: list[str] = []
    for line_no, group in enumerate(cluster_lines([r[0] for r in read])):
        parts = [read[i] for i in group]
        line_text = " ".join(p[1] for p in parts)
        text_lines.append(line_text)
        for _, seg_text, seg_conf in parts:
            for token in seg_text.split():
                words.append(OcrWord(text=token, confidence=seg_conf, line=line_no))

    text = "\n".join(text_lines)

    # Hybrid MRZ pass — same charset-whitelisted read as engine.ocr, appended
    # so engine.mrz sees both candidates and keeps whichever checksums.
    from engine import ocr as tess

    if tess.tesseract_available():
        try:
            import pytesseract

            mrz_text = pytesseract.image_to_string(
                tess._preprocess(image_bytes),
                config="-c tessedit_char_whitelist=ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789< --psm 6",
            )
            text = text + "\n" + mrz_text
        except Exception:  # tesseract present but broken — PP-OCR text stands
            pass

    return OcrOutput(words=words, text=text)
