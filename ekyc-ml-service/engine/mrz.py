"""ICAO 9303 MRZ parsing with check-digit verification.

Pure stdlib — no OCR or model dependency — so it is unit-testable and can be
trusted as the authoritative source of document fields whenever an MRZ is
present and its check digits verify (OCR of the visual zone is the fallback).

Supported formats:
  TD3 — passports, 2 lines x 44 chars
  TD1 — ID cards,  3 lines x 30 chars
"""
from __future__ import annotations

import re
from dataclasses import dataclass, field
from datetime import date

# ICAO 9303 check-digit character values: digits 0-9, A-Z = 10-35, filler '<' = 0.
_WEIGHTS = (7, 3, 1)

_MRZ_CHARS = re.compile(r"^[A-Z0-9<]+$")


def char_value(c: str) -> int:
    """9303 numeric value of one MRZ character."""
    if c == "<":
        return 0
    if c.isdigit():
        return int(c)
    if "A" <= c <= "Z":
        return ord(c) - ord("A") + 10
    raise ValueError(f"invalid MRZ character: {c!r}")


def check_digit(data: str) -> int:
    """ICAO 9303 check digit: weighted sum (7,3,1 repeating) mod 10."""
    return sum(char_value(c) * _WEIGHTS[i % 3] for i, c in enumerate(data)) % 10


def verify_check_digit(data: str, digit: str) -> bool:
    """True when `digit` is the valid check digit for `data`.

    A '<' check digit is accepted only for an all-filler (absent) field, which
    9303 permits for the optional personal-number field.
    """
    if digit == "<":
        return set(data) <= {"<"}
    if not digit.isdigit():
        return False
    return check_digit(data) == int(digit)


@dataclass
class MRZResult:
    format: str  # "TD1" | "TD3"
    document_type: str
    issuing_country: str
    surname: str
    given_names: str
    document_number: str
    nationality: str
    date_of_birth: str  # ISO yyyy-mm-dd, "" if unparseable
    sex: str
    expiry_date: str  # ISO yyyy-mm-dd, "" if unparseable
    personal_number: str
    checks: dict = field(default_factory=dict)  # per-field check-digit results
    valid: bool = False  # every mandatory check digit verified

    @property
    def full_name(self) -> str:
        return " ".join(p for p in (self.given_names, self.surname) if p)


def _clean_lines(text_or_lines) -> list[str]:
    """Normalize input into candidate MRZ lines (uppercased, spaces stripped)."""
    if isinstance(text_or_lines, str):
        lines = text_or_lines.splitlines()
    else:
        lines = list(text_or_lines)
    out = []
    for ln in lines:
        ln = ln.strip().upper().replace(" ", "")
        if len(ln) >= 30 and _MRZ_CHARS.match(ln):
            out.append(ln)
    return out


def _decode_name(name_field: str) -> tuple[str, str]:
    """'ERIKSSON<<ANNA<MARIA<<<' -> ('ERIKSSON', 'ANNA MARIA')."""
    name_field = name_field.rstrip("<")
    if "<<" in name_field:
        surname, given = name_field.split("<<", 1)
    else:
        surname, given = name_field, ""
    return surname.replace("<", " ").strip(), given.replace("<", " ").strip()


def _decode_date(yymmdd: str, is_expiry: bool) -> str:
    """YYMMDD -> ISO date. Century heuristic: expiry dates map to 2000-2099;
    birth dates after the current year roll back to the 1900s."""
    if not re.match(r"^\d{6}$", yymmdd):
        return ""
    yy, mm, dd = int(yymmdd[:2]), int(yymmdd[2:4]), int(yymmdd[4:6])
    if is_expiry:
        century = 2000
    else:
        century = 2000 if yy <= date.today().year % 100 else 1900
    try:
        return date(century + yy, mm, dd).isoformat()
    except ValueError:
        return ""


def parse_td3(l1: str, l2: str) -> MRZResult:
    """Parse a 2x44 passport MRZ."""
    surname, given = _decode_name(l1[5:44])
    checks = {
        "document_number": verify_check_digit(l2[0:9], l2[9]),
        "date_of_birth": verify_check_digit(l2[13:19], l2[19]),
        "expiry_date": verify_check_digit(l2[21:27], l2[27]),
        "personal_number": verify_check_digit(l2[28:42], l2[42]),
        "composite": verify_check_digit(l2[0:10] + l2[13:20] + l2[21:43], l2[43]),
    }
    return MRZResult(
        format="TD3",
        document_type=l1[0:2].rstrip("<"),
        issuing_country=l1[2:5].rstrip("<"),
        surname=surname,
        given_names=given,
        document_number=l2[0:9].rstrip("<"),
        nationality=l2[10:13].rstrip("<"),
        date_of_birth=_decode_date(l2[13:19], is_expiry=False),
        sex=l2[20].replace("<", ""),
        expiry_date=_decode_date(l2[21:27], is_expiry=True),
        personal_number=l2[28:42].rstrip("<"),
        checks=checks,
        valid=all(checks.values()),
    )


def parse_td1(l1: str, l2: str, l3: str) -> MRZResult:
    """Parse a 3x30 ID-card MRZ."""
    surname, given = _decode_name(l3)
    checks = {
        "document_number": verify_check_digit(l1[5:14], l1[14]),
        "date_of_birth": verify_check_digit(l2[0:6], l2[6]),
        "expiry_date": verify_check_digit(l2[8:14], l2[14]),
        "composite": verify_check_digit(
            l1[5:30] + l2[0:7] + l2[8:15] + l2[18:29], l2[29]
        ),
    }
    return MRZResult(
        format="TD1",
        document_type=l1[0:2].rstrip("<"),
        issuing_country=l1[2:5].rstrip("<"),
        surname=surname,
        given_names=given,
        document_number=l1[5:14].rstrip("<"),
        nationality=l2[15:18].rstrip("<"),
        date_of_birth=_decode_date(l2[0:6], is_expiry=False),
        sex=l2[7].replace("<", ""),
        expiry_date=_decode_date(l2[8:14], is_expiry=True),
        personal_number=l1[15:30].rstrip("<"),
        checks=checks,
        valid=all(checks.values()),
    )


def parse_mrz(text_or_lines) -> MRZResult | None:
    """Find and parse an MRZ in OCR output (raw text or a line list).

    Returns None when no plausible MRZ is present. The `valid` flag on the
    result reports whether all mandatory check digits verified — callers must
    treat valid=False MRZ data as untrusted (OCR misread or tampering).
    """
    lines = _clean_lines(text_or_lines)
    # TD3: two consecutive 44-char lines, first starting with 'P'.
    for i in range(len(lines) - 1):
        a, b = lines[i], lines[i + 1]
        if len(a) == 44 and len(b) == 44 and a[0] == "P":
            return parse_td3(a, b)
    # TD1: three consecutive 30-char lines, first starting with A/C/I.
    for i in range(len(lines) - 2):
        a, b, c = lines[i], lines[i + 1], lines[i + 2]
        if (
            len(a) == 30
            and len(b) == 30
            and len(c) == 30
            and a[0] in "ACI"
        ):
            return parse_td1(a, b, c)
    return None
