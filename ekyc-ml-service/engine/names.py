"""Name normalization and fuzzy matching (pure stdlib).

Used both for sanctions/PEP screening and (mirrored in Go) for comparing
declared vs OCR-extracted names. Deterministic, dependency-free: NFKD
diacritic stripping + token-aware similarity built on difflib.
"""
from __future__ import annotations

import re
import unicodedata
from difflib import SequenceMatcher


def normalize_name(s: str) -> str:
    """Casefold, strip diacritics and punctuation, collapse whitespace.

    'Müller-Ávila, José' -> 'muller avila jose'
    """
    s = unicodedata.normalize("NFKD", s)
    s = "".join(c for c in s if not unicodedata.combining(c))
    s = s.casefold()
    s = re.sub(r"[^a-z0-9]+", " ", s)
    return " ".join(s.split())


def _ratio(a: str, b: str) -> float:
    return SequenceMatcher(None, a, b).ratio()


def similarity(a: str, b: str) -> float:
    """Similarity of two names in [0, 1], order-insensitive.

    Max of:
      - full-string ratio on normalized forms,
      - token-sort ratio (handles SURNAME FIRST vs First Surname),
      - token-set containment (handles missing middle names), discounted so a
        bare single-token subset cannot alone clear typical thresholds.
    """
    na, nb = normalize_name(a), normalize_name(b)
    if not na or not nb:
        return 0.0
    if na == nb:
        return 1.0

    scores = [_ratio(na, nb)]

    ta, tb = na.split(), nb.split()
    scores.append(_ratio(" ".join(sorted(ta)), " ".join(sorted(tb))))

    sa, sb = set(ta), set(tb)
    inter = sa & sb
    if inter:
        containment = len(inter) / min(len(sa), len(sb))
        coverage = len(inter) / max(len(sa), len(sb))
        # full containment of the shorter name scores high but not 1.0
        scores.append(0.55 * containment + 0.45 * coverage)

    return round(min(1.0, max(scores)), 4)


def best_alias_score(query: str, names: list[str]) -> tuple[float, str]:
    """Highest similarity between query and any of names; ('' , 0) if empty."""
    best, best_name = 0.0, ""
    for n in names:
        s = similarity(query, n)
        if s > best:
            best, best_name = s, n
    return best, best_name
