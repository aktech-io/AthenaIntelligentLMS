#!/usr/bin/env python3
"""Sync real sanctions lists into the ekyc-ml-service CSV shape.

Downloads the public consolidated sanctions sources and converts them to the
`id,name,aliases,country,program` CSV shape that `engine/screening.py` loads
(aliases ';'-separated, header row required):

    sanctions_ofac.csv   OFAC SDN + OFAC (non-SDN) Consolidated
    sanctions_un.csv     UN Security Council Consolidated List (XML)
    sanctions_eu.csv     EU Financial Sanctions Files (XML; the download URL
                         embeds an operator token, so it is supplied via
                         --eu-url and the file is skipped when absent)

Robustness contract (mirrors the engine's fail-closed philosophy):

    - Each output is written to a temp file in the target directory and
      atomically renamed only after it parsed and cleared --min-rows, so a
      failed/truncated download can NEVER clobber a good previous file.
    - Any configured source failing -> the others still sync, previous files
      stay in place, and the exit code is nonzero so cron/alerting notices.
    - --verify performs no downloads: it just checks the output files exist,
      are non-trivially populated and are fresher than --max-age-hours.

Stdlib only (urllib, csv, xml.etree) — runnable from any ops box or cron
container with a bare Python 3.10+. See ekyc-ml-service/README.md
("Screening lists") for usage and a cron example. PEP lists are a separate
concern (commercial feed or curated national lists — no free consolidated
source exists to automate here).
"""
from __future__ import annotations

import argparse
import csv
import os
import re
import sys
import tempfile
import time
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

# OFAC publishes the same legacy-shape CSVs at two hosts; the API-gateway
# host rejects some client networks (403 ForbiddenException), so each file
# has an ordered fallback list — first URL that downloads wins.
OFAC_SDN_URLS = [
    "https://sanctionslist.ofac.treas.gov/api/download/sdn.csv",
    "https://sanctionslistservice.ofac.treas.gov/api/PublicationPreview/exports/SDN.CSV",
]
OFAC_CONS_URLS = [
    "https://sanctionslist.ofac.treas.gov/api/download/cons_prim.csv",
    "https://sanctionslistservice.ofac.treas.gov/api/PublicationPreview/exports/CONS_PRIM.CSV",
]
UN_URL = "https://scsanctions.un.org/resources/xml/en/consolidated.xml"

# Some of these hosts reject Python's default urllib User-Agent.
USER_AGENT = "nemo-ekyc-list-sync/1.0 (ekyc-ml-service screening refresh)"
DOWNLOAD_TIMEOUT = 180  # seconds; the UN XML is several MB

CSV_HEADER = ["id", "name", "aliases", "country", "program"]

# Legacy OFAC CSVs use "-0-" as their null marker.
_OFAC_NULL = "-0-"
# a.k.a. names embedded in the OFAC remarks column: a.k.a. 'SOME NAME'.
_AKA_RE = re.compile(r"a\.k\.a\.\s+'([^']+)'")


class Row:
    """One screening-list entry in the engine's CSV shape."""

    __slots__ = ("entry_id", "name", "aliases", "country", "program")

    def __init__(self, entry_id, name, aliases=(), country="", program=""):
        self.entry_id = entry_id
        self.name = name
        self.aliases = [a for a in aliases if a]
        self.country = country
        self.program = program

    def as_csv(self) -> list[str]:
        return [
            self.entry_id,
            self.name,
            ";".join(self.aliases),
            self.country,
            self.program,
        ]


# ─── parsers (pure — unit-tested with inline fixtures) ───────────────────────


def parse_ofac_csv(text: str, id_prefix: str) -> list[Row]:
    """Parse an OFAC legacy-shape CSV (sdn.csv / cons_prim.csv).

    Columns (positional, historically headerless — a header row containing
    "ent_num" is detected and skipped): ent_num, name, type, program, title,
    call_sign, vess_type, tonnage, grt, vess_flag, vess_owner, remarks.
    Nulls are "-0-". Aliases are recovered from ``a.k.a. '...'`` markers in
    the remarks column (the full ALT file is address/alias detail we don't
    need row-level fidelity on for fuzzy name screening).
    """
    rows: list[Row] = []
    for record in csv.reader(text.splitlines()):
        if len(record) < 4:
            continue
        ent_num = record[0].strip()
        if not ent_num or "ent_num" in ent_num.lower():
            continue  # header row (newer API exports include one)
        name = record[1].strip()
        if not name or name == _OFAC_NULL:
            continue
        program = record[3].strip()
        remarks = record[11].strip() if len(record) > 11 else ""
        aliases = _AKA_RE.findall(remarks) if remarks != _OFAC_NULL else []
        rows.append(
            Row(
                entry_id=f"{id_prefix}-{ent_num}",
                name=name,
                aliases=aliases,
                country="",  # addresses live in separate OFAC files
                program="" if program == _OFAC_NULL else program,
            )
        )
    return rows


