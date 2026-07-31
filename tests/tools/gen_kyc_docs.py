#!/usr/bin/env python3
"""Synthetic identity-document test assets for OCR-first onboarding.

docs/nemo/07 WS-4: deterministic generator for the four documents in the
KE/ET test matrix — KE national ID, KE passport (TD3), ET Fayda national
ID, ET passport (TD3). No randomness anywhere: fixed personas, fixed
layout, fixed colours, so the rendered bytes are stable across runs and
usable as golden assets in emulator/E2E tests.

Passport MRZs are built with real ICAO 9303 check digits (7-3-1 weights,
0-9 / A-Z / '<' character values) and must round-trip through the server
parser at ekyc-ml-service/engine/mrz.py — the --self-test mode asserts
exactly that.

Usage:
    python3 tests/tools/gen_kyc_docs.py <outdir> [ke_id|ke_passport|et_fayda|et_passport|all]
    python3 tests/tools/gen_kyc_docs.py --self-test

Every document is written as both PNG (lossless golden) and JPG (what a
phone camera/gallery upload actually sends).

SYNTHETIC DOCUMENTS — test personas only, not real people or documents.
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

FONT_DIR = "/usr/share/fonts/truetype/dejavu"

# Ethiopic-capable fonts, tried in order; the Amharic header on the Fayda
# card is skipped when none is installed (the English half is what the
# on-device Latin OCR reads anyway — docs/nemo/07 §2).
ETHIOPIC_FONT_CANDIDATES = (
    "/usr/share/fonts/truetype/noto/NotoSansEthiopic-Bold.ttf",
    "/usr/share/fonts/truetype/noto/NotoSansEthiopic-Regular.ttf",
    "/usr/share/fonts/truetype/abyssinica/AbyssinicaSIL-Regular.ttf",
    "/usr/share/fonts/truetype/freefont/FreeSerif.ttf",
)

JPG_QUALITY = 92

# ---------------------------------------------------------------------------
# Fixed personas — the WS-4 deterministic test matrix
# ---------------------------------------------------------------------------
PERSONAS = {
    "ke_id": {
        "full_name": "AMINA EMUTEST",
        "id_number": "31459265",
        "dob": "12.03.1995",
    },
    "ke_passport": {
        "surname": "MWANGI",
        "given_names": "JUMA",
        "passport_no": "A1234567",
        "country": "KEN",
        "nationality": "KEN",
        "nationality_label": "KENYAN",
        "dob_iso": "1990-05-14",
        "expiry_iso": "2030-05-13",
        "sex": "M",
    },
    "et_fayda": {
        "full_name": "HANNA TESFAYE",
        "fan": "3012 4455 6677 8899",
        "dob": "02.11.1998",
        "sex": "F",
    },
    "et_passport": {
        "surname": "TESFAYE",
        "given_names": "HANNA",
        "passport_no": "EP1234567",
        "country": "ETH",
        "nationality": "ETH",
        "nationality_label": "ETHIOPIAN",
        "dob_iso": "1998-11-02",
        "expiry_iso": "2031-01-15",
        "sex": "F",
    },
}


# ---------------------------------------------------------------------------
# ICAO 9303 check digits (Part 3, §4.9): weighted sum mod 10, weights
# 7-3-1 repeating; character values 0-9 -> 0-9, A-Z -> 10-35, '<' -> 0.
# ---------------------------------------------------------------------------
_WEIGHTS = (7, 3, 1)


def mrz_char_value(c: str) -> int:
    if c == "<":
        return 0
    if c.isdigit():
        return int(c)
    if "A" <= c <= "Z":
        return ord(c) - ord("A") + 10
    raise ValueError(f"invalid MRZ character: {c!r}")


def mrz_check_digit(data: str) -> str:
    return str(sum(mrz_char_value(c) * _WEIGHTS[i % 3] for i, c in enumerate(data)) % 10)


def _yymmdd(iso: str) -> str:
    y, m, d = iso.split("-")
    return y[2:] + m + d


def td3_lines(p: dict) -> tuple[str, str]:
    """Build the two 44-char TD3 MRZ lines with valid check digits."""
    name = (p["surname"] + "<<" + p["given_names"]).replace(" ", "<")
    l1 = ("P<" + p["country"] + name).ljust(44, "<")[:44]

    doc = p["passport_no"].ljust(9, "<")
    dob = _yymmdd(p["dob_iso"])
    exp = _yymmdd(p["expiry_iso"])
    personal = "".ljust(14, "<")
    l2 = (
        doc + mrz_check_digit(doc)
        + p["nationality"]
        + dob + mrz_check_digit(dob)
        + p["sex"]
        + exp + mrz_check_digit(exp)
        + personal + mrz_check_digit(personal)
    )
    l2 += mrz_check_digit(l2[0:10] + l2[13:20] + l2[21:43])
    assert len(l1) == 44 and len(l2) == 44
    return l1, l2


# ---------------------------------------------------------------------------
# Drawing helpers
# ---------------------------------------------------------------------------
def _font(name: str, size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(os.path.join(FONT_DIR, name), size)


def _ethiopic_font(size: int) -> ImageFont.FreeTypeFont | None:
    for path in ETHIOPIC_FONT_CANDIDATES:
        if os.path.exists(path):
            return ImageFont.truetype(path, size)
    return None


def _centered(draw: ImageDraw.ImageDraw, y: int, text: str, font, fill, width: int) -> None:
    w = draw.textlength(text, font=font)
    draw.text(((width - w) / 2, y), text, font=font, fill=fill)


def _photo_box(draw: ImageDraw.ImageDraw, box: tuple[int, int, int, int]) -> None:
    """Neutral portrait placeholder — clearly synthetic, no face."""
    x0, y0, x1, y1 = box
    draw.rectangle(box, fill=(210, 214, 218), outline=(120, 126, 132), width=3)
    cx, cy = (x0 + x1) // 2, (y0 + y1) // 2
    r = (x1 - x0) // 6
    draw.ellipse((cx - r, y0 + (y1 - y0) // 5, cx + r, y0 + (y1 - y0) // 5 + 2 * r),
                 fill=(160, 166, 172))
    draw.pieslice((x0 + (x1 - x0) // 6, cy, x1 - (x1 - x0) // 6, y1 + (y1 - y0) // 2),
                  180, 360, fill=(160, 166, 172))
    draw.rectangle(box, outline=(120, 126, 132), width=3)


def _save(img: Image.Image, outdir: Path, stem: str) -> list[Path]:
    outdir.mkdir(parents=True, exist_ok=True)
    png = outdir / f"{stem}.png"
    jpg = outdir / f"{stem}.jpg"
    img.save(png, "PNG")
    img.convert("RGB").save(jpg, "JPEG", quality=JPG_QUALITY)
    return [png, jpg]


def _passport_page(outdir: Path, stem: str, p: dict, header_lines: list[str],
                   page_tint: tuple[int, int, int]) -> list[Path]:
    """Shared TD3 passport data-page layout (KE and ET differ only in
    persona + tint + header)."""
    W, H = 1400, 900
    img = Image.new("RGB", (W, H), page_tint)
    d = ImageDraw.Draw(img)

    f_header = _font("DejaVuSans-Bold.ttf", 34)
    f_sub = _font("DejaVuSans-Bold.ttf", 26)
    f_label = _font("DejaVuSans.ttf", 20)
    f_value = _font("DejaVuSans-Bold.ttf", 30)
    f_mrz = _font("DejaVuSansMono-Bold.ttf", 44)

    ink = (25, 30, 60)
    label_ink = (90, 96, 120)

    _centered(d, 28, header_lines[0], f_header, ink, W)
    _centered(d, 74, header_lines[1], f_sub, ink, W)
    d.line((60, 120, W - 60, 120), fill=label_ink, width=2)

    d.text((60, 132), "TYPE / P", font=f_label, fill=label_ink)
    d.text((300, 132), "COUNTRY CODE / " + p["country"], font=f_label, fill=label_ink)

    _photo_box(d, (60, 180, 360, 560))

    def field(x: int, y: int, label: str, value: str) -> None:
        d.text((x, y), label, font=f_label, fill=label_ink)
        d.text((x, y + 28), value, font=f_value, fill=ink)

    lx, rx = 420, 900
    field(lx, 190, "SURNAME", p["surname"])
    field(lx, 290, "GIVEN NAMES", p["given_names"])
    field(lx, 390, "NATIONALITY", p["nationality_label"])
    field(lx, 490, "DATE OF BIRTH", p["dob_iso"].replace("-", " "))
    field(rx, 190, "PASSPORT NO", p["passport_no"])
    field(rx, 290, "SEX", p["sex"])
    field(rx, 390, "DATE OF EXPIRY", p["expiry_iso"].replace("-", " "))
    field(rx, 490, "PLACE OF ISSUE", "SYNTHETIC / TEST")

    d.text((60, 600), "SPECIMEN — SYNTHETIC TEST DOCUMENT", font=f_label, fill=(150, 60, 60))

    # MRZ strip: white band, two 44-char monospaced lines, valid check digits.
    l1, l2 = td3_lines(p)
    d.rectangle((0, 660, W, H), fill=(255, 255, 255))
    d.text((44, 706), l1, font=f_mrz, fill=(10, 10, 10))
    d.text((44, 786), l2, font=f_mrz, fill=(10, 10, 10))

    return _save(img, outdir, stem)


# ---------------------------------------------------------------------------
# Document generators
# ---------------------------------------------------------------------------
def ke_national_id(outdir: Path) -> list[Path]:
    """KE national ID front card — the layout proven in the 2026-07-31
    emulator run: green header band, English labels."""
    p = PERSONAS["ke_id"]
    W, H = 1000, 630
    img = Image.new("RGB", (W, H), (246, 244, 236))
    d = ImageDraw.Draw(img)

    green = (0, 92, 49)
    d.rectangle((0, 0, W, 130), fill=green)
    _centered(d, 18, "REPUBLIC OF KENYA", _font("DejaVuSans-Bold.ttf", 40), (255, 255, 255), W)
    _centered(d, 76, "NATIONAL IDENTITY CARD", _font("DejaVuSans-Bold.ttf", 30), (255, 255, 255), W)

    _photo_box(d, (60, 190, 320, 520))

    f_label = _font("DejaVuSans-Bold.ttf", 24)
    f_value = _font("DejaVuSans-Bold.ttf", 36)

    def field(y: int, label: str, value: str) -> None:
        d.text((380, y), label, font=f_label, fill=green)
        d.text((380, y + 34), value, font=f_value, fill=(20, 20, 20))

    field(190, "FULL NAME", p["full_name"])
    field(310, "ID NUMBER", p["id_number"])
    field(430, "DATE OF BIRTH", p["dob"])

    d.text((60, 560), "SPECIMEN — SYNTHETIC TEST DOCUMENT",
           font=_font("DejaVuSans.ttf", 20), fill=(150, 60, 60))
    return _save(img, outdir, "ke_national_id")


def ke_passport(outdir: Path) -> list[Path]:
    return _passport_page(
        outdir, "ke_passport", PERSONAS["ke_passport"],
        ["REPUBLIC OF KENYA", "PASSPORT / PASSEPORT"],
        (222, 230, 242),
    )


def et_fayda(outdir: Path) -> list[Path]:
    """ET Fayda national ID — bilingual card. Amharic header rendered when
    an Ethiopic font exists; the English half carries the OCR targets."""
    p = PERSONAS["et_fayda"]
    W, H = 1000, 630
    img = Image.new("RGB", (W, H), (248, 248, 246))
    d = ImageDraw.Draw(img)

    teal = (0, 105, 92)
    d.rectangle((0, 0, W, 140), fill=(255, 255, 255))
    d.rectangle((0, 140, W, 148), fill=(60, 145, 70))
    d.rectangle((0, 148, W, 154), fill=(220, 175, 45))

    am = _ethiopic_font(34)
    if am is not None:
        _centered(d, 14, "የኢትዮጵያ ብሔራዊ መታወቂያ", am, teal, W)
        _centered(d, 88, "ETHIOPIAN NATIONAL ID", _font("DejaVuSans-Bold.ttf", 30), teal, W)
    else:
        _centered(d, 24, "ETHIOPIAN NATIONAL ID", _font("DejaVuSans-Bold.ttf", 38), teal, W)

    _photo_box(d, (60, 200, 300, 510))

    f_label = _font("DejaVuSans-Bold.ttf", 22)
    f_value = _font("DejaVuSans-Bold.ttf", 32)

    def field(x: int, y: int, label: str, value: str) -> None:
        d.text((x, y), label, font=f_label, fill=teal)
        d.text((x, y + 30), value, font=f_value, fill=(20, 20, 20))

    field(360, 200, "Full Name", p["full_name"])
    field(360, 300, "FAN", p["fan"])
    field(360, 400, "Date of Birth", p["dob"])
    field(760, 400, "Sex", p["sex"])

    d.text((60, 560), "SPECIMEN — SYNTHETIC TEST DOCUMENT",
           font=_font("DejaVuSans.ttf", 20), fill=(150, 60, 60))
    return _save(img, outdir, "et_fayda")


def et_passport(outdir: Path) -> list[Path]:
    return _passport_page(
        outdir, "et_passport", PERSONAS["et_passport"],
        ["FEDERAL DEMOCRATIC REPUBLIC OF ETHIOPIA", "PASSPORT"],
        (224, 238, 228),
    )


GENERATORS = {
    "ke_id": ke_national_id,
    "ke_passport": ke_passport,
    "et_fayda": et_fayda,
    "et_passport": et_passport,
}


# ---------------------------------------------------------------------------
# Self-test: files render, and the MRZs round-trip through the server's
# authoritative ICAO 9303 parser (ekyc-ml-service/engine/mrz.py).
# ---------------------------------------------------------------------------
def self_test() -> None:
    import tempfile

    repo_root = Path(__file__).resolve().parents[2]
    sys.path.insert(0, str(repo_root / "ekyc-ml-service"))
    from engine.mrz import parse_mrz  # noqa: E402

    with tempfile.TemporaryDirectory(prefix="kyc-docs-selftest-") as td:
        outdir = Path(td)
        produced: list[Path] = []
        for name, gen in GENERATORS.items():
            paths = gen(outdir)
            for path in paths:
                assert path.exists() and path.stat().st_size > 0, f"{name}: {path} missing/empty"
            produced += paths
        assert len(produced) == 2 * len(GENERATORS)

        for key in ("ke_passport", "et_passport"):
            p = PERSONAS[key]
            l1, l2 = td3_lines(p)
            r = parse_mrz(l1 + "\n" + l2)
            assert r is not None, f"{key}: parse_mrz found no MRZ"
            assert r.valid, f"{key}: check digits failed: {r.checks}"
            assert all(r.checks.values()), f"{key}: {r.checks}"
            assert r.format == "TD3"
            assert r.document_number == p["passport_no"]
            assert r.surname == p["surname"]
            assert r.given_names == p["given_names"]
            assert r.nationality == p["nationality"]
            assert r.date_of_birth == p["dob_iso"]
            assert r.expiry_date == p["expiry_iso"]
            assert r.sex == p["sex"]
            print(f"  {key}: MRZ round-trip OK — {r.full_name}, "
                  f"{r.document_number}, checks={r.checks}")

        # Determinism spot-check: a second render is byte-identical.
        second = Path(td) / "again"
        for name, gen in GENERATORS.items():
            for a, b in zip(sorted(gen(second)), sorted(
                    p for p in produced if p.stem == b_stem(name))):
                assert a.read_bytes() == b.read_bytes(), f"{name}: non-deterministic render"

    print(f"self-test OK: {len(GENERATORS)} documents x 2 formats, "
          "MRZ check digits verified against engine/mrz.py")


def b_stem(name: str) -> str:
    return {"ke_id": "ke_national_id"}.get(name, name)


def main(argv: list[str]) -> int:
    if len(argv) >= 2 and argv[1] == "--self-test":
        self_test()
        return 0
    if len(argv) < 2:
        print(__doc__)
        return 2
    outdir = Path(argv[1])
    which = argv[2] if len(argv) > 2 else "all"
    if which != "all" and which not in GENERATORS:
        print(f"unknown document {which!r}; choose from {', '.join(GENERATORS)} or all")
        return 2
    names = list(GENERATORS) if which == "all" else [which]
    for name in names:
        for path in GENERATORS[name](outdir):
            print(path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
