"""Sanctions / PEP name screening over CSV list files.

The matcher is the product; the lists are data. This module loads every
`sanctions*.csv` and `pep*.csv` under the data directory (EKYC_DATA_DIR, or
the packaged ./data), so ops refresh real lists by dropping files in — the
shipped *_demo.csv files are placeholders for development and demos.

CSV columns: id,name,aliases,country,program
  - aliases: ';'-separated alternate spellings (optional)
  - extra columns are ignored; a header row is required.

Real-list sources to drop in (refresh cadence: daily, via ops cron):
  - OFAC SDN + Consolidated (https://sanctionslist.ofac.treas.gov/)
  - UN Security Council Consolidated List
  - EU Consolidated Financial Sanctions List
  - PEP data: commercial feed or curated national lists
converted to the CSV shape above (one row per entity, aliases joined by ';').
"""
from __future__ import annotations

import csv
import glob
import os
import threading
from dataclasses import dataclass, field as dc_field

from engine.names import best_alias_score, normalize_name

DEFAULT_THRESHOLD = 0.85


@dataclass
class ListEntry:
    entry_id: str
    name: str
    list_name: str  # "sanctions" | "pep"
    source_file: str
    aliases: list[str] = dc_field(default_factory=list)
    country: str = ""
    program: str = ""

    @property
    def all_names(self) -> list[str]:
        return [self.name, *self.aliases]


@dataclass
class Match:
    list_name: str
    entry_id: str
    matched_name: str
    score: float
    source_file: str

    def as_dict(self) -> dict:
        return {
            "list": self.list_name,
            "entryId": self.entry_id,
            "name": self.matched_name,
            "score": self.score,
            "sourceFile": self.source_file,
        }


def _load_csv(path: str, list_name: str) -> list[ListEntry]:
    entries = []
    with open(path, newline="", encoding="utf-8-sig") as f:
        for row in csv.DictReader(f):
            name = (row.get("name") or "").strip()
            if not name:
                continue
            aliases = [
                a.strip() for a in (row.get("aliases") or "").split(";") if a.strip()
            ]
            entries.append(
                ListEntry(
                    entry_id=(row.get("id") or "").strip(),
                    name=name,
                    list_name=list_name,
                    source_file=os.path.basename(path),
                    aliases=aliases,
                    country=(row.get("country") or "").strip(),
                    program=(row.get("program") or "").strip(),
                )
            )
    return entries


class Screener:
    """Loads list files once and screens names against them."""

    def __init__(self, data_dir: str):
        self.data_dir = data_dir
        self.entries: list[ListEntry] = []
        self.files: list[str] = []
        self.reload()

    def reload(self) -> None:
        entries: list[ListEntry] = []
        files: list[str] = []
        for pattern, list_name in (("sanctions*.csv", "sanctions"), ("pep*.csv", "pep")):
            for path in sorted(glob.glob(os.path.join(self.data_dir, pattern))):
                entries.extend(_load_csv(path, list_name))
                files.append(os.path.basename(path))
        self.entries = entries
        self.files = files

    def screen(self, full_name: str, threshold: float = DEFAULT_THRESHOLD) -> list[Match]:
        """All list entries whose best name/alias similarity >= threshold,
        strongest first."""
        if not normalize_name(full_name):
            return []
        matches = []
        for e in self.entries:
            score, matched = best_alias_score(full_name, e.all_names)
            if score >= threshold:
                matches.append(
                    Match(
                        list_name=e.list_name,
                        entry_id=e.entry_id,
                        matched_name=matched,
                        score=score,
                        source_file=e.source_file,
                    )
                )
        matches.sort(key=lambda m: m.score, reverse=True)
        return matches


_screener: Screener | None = None
_lock = threading.Lock()


def get_screener() -> Screener:
    """Process-wide screener over EKYC_DATA_DIR (default: packaged ./data)."""
    global _screener
    with _lock:
        if _screener is None:
            data_dir = os.getenv(
                "EKYC_DATA_DIR",
                os.path.join(os.path.dirname(os.path.dirname(__file__)), "data"),
            )
            _screener = Screener(data_dir)
        return _screener
