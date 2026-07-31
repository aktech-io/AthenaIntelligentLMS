"""Per-document extraction profiles over raw OCR output.

Pure logic (no pytesseract/OpenCV import), same testability contract as
engine.fields: the OCR layer hands over words + full text and a profile id,
and this module dispatches to the matching extraction strategy:

  ke-national-id  — alias for the default label-anchored extraction with
                    MRZ override (the pre-profile behaviour, now explicit).
  passport-mrz    — MRZ is PRIMARY (TD3 first, TD1 fallback). When check
                    digits verify, the MRZ values ARE the result fields at
                    0.99; the visual zone only fills fields the MRZ lacks.
                    No valid MRZ -> low-confidence result, never a silent
                    fall back to label extraction: the caller's declared-
                    value agreement must fail and refer to an officer.
  et-fayda        — label-anchored extraction against the English half of
                    the bilingual Fayda card (Full Name / FAN / Date of
                    Birth / Sex). documentNumber must be the 16-digit FAN.

Unknown or missing profile ids fall through to the default so existing
callers are untouched.
"""
from __future__ import annotations

import re
from dataclasses import dataclass, field

from engine.fields import (
    FieldValue,
    OcrWord,
    _DATE_TOKEN,
    _find_labelled,
    _lines,
    _line_text,
    _mean_conf,
    clean_name,
    extract_fields,
    merge_mrz,
    parse_date_any,
)
from engine.mrz import MRZResult, parse_mrz

# Confidence assigned to MRZ-derived values whose check digits did NOT
# verify: kept for officer review, low enough to fail any extraction floor.
_MRZ_INVALID_CONF = 0.30

# Fayda (ET national ID) English-half label variants.
FAYDA_NAME_LABELS = ("FULL NAME", "NAME")
FAYDA_FAN_LABELS = ("FAN",)
FAYDA_DOB_LABELS = ("DATE OF BIRTH", "DOB")
FAYDA_SEX_LABELS = ("SEX",)

# FAN as printed: four spaced/dashed groups of 4 digits (or 16 contiguous).
_FAN_GROUPS = re.compile(r"\b\d{4}[ \-]\d{4}[ \-]\d{4}[ \-]\d{4}\b|\b\d{16}\b")
_FAN_16 = re.compile(r"^[0-9]{16}$")


@dataclass
class ExtractionResult:
    fields: dict[str, FieldValue] = field(default_factory=dict)
    mrz: MRZResult | None = None
    document_type_detected: str = ""  # e.g. "P" from a passport MRZ


def normalize_fan(s: str) -> str:
    """Drop the spaces/dashes Fayda cards print between FAN digit groups."""
    return re.sub(r"[ \-]", "", s.strip())


def _extract_default(words: list[OcrWord], text: str) -> ExtractionResult:
    """Pre-profile behaviour: label-anchored extraction, MRZ override."""
    mrz = parse_mrz(text)
    return ExtractionResult(fields=merge_mrz(extract_fields(words), mrz), mrz=mrz)


def _extract_passport_mrz(words: list[OcrWord], text: str) -> ExtractionResult:
    mrz = parse_mrz(text)

    if mrz is not None and mrz.valid:
        fields: dict[str, FieldValue] = {}
        if mrz.full_name.strip():
            fields["fullName"] = FieldValue(clean_name(mrz.full_name).upper(), 0.99)
        if mrz.document_number:
            fields["documentNumber"] = FieldValue(mrz.document_number, 0.99)
        if mrz.date_of_birth:
            fields["dateOfBirth"] = FieldValue(mrz.date_of_birth, 0.99)
        if mrz.expiry_date:
            fields["expiryDate"] = FieldValue(mrz.expiry_date, 0.99)
        if mrz.nationality:
            fields["nationality"] = FieldValue(mrz.nationality, 0.99)
        # Visual zone only fills fields the checksummed MRZ lacks.
        for k, v in extract_fields(words).items():
            fields.setdefault(k, v)
        return ExtractionResult(fields, mrz, mrz.document_type)

    # No trustworthy MRZ on a passport profile: low-confidence result, and
    # deliberately NO fall back to label extraction — the Go side's
    # declared-value agreement must fail so the case refers to an officer.
    fields = {}
    if mrz is not None:  # parsed but check digits failed: keep for review
        for key, value in (
            ("fullName", clean_name(mrz.full_name).upper()),
            ("documentNumber", mrz.document_number),
            ("dateOfBirth", mrz.date_of_birth),
            ("expiryDate", mrz.expiry_date),
            ("nationality", mrz.nationality),
        ):
            if value:
                fields[key] = FieldValue(value, _MRZ_INVALID_CONF)
    return ExtractionResult(
        fields, mrz, mrz.document_type if mrz is not None else ""
    )


def _extract_et_fayda(words: list[OcrWord], text: str) -> ExtractionResult:
    lines = _lines(words)
    out: dict[str, FieldValue] = {}

    # Full name (anchored only — the unanchored longest-line guess of the
    # default profile would happily pick a transliterated Amharic line).
    hit = _find_labelled(lines, FAYDA_NAME_LABELS)
    if hit:
        value = clean_name(hit[0])
        if value:
            out["fullName"] = FieldValue(value.upper(), hit[1])

    # FAN: labelled first, else the distinctive 4x4 digit-group pattern on
    # any line (discounted). Anything that does not normalize to exactly
    # 16 digits is rejected outright.
    hit = _find_labelled(lines, FAYDA_FAN_LABELS)
    if hit:
        m = _FAN_GROUPS.search(hit[0])
        fan = normalize_fan(m.group(0)) if m else normalize_fan(hit[0])
        if _FAN_16.fullmatch(fan):
            out["documentNumber"] = FieldValue(fan, hit[1])
    if "documentNumber" not in out:
        for ws in lines.values():
            m = _FAN_GROUPS.search(_line_text(ws))
            if m:
                fan = normalize_fan(m.group(0))
                if _FAN_16.fullmatch(fan):
                    out["documentNumber"] = FieldValue(fan, _mean_conf(ws) * 0.8)
                    break

    # Date of birth (anchored only).
    hit = _find_labelled(lines, FAYDA_DOB_LABELS)
    if hit:
        m = _DATE_TOKEN.search(hit[0])
        iso = parse_date_any(m.group(1)) if m else parse_date_any(hit[0])
        if iso:
            out["dateOfBirth"] = FieldValue(iso, hit[1])

    # Sex.
    hit = _find_labelled(lines, FAYDA_SEX_LABELS)
    if hit:
        tok = hit[0].split()[0].upper().strip(":.")
        if tok in ("M", "MALE"):
            out["sex"] = FieldValue("M", hit[1])
        elif tok in ("F", "FEMALE"):
            out["sex"] = FieldValue("F", hit[1])

    return ExtractionResult(fields=out, mrz=None)


_PROFILES = {
    "ke-national-id": _extract_default,  # explicit alias for the default
    "passport-mrz": _extract_passport_mrz,
    "et-fayda": _extract_et_fayda,
}


def extract_with_profile(
    words: list[OcrWord], text: str, profile: str | None = None
) -> ExtractionResult:
    """Dispatch extraction by profile id.

    Missing or unknown profile ids use the default (pre-profile) strategy,
    so the endpoint stays backwards compatible for callers that send only
    the file part.
    """
    fn = _PROFILES.get((profile or "").strip().lower(), _extract_default)
    return fn(words, text)
