"""Field extraction post-processing over raw OCR output.

Pure logic (no pytesseract/OpenCV import) so it is fully unit-testable: the
OCR layer hands over a list of OcrWord (text, confidence, line number) and
this module turns it into typed, per-field-confidence document fields.

Strategy: label-anchored extraction. ID documents (Kenyan national ID, most
East-African cards, passports' visual zone) print field labels — we find a
known label on a line and read the value from the remainder of that line or
the next line. Regex fallbacks cover unlabelled layouts. When an MRZ parses
with valid check digits the caller overrides these fields with the MRZ values
(confidence 0.99) — the MRZ is checksummed, the visual zone is not.
"""
from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import date, datetime


@dataclass
class OcrWord:
    text: str
    confidence: float  # 0..1
    line: int  # line index within the page


@dataclass
class FieldValue:
    value: str
    confidence: float  # 0..1

    def as_dict(self) -> dict:
        return {"value": self.value, "confidence": round(self.confidence, 4)}


# Label variants seen on East-African / common ID documents (uppercased,
# punctuation-stripped comparison).
NAME_LABELS = ("FULL NAMES", "FULL NAME", "NAMES", "NAME", "SURNAME")
ID_LABELS = (
    "ID NUMBER", "ID NO", "IDENTITY NUMBER", "NATIONAL ID",
    "DOCUMENT NUMBER", "DOCUMENT NO", "PASSPORT NUMBER", "PASSPORT NO",
    "SERIAL NUMBER", "SERIAL NO",
)
DOB_LABELS = ("DATE OF BIRTH", "BIRTH DATE", "DOB", "BORN")

# Words that are labels/boilerplate, never part of a value.
_NOISE_WORDS = {
    "REPUBLIC", "KENYA", "UGANDA", "TANZANIA", "JAMHURI", "YA",
    "IDENTITY", "CARD", "NATIONAL", "PASSPORT", "HOLDER", "SIGNATURE",
    "SEX", "MALE", "FEMALE", "DISTRICT", "DIVISION", "LOCATION",
    "PLACE", "ISSUE", "ISSUED", "EXPIRY", "SPECIMEN",
}

_DATE_FORMATS = (
    "%d.%m.%Y", "%d.%m.%y", "%d/%m/%Y", "%d/%m/%y", "%d-%m-%Y",
    "%Y-%m-%d", "%d %b %Y", "%d %B %Y", "%b %d %Y", "%B %d %Y",
)

_DATE_TOKEN = re.compile(
    r"\b(\d{1,2}[./-]\d{1,2}[./-]\d{2,4}"
    r"|\d{4}-\d{2}-\d{2}"
    r"|\d{1,2}\s+[A-Za-z]{3,9}\.?\s+\d{4})\b"
)
# 6-10 char numeric or letter-prefixed alphanumeric document numbers.
_DOCNUM_TOKEN = re.compile(r"\b([A-Z]{0,2}\d{6,10})\b")


def parse_date_any(s: str) -> str:
    """Parse a printed date in common ID formats -> ISO yyyy-mm-dd ('' if not
    a date). Two-digit years follow the MRZ birth-date heuristic."""
    s = s.strip().rstrip(".,").replace("‐", "-")
    s = re.sub(r"\s+", " ", s)
    s = re.sub(r"(\d)(st|nd|rd|th)\b", r"\1", s, flags=re.I)
    for fmt in _DATE_FORMATS:
        try:
            d = datetime.strptime(s, fmt).date()
        except ValueError:
            continue
        if "%y" in fmt and d.year - 2000 > date.today().year % 100:
            d = d.replace(year=d.year - 100)
        return d.isoformat()
    return ""


def clean_name(s: str) -> str:
    """Strip OCR noise from a name value: keep letters, spaces, hyphens and
    apostrophes; collapse whitespace; drop 1-char fragments."""
    s = re.sub(r"[^A-Za-z' \-]", " ", s)
    parts = [p for p in s.split() if len(p) > 1 or p.upper() == "O"]
    return " ".join(parts).strip()


def normalize_docnum(s: str) -> str:
    """Uppercase alphanumerics only — for comparing document numbers."""
    return re.sub(r"[^A-Z0-9]", "", s.upper())


def _canon(s: str) -> str:
    return re.sub(r"[^A-Z ]", "", s.upper()).strip()


def _lines(words: list[OcrWord]) -> dict[int, list[OcrWord]]:
    lines: dict[int, list[OcrWord]] = {}
    for w in words:
        if w.text.strip():
            lines.setdefault(w.line, []).append(w)
    return lines


def _line_text(ws: list[OcrWord]) -> str:
    return " ".join(w.text for w in ws)


def _mean_conf(ws: list[OcrWord]) -> float:
    return sum(w.confidence for w in ws) / len(ws) if ws else 0.0