def _local(tag: str) -> str:
    """Local element name with any XML namespace stripped."""
    return tag.rsplit("}", 1)[-1]


def _child_text(elem: ET.Element, local_name: str) -> str:
    for child in elem:
        if _local(child.tag) == local_name:
            return (child.text or "").strip()
    return ""


def parse_un_xml(data: bytes) -> list[Row]:
    """Parse the UN Security Council consolidated list XML.

    INDIVIDUAL entries carry FIRST/SECOND/THIRD/FOURTH_NAME parts,
    INDIVIDUAL_ALIAS/ALIAS_NAME aliases, NATIONALITY/VALUE and UN_LIST_TYPE;
    ENTITY entries the same minus the name parts (FIRST_NAME is the full
    name) with ENTITY_ALIAS. The REFERENCE_NUMBER (e.g. QDi.400) is the
    stable public id; DATAID is the fallback.
    """
    root = ET.fromstring(data)
    rows: list[Row] = []
    for elem in root.iter():
        kind = _local(elem.tag)
        if kind not in ("INDIVIDUAL", "ENTITY"):
            continue
        # Direct children only — INDIVIDUAL_ALIAS elements nest ALIAS_NAME.
        parts = [
            _child_text(elem, p)
            for p in ("FIRST_NAME", "SECOND_NAME", "THIRD_NAME", "FOURTH_NAME")
        ]
        name = " ".join(p for p in parts if p)
        if not name:
            continue
        aliases = []
        countries = []
        for child in elem:
            local = _local(child.tag)
            if local in ("INDIVIDUAL_ALIAS", "ENTITY_ALIAS"):
                alias = _child_text(child, "ALIAS_NAME")
                if alias:
                    aliases.append(alias)
            elif local == "NATIONALITY":
                value = _child_text(child, "VALUE")
                if value:
                    countries.append(value)
        ref = _child_text(elem, "REFERENCE_NUMBER") or _child_text(
            elem, "DATAID"
        )
        rows.append(
            Row(
                entry_id=f"UN-{ref}" if ref else "UN",
                name=name,
                aliases=aliases,
                country="; ".join(countries),
                program=_child_text(elem, "UN_LIST_TYPE"),
            )
        )
    return rows


def parse_eu_xml(data: bytes) -> list[Row]:
    """Parse the EU Financial Sanctions Files (FSF) XML export.

    Namespace-agnostic (the export uses http://eu.europa.ec/fpi/fsd/export):
    each sanctionEntity carries nameAlias children (attribute ``wholeName``
    — the first is taken as the primary name, the rest as aliases),
    citizenship/countryDescription, and regulation/@programme.
    """
    root = ET.fromstring(data)
    rows: list[Row] = []
    for elem in root.iter():
        if _local(elem.tag) != "sanctionEntity":
            continue
        names = []
        countries = []
        program = ""
        for child in elem:
            local = _local(child.tag)
            if local == "nameAlias":
                whole = (child.get("wholeName") or "").strip()
                if whole:
                    names.append(whole)
            elif local == "citizenship":
                desc = (child.get("countryDescription") or "").strip()
                if desc and desc.upper() != "UNKNOWN":
                    countries.append(desc)
            elif local == "regulation" and not program:
                program = (child.get("programme") or "").strip()
        if not names:
            continue
        entity_id = (
            elem.get("logicalId") or elem.get("euReferenceNumber") or ""
        ).strip()
        rows.append(
            Row(
                entry_id=f"EU-{entity_id}" if entity_id else "EU",
                name=names[0],
                aliases=names[1:],
                country="; ".join(dict.fromkeys(countries)),
                program=program,
            )
        )
    return rows


# ─── download + atomic write ─────────────────────────────────────────────────


def fetch(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=DOWNLOAD_TIMEOUT) as resp:
        return resp.read()


def fetch_first(urls: list[str]) -> bytes:
    """Try each mirror in order; raise the last error if all fail."""
    last_error: Exception | None = None
    for url in urls:
        try:
            return fetch(url)
        except (urllib.error.URLError, OSError) as e:
            print(f"[sync] mirror failed ({url}): {e}", file=sys.stderr)
            last_error = e
    raise last_error if last_error else urllib.error.URLError("no urls given")


def write_csv_atomic(target_dir: str, filename: str, rows: list[Row]) -> str:
    """Write rows to ``target_dir/filename`` via temp-file + atomic rename,
    so readers (and a crash mid-write) only ever see complete files."""
    fd, tmp_path = tempfile.mkstemp(
        prefix=f".{filename}.", suffix=".tmp", dir=target_dir
    )
    try:
        with os.fdopen(fd, "w", newline="", encoding="utf-8") as f:
            writer = csv.writer(f)
            writer.writerow(CSV_HEADER)
            for row in rows:
                writer.writerow(row.as_csv())
        final = os.path.join(target_dir, filename)
        os.replace(tmp_path, final)
        return final
    except BaseException:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass
        raise


