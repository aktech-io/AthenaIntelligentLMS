"""CelebA-Spoof loader — original archive layout AND the HF parquet mirror.

RESEARCH-ONLY LICENSE — see liveness-training/DATASETS.md. Bootstraps the
teacher and internal evals only; the commercial certified model trains on
NLD-EA.

Two on-disk layouts are supported, auto-detected from the root:

1. **Original archive layout** (github.com/ZhangYuanhan-AI/CelebA-Spoof)::

       <root>/Data/train/<subjectID>/live/<imageID>.png|.jpg
       <root>/Data/train/<subjectID>/spoof/<imageID>.png|.jpg
       <root>/Data/train/<subjectID>/spoof/<imageID>_BB.txt   (optional bbox)
       <root>/metas/intra_test/train_label.json               (optional, 43-attr
                                                               vectors; index 40
                                                               = spoof type)

   Subject id = the numeric directory name. When the metas JSON is present
   the 0..10 spoof-type code is mapped onto the NLD-EA attack vocabulary;
   otherwise spoof images get attack_type="unknown_2d".

2. **HF parquet mirror** (Ar4ikov/celebA_spoof, e.g. a local snapshot at
   /mnt/ml/datasets/celeba-spoof-parquet)::

       <root>/data/train-*.parquet   (also valid-*/test-*)

   Row schema: ``Filepath`` (HF image feature -> struct{bytes, path}),
   ``Bbox`` (list<int64>), ``Class`` ("live"|"spoof"). Requires **pyarrow**
   (optional extra — install it only for parquet mode). Attack-type detail
   is not in this mirror, so spoof rows are attack_type="unknown_2d".
   Indexing reads only the nested ``Filepath.path`` + ``Class`` columns —
   never the ~46GB of image bytes.

   SUBJECT-IDENTITY CAVEAT (verified against the real mirror 2026-08-02):
   the mirror's embedded ``path`` is a bare image id ("519622.png") — the
   original ``Data/<split>/<subjectID>/<live|spoof>/`` prefix is stripped,
   so the mirror carries NO subject identity. When paths do keep the
   original prefix the loader recovers real subjects; otherwise it falls
   back to per-image pseudo-subjects (stable across re-downloads) and emits
   a RuntimeWarning: the split is then per-image, NOT subject-disjoint.
   For subject-safe training/evals use the original archive layout.

The original-archive mode splits train/val **subject-disjoint** by hashing
subject ids (base.split_subjects_disjoint) — never by the archive's own
partitions.
"""
from __future__ import annotations

import json
import re
import warnings
from pathlib import Path
from typing import Iterator, Optional

import numpy as np

from liveness_training.datasets.base import (
    PadDatasetBase,
    PadSample,
    split_subjects_disjoint,
)
from liveness_training.deployment import LABEL_LIVE, LABEL_SPOOF

# CelebA-Spoof 43-attr vector, index 40 (spoof type) -> NLD-EA vocabulary.
# 0=Live 1=Photo 2=Poster 3=A4 4=Face Mask 5=Upper Body Mask 6=Region Mask
# 7=PC 8=Pad 9=Phone 10=3D Mask
SPOOF_TYPE_MAP = {
    1: "print_flat",
    2: "print_flat",
    3: "print_flat",
    4: "cutout_paper",
    5: "cutout_paper",
    6: "cutout_paper",
    7: "replay_monitor",
    8: "replay_monitor",
    9: "replay_phone",
    10: "mask_3d",
}

_IMG_EXTS = (".png", ".jpg", ".jpeg")
# original-path shape kept by the parquet mirror, e.g.
# ".../Data/train/12345/spoof/000001.png"
_PARQUET_PATH_RE = re.compile(r"(?:^|/)(?P<subject>\d+)/(?P<kind>live|spoof)/[^/]+$")


def _decode_image_bytes(buf: bytes) -> np.ndarray:
    import cv2

    img = cv2.imdecode(np.frombuffer(buf, dtype=np.uint8), cv2.IMREAD_COLOR)
    if img is None:
        raise IOError("could not decode embedded image bytes")
    return img  # BGR uint8