def _find_labelled(
    lines: dict[int, list[OcrWord]], labels: tuple[str, ...]
) -> tuple[str, float] | None:
    """Find a label's tokens on a line; value = the words after the label on
    that line, else the whole next line."""
    order = sorted(lines)
    for label in labels:  # label priority: most specific first
        ltoks = label.split()
        for idx, ln in enumerate(order):
            ws = lines[ln]
            ctoks = [_canon(w.text) for w in ws]
            end = _match_token_seq(ctoks, ltoks)
            if end < 0:
                continue
            after = [w for w in ws[end:] if _canon(w.text) or w.text.strip()]
            after = [w for w in after if w.text.strip(":.- ")]
            if after:
                return _line_text(after), _mean_conf(after)
            if idx + 1 < len(order):
                nxt = lines[order[idx + 1]]
                return _line_text(nxt), _mean_conf(nxt)
    return None


def _match_token_seq(tokens: list[str], label_tokens: list[str]) -> int:
    """Index just past the first occurrence of label_tokens within tokens
    (canonical comparison), or -1. Tolerates label tokens fused into one OCR
    token ('IDNUMBER')."""
    fused = "".join(label_tokens)
    for i, t in enumerate(tokens):
        if t == fused or t.replace(" ", "") == fused:
            return i + 1
        if t == label_tokens[0] and len(label_tokens) > 1:
            j, k = i + 1, 1
            while j < len(tokens) and k < len(label_tokens) and tokens[j] == label_tokens[k]:
                j, k = j + 1, k + 1
            if k == len(label_tokens):
                return j
    return -1


def _fallback_name(lines: dict[int, list[OcrWord]]) -> tuple[str, float] | None:
    """Unlabelled layout: the longest multi-word all-letter line that is not
    boilerplate. Confidence is discounted — this is a guess, not an anchor."""
    best: tuple[str, float] | None = None
    for ws in lines.values():
        text = _line_text(ws)
        cleaned = clean_name(text)
        tokens = cleaned.upper().split()
        if len(tokens) < 2 or any(t in _NOISE_WORDS for t in tokens):
            continue
        if not re.fullmatch(r"[A-Za-z' \-]+", text.strip()):
            continue
        if best is None or len(cleaned) > len(best[0]):
            best = (cleaned, _mean_conf(ws) * 0.7)
    return best


def extract_fields(words: list[OcrWord]) -> dict[str, FieldValue]:
    """Extract fullName / documentNumber / dateOfBirth from OCR words.

    Per-field confidence = mean OCR confidence of the contributing words,
    discounted when the value came from a positional fallback rather than a
    printed label. Missing fields are simply absent from the dict.
    """
    lines = _lines(words)
    out: dict[str, FieldValue] = {}

    # Full name
    hit = _find_labelled(lines, NAME_LABELS) or _fallback_name(lines)
    if hit:
        value = clean_name(hit[0])
        if value:
            out["fullName"] = FieldValue(value.upper(), hit[1])

    # Document number: labelled, else first plausible token
    hit = _find_labelled(lines, ID_LABELS)
    if hit:
        m = _DOCNUM_TOKEN.search(normalize_docnum_spaced(hit[0]))
        if m:
            out["documentNumber"] = FieldValue(m.group(1), hit[1])
    if "documentNumber" not in out:
        for ws in lines.values():
            m = _DOCNUM_TOKEN.search(normalize_docnum_spaced(_line_text(ws)))
            if m:
                out["documentNumber"] = FieldValue(m.group(1), _mean_conf(ws) * 0.8)
                break

    # Date of birth: labelled, else first date token anywhere (discounted —
    # could be issue/expiry on unlabelled layouts).
    hit = _find_labelled(lines, DOB_LABELS)
    if hit:
        m = _DATE_TOKEN.search(hit[0])
        iso = parse_date_any(m.group(1)) if m else parse_date_any(hit[0])
        if iso:
            out["dateOfBirth"] = FieldValue(iso, hit[1])
    if "dateOfBirth" not in out:
        for ws in lines.values():
            m = _DATE_TOKEN.search(_line_text(ws))
            if m:
                iso = parse_date_any(m.group(1))
                if iso:
                    out["dateOfBirth"] = FieldValue(iso, _mean_conf(ws) * 0.6)
                    break

    return out


def normalize_docnum_spaced(s: str) -> str:
    """Uppercase and drop spaces/dots inside digit groups so '123 456 78'
    or '12.345.678' matches the document-number token regex."""
    s = s.upper()
    s = re.sub(r"(?<=\d)[ .](?=\d)", "", s)
    return s


def merge_mrz(fields: dict[str, FieldValue], mrz) -> dict[str, FieldValue]:
    """Override visual-zone fields with checksummed MRZ values.

    Only applied when the MRZ check digits all verified (mrz.valid) — a valid
    MRZ is machine-verifiable ground truth, so its fields carry 0.99.
    """
    if mrz is None or not mrz.valid:
        return fields
    merged = dict(fields)
    if mrz.full_name.strip():
        merged["fullName"] = FieldValue(clean_name(mrz.full_name).upper(), 0.99)
    if mrz.document_number:
        merged["documentNumber"] = FieldValue(mrz.document_number, 0.99)
    if mrz.date_of_birth:
        merged["dateOfBirth"] = FieldValue(mrz.date_of_birth, 0.99)
    return merged