def _sync_source(name, filename, target_dir, min_rows, build_rows) -> bool:
    """Run one source end-to-end; True on success. Failures are contained:
    log to stderr, leave any previous file untouched."""
    try:
        rows = build_rows()
        if len(rows) < min_rows:
            raise ValueError(
                f"only {len(rows)} rows parsed (--min-rows {min_rows}) — "
                "refusing to overwrite the previous file with a suspiciously "
                "small list"
            )
        path = write_csv_atomic(target_dir, filename, rows)
        print(f"[sync] {name}: {len(rows)} entries -> {path}")
        return True
    except (urllib.error.URLError, OSError, ValueError, ET.ParseError) as e:
        print(f"[sync] ERROR: {name} failed ({e}); previous {filename} kept",
              file=sys.stderr)
        return False


def sync(target_dir: str, eu_url: str | None, min_rows: int) -> int:
    os.makedirs(target_dir, exist_ok=True)
    ok = True

    def ofac_rows():
        rows = parse_ofac_csv(
            fetch_first(OFAC_SDN_URLS).decode("utf-8-sig", errors="replace"),
            "OFAC-SDN",
        )
        rows += parse_ofac_csv(
            fetch_first(OFAC_CONS_URLS).decode("utf-8-sig", errors="replace"),
            "OFAC-CONS",
        )
        return rows

    ok &= _sync_source(
        "OFAC SDN+Consolidated", "sanctions_ofac.csv", target_dir, min_rows,
        ofac_rows,
    )
    ok &= _sync_source(
        "UN Consolidated", "sanctions_un.csv", target_dir, min_rows,
        lambda: parse_un_xml(fetch(UN_URL)),
    )
    if eu_url:
        ok &= _sync_source(
            "EU FSF", "sanctions_eu.csv", target_dir, min_rows,
            lambda: parse_eu_xml(fetch(eu_url)),
        )
    else:
        print("[sync] EU FSF skipped (no --eu-url; the EU download URL "
              "embeds an operator token — see README)")
    return 0 if ok else 1


# ─── verify (no network) ─────────────────────────────────────────────────────


def verify(target_dir: str, max_age_hours: float, expect_eu: bool) -> int:
    """Freshness check for cron/monitoring: every expected file must exist,
    contain more than just the header, and be younger than --max-age-hours."""
    expected = ["sanctions_ofac.csv", "sanctions_un.csv"]
    if expect_eu or os.path.exists(
        os.path.join(target_dir, "sanctions_eu.csv")
    ):
        expected.append("sanctions_eu.csv")
    ok = True
    now = time.time()
    for filename in expected:
        path = os.path.join(target_dir, filename)
        if not os.path.isfile(path):
            print(f"[verify] MISSING: {path}", file=sys.stderr)
            ok = False
            continue
        age_h = (now - os.path.getmtime(path)) / 3600
        with open(path, newline="", encoding="utf-8") as f:
            entries = max(0, sum(1 for _ in f) - 1)  # minus header
        status = []
        if age_h > max_age_hours:
            status.append(f"STALE ({age_h:.1f}h > {max_age_hours}h)")
        if entries == 0:
            status.append("EMPTY")
        if status:
            print(f"[verify] {path}: {', '.join(status)}", file=sys.stderr)
            ok = False
        else:
            print(f"[verify] {path}: {entries} entries, {age_h:.1f}h old — ok")
    return 0 if ok else 1


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=__doc__.splitlines()[0],
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "target_dir",
        help="directory the ekyc-ml-service reads lists from (EKYC_DATA_DIR)",
    )
    parser.add_argument(
        "--eu-url",
        default=None,
        help="EU FSF XML download URL (embeds your operator token, "
        "https://webgate.ec.europa.eu/fsd/fsf — omit to skip the EU list)",
    )
    parser.add_argument(
        "--min-rows",
        type=int,
        default=50,
        help="refuse to overwrite a previous file with fewer parsed rows "
        "than this (guards against truncated/garbage downloads; default 50)",
    )
    parser.add_argument(
        "--verify",
        action="store_true",
        help="no downloads: check output files exist, are populated and "
        "fresher than --max-age-hours",
    )
    parser.add_argument(
        "--max-age-hours",
        type=float,
        default=48.0,
        help="--verify freshness threshold (default 48h for a daily cron)",
    )
    args = parser.parse_args(argv)
    if args.verify:
        return verify(args.target_dir, args.max_age_hours, bool(args.eu_url))
    return sync(args.target_dir, args.eu_url, args.min_rows)


if __name__ == "__main__":
    sys.exit(main())