class CelebASpoofDataset(PadDatasetBase):
    """split: "train" | "val" — OUR subject-disjoint split over the source
    partition, not the archive's own. source_partition picks which archive
    partition to draw from ("train" by default; "test" for held-out evals).
    """

    def __init__(
        self,
        root,
        split: str = "train",
        source_partition: str = "train",
        val_fraction: float = 0.15,
        max_samples: Optional[int] = None,
    ) -> None:
        if split not in ("train", "val"):
            raise ValueError(f"split must be train|val, got {split!r}")
        self.root = Path(root)
        self.split = split
        self.source_partition = source_partition
        self.val_fraction = val_fraction
        self.max_samples = max_samples
        if not self.root.is_dir():
            raise FileNotFoundError(f"CelebA-Spoof root not found: {self.root}")

        self.mode = self._detect_mode()
        if self.mode == "parquet":
            self._index_parquet()
        else:
            self._index_original()

    # ─── layout detection ────────────────────────────────────────────────
    def _detect_mode(self) -> str:
        if list(self.root.glob("data/*.parquet")) or list(self.root.glob("*.parquet")):
            return "parquet"
        if (self.root / "Data").is_dir():
            return "original"
        # empty/in-progress parquet snapshot (README present, shards pending)
        if (self.root / "data").is_dir():
            return "parquet"
        raise FileNotFoundError(
            f"{self.root}: neither Data/ (original layout) nor *.parquet "
            "(HF mirror) found"
        )

    # ─── original archive layout ─────────────────────────────────────────
    def _index_original(self) -> None:
        part_dir = self.root / "Data" / self.source_partition
        if not part_dir.is_dir():
            raise FileNotFoundError(f"missing partition dir {part_dir}")
        self._attack_types = self._load_meta_attack_types()
        records: list[tuple[Path, int, str]] = []  # (image, label, subject)
        for subj_dir in sorted(p for p in part_dir.iterdir() if p.is_dir()):
            for kind, label in (("live", LABEL_LIVE), ("spoof", LABEL_SPOOF)):
                kdir = subj_dir / kind
                if not kdir.is_dir():
                    continue
                for img in sorted(kdir.iterdir()):
                    if img.suffix.lower() in _IMG_EXTS:
                        records.append((img, label, subj_dir.name))
        self._finish_index(records)

    def _load_meta_attack_types(self) -> dict[str, str]:
        """relative image path -> NLD-EA attack type, from the metas JSON."""
        meta = self.root / "metas" / "intra_test" / f"{self.source_partition}_label.json"
        out: dict[str, str] = {}
        if not meta.is_file():
            return out
        try:
            data = json.loads(meta.read_text(encoding="utf-8"))
        except (json.JSONDecodeError, OSError):
            return out
        for rel_path, attrs in data.items():
            if isinstance(attrs, list) and len(attrs) > 40:
                out[rel_path.lstrip("/")] = SPOOF_TYPE_MAP.get(int(attrs[40]), "unknown_2d")
        return out

    # ─── HF parquet mirror ───────────────────────────────────────────────
    def _parquet_files(self) -> list[Path]:
        pattern = f"{self.source_partition}-*.parquet"
        files = sorted(self.root.glob(f"data/{pattern}")) or sorted(self.root.glob(pattern))
        if not files:
            raise FileNotFoundError(
                f"{self.root}: no {pattern} shards found — download still in "
                "progress, or wrong source_partition (train|valid|test)?"
            )
        return files

    def _index_parquet(self) -> None:
        try:
            import pyarrow.parquet  # noqa: F401
        except ImportError as e:  # pragma: no cover - env dependent
            raise ImportError(
                "the CelebA-Spoof HF parquet mirror needs pyarrow: "
                "pip install pyarrow"
            ) from e
        # (file, row_group, row_in_group, label, subject_id)
        records: list[tuple[Path, int, int, int, str]] = []
        self._pseudo_subjects = False
        for pf in self._parquet_files():
            meta_reader = pyarrow.parquet.ParquetFile(pf)
            for rg in range(meta_reader.num_row_groups):
                tbl = self._read_meta_group(meta_reader, rg)
                paths, classes = self._extract_meta(tbl)
                for i in range(tbl.num_rows):
                    label, subject = self._row_identity(paths[i], classes[i], pf, rg, i)
                    records.append((pf, rg, i, label, subject))
        if self._pseudo_subjects:
            warnings.warn(
                "CelebA-Spoof parquet mirror carries no subject identity "
                "(bare image-id paths) — falling back to per-image "
                "pseudo-subjects. The train/val split is per-IMAGE, NOT "
                "subject-disjoint; the same celebrity can appear in both. "
                "Use the original archive layout for subject-safe evals.",
                RuntimeWarning,
                stacklevel=3,
            )
        self._finish_index(records, parquet=True)

    @staticmethod
    def _read_meta_group(reader, rg: int):
        """Read only path + class for indexing. Nested selection
        ("Filepath.path") skips the image bytes — verified against the real
        mirror; fall back to the full struct column on older pyarrow."""
        names = set(reader.schema_arrow.names)
        if not {"Filepath", "Class"} & names:
            raise ValueError("parquet shard has neither Filepath nor Class column")
        cols = []
        if "Filepath" in names:
            cols.append("Filepath.path")
        if "Class" in names:
            cols.append("Class")
        try:
            return reader.read_row_group(rg, columns=cols)
        except (KeyError, OSError, ValueError):  # pragma: no cover - old pyarrow
            return reader.read_row_group(
                rg, columns=[c for c in ("Filepath", "Class") if c in names]
            )

    @staticmethod
    def _extract_meta(tbl) -> tuple[list, list]:
        n = tbl.num_rows
        paths: list = [None] * n
        classes: list = [None] * n
        if "Filepath" in tbl.column_names:
            col = tbl.column("Filepath").to_pylist()
            # HF image feature -> {"bytes": ..., "path": ...}; keep only path here
            paths = [(v or {}).get("path") if isinstance(v, dict) else v for v in col]
        if "Class" in tbl.column_names:
            classes = tbl.column("Class").to_pylist()
        return paths, classes

    def _row_identity(self, path, cls, pf: Path, rg: int, i: int) -> tuple[int, str]:
        """(label, subject_id) for a parquet row. Uses the original path
        when the mirror preserved it (subject dir + live/spoof); otherwise
        label comes from Class and the subject degrades to a per-image
        pseudo-subject keyed on the image id (stable across re-downloads)."""
        norm = path.replace("\\", "/") if isinstance(path, str) else None
        if norm:
            m = _PARQUET_PATH_RE.search(norm)
            if m:
                label = LABEL_LIVE if m.group("kind") == "live" else LABEL_SPOOF
                return label, m.group("subject")
        if isinstance(cls, str):
            c = cls.strip().lower()
            if c in ("live", "real", "genuine"):
                label = LABEL_LIVE
            elif c in ("spoof", "fake", "attack"):
                label = LABEL_SPOOF
            else:
                raise ValueError(f"{pf} rg{rg} row{i}: unrecognized Class {cls!r}")
            self._pseudo_subjects = True
            image_id = Path(norm).stem if norm else f"{pf.stem}:{rg}:{i}"
            return label, f"img_{image_id}"
        raise ValueError(f"{pf} rg{rg} row{i}: cannot determine label")

    # ─── shared ──────────────────────────────────────────────────────────
    def _finish_index(self, records: list, parquet: bool = False) -> None:
        subjects = {r[-1] for r in records}
        train_subj, val_subj = split_subjects_disjoint(subjects, self.val_fraction)
        keep = train_subj if self.split == "train" else val_subj
        records = [r for r in records if r[-1] in keep]
        if self.max_samples is not None:
            records = records[: self.max_samples]
        self._records = records
        self._parquet = parquet

    def __len__(self) -> int:
        return len(self._records)

    def subjects(self) -> set[str]:
        return {r[-1] for r in self._records}

    def __iter__(self) -> Iterator[PadSample]:
        if self._parquet:
            yield from self._iter_parquet()
        else:
            yield from self._iter_original()

    def _iter_original(self) -> Iterator[PadSample]:
        import cv2

        for img_path, label, subject in self._records:
            img = cv2.imread(str(img_path), cv2.IMREAD_COLOR)
            if img is None:
                continue  # tolerate the occasional corrupt archive file
            attack = None
            if label == LABEL_SPOOF:
                rel = str(img_path.relative_to(self.root))
                attack = self._attack_types.get(rel, "unknown_2d")
            yield PadSample(
                frames=[img],
                label=label,
                attack_type=attack,
                skin_tone=None,  # CelebA-Spoof carries no skin-tone labels
                subject_id=subject,
                meta={"path": str(img_path), "dataset": "celeba_spoof"},
            )

    def _iter_parquet(self) -> Iterator[PadSample]:
        import pyarrow.parquet

        # group rows by (file, row_group) so each group is read exactly once
        by_group: dict[tuple[Path, int], list[tuple[int, int, str]]] = {}
        for pf, rg, i, label, subject in self._records:
            by_group.setdefault((pf, rg), []).append((i, label, subject))
        for (pf, rg), rows in by_group.items():
            tbl = pyarrow.parquet.ParquetFile(pf).read_row_group(rg, columns=["Filepath"])
            col = tbl.column("Filepath").to_pylist()
            for i, label, subject in rows:
                cell = col[i]
                buf = cell.get("bytes") if isinstance(cell, dict) else cell
                if not buf:
                    continue
                img = _decode_image_bytes(buf)
                yield PadSample(
                    frames=[img],
                    label=label,
                    attack_type="unknown_2d" if label == LABEL_SPOOF else None,
                    skin_tone=None,
                    subject_id=subject,
                    meta={
                        "path": (cell.get("path") if isinstance(cell, dict) else None),
                        "dataset": "celeba_spoof_parquet",
                        "shard_file": pf.name,
                    },
                )
