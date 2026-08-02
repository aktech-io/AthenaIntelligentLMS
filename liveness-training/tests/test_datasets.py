"""Loader-contract tests for CelebA-Spoof (both layouts), CeFA, synthetic,
and the FramePadDataset adapter."""
from pathlib import Path

import numpy as np
import pytest

from liveness_training.datasets.base import (
    ATTACK_TYPES_EXTENDED,
    PadSample,
    split_subjects_disjoint,
)
from liveness_training.datasets.celeba_spoof import CelebASpoofDataset
from liveness_training.datasets.cefa import CeFADataset
from liveness_training.datasets.synthetic import SyntheticPadDataset

try:
    import pyarrow  # noqa: F401

    HAVE_PYARROW = True
except ImportError:
    HAVE_PYARROW = False


def _assert_contract(samples):
    assert samples
    for s in samples:
        assert isinstance(s, PadSample)
        assert s.frames and all(f.dtype == np.uint8 and f.ndim == 3 for f in s.frames)
        assert s.label in (0, 1)
        if s.label == 0:
            assert s.attack_type in ATTACK_TYPES_EXTENDED
        else:
            assert s.attack_type is None
        assert isinstance(s.subject_id, str) and s.subject_id


class TestPadSampleValidation:
    def test_label_attack_consistency_enforced(self):
        frame = np.zeros((8, 8, 3), np.uint8)
        with pytest.raises(ValueError):
            PadSample([frame], 1, "print_flat", None, "s")  # live with attack
        with pytest.raises(ValueError):
            PadSample([frame], 0, None, None, "s")  # spoof without attack
        with pytest.raises(ValueError):
            PadSample([frame], 0, "hologram", None, "s")  # unknown vocab
        with pytest.raises(ValueError):
            PadSample([frame], 1, None, "monk_00", "s")  # bad tone


class TestSubjectDisjointSplit:
    def test_disjoint_deterministic_and_complete(self):
        subjects = [f"s{i}" for i in range(500)]
        t1, v1 = split_subjects_disjoint(subjects, 0.15)
        t2, v2 = split_subjects_disjoint(subjects, 0.15)
        assert (t1, v1) == (t2, v2)
        assert t1.isdisjoint(v1)
        assert t1 | v1 == set(subjects)
        assert 0.08 < len(v1) / 500 < 0.24


class TestCelebASpoofOriginal:
    def test_contract_and_attack_types_from_metas(self, celeba_root):
        train = CelebASpoofDataset(celeba_root, split="train")
        samples = list(train)
        _assert_contract(samples)
        # metas JSON present -> spoof samples must get specific attack types
        spoof_types = {s.attack_type for s in samples if s.label == 0}
        assert spoof_types & {"print_flat", "replay_phone", "replay_monitor",
                              "cutout_paper", "mask_3d"}
        # skin tone: CelebA-Spoof has none
        assert all(s.skin_tone is None for s in samples)

    def test_subject_disjoint_train_val(self, celeba_root):
        train = CelebASpoofDataset(celeba_root, split="train")
        val = CelebASpoofDataset(celeba_root, split="val")
        assert train.subjects().isdisjoint(val.subjects())
        assert train.subjects() or val.subjects()


@pytest.fixture(scope="module")
def parquet_root(tmp_path_factory):
    """Real-mirror shape: bare image-id paths, no subject identity."""
    if not HAVE_PYARROW:
        pytest.skip("pyarrow not installed")
    from liveness_training.datasets.synthetic import (
        generate_celeba_spoof_parquet_fixture,
    )

    return generate_celeba_spoof_parquet_fixture(
        tmp_path_factory.mktemp("celeba_parquet"), n_subjects=6, imgs_per_class=2
    )


@pytest.mark.skipif(not HAVE_PYARROW, reason="pyarrow not installed")
class TestCelebASpoofParquet:
    def test_contract_and_pseudo_subject_warning(self, parquet_root):
        # the real mirror has no subject dirs -> loader must warn loudly
        with pytest.warns(RuntimeWarning, match="NOT subject-disjoint"):
            ds = CelebASpoofDataset(parquet_root, split="train")
        assert ds.mode == "parquet"
        samples = list(ds)
        _assert_contract(samples)
        assert {s.label for s in samples} == {0, 1}
        assert all(s.subject_id.startswith("img_") for s in samples)

    def test_split_still_deterministic_and_disjoint_per_image(self, parquet_root):
        with pytest.warns(RuntimeWarning):
            train = CelebASpoofDataset(parquet_root, split="train")
            train2 = CelebASpoofDataset(parquet_root, split="train")
            val = CelebASpoofDataset(parquet_root, split="val")
        assert train.subjects() == train2.subjects()
        assert train.subjects().isdisjoint(val.subjects())
        assert len(train) + len(val) == 24  # 6 ids * 2 classes * 2 imgs

    def test_preserved_paths_recover_real_subjects(self, tmp_path):
        from liveness_training.datasets.synthetic import (
            generate_celeba_spoof_parquet_fixture,
        )
        import warnings as _w

        root = generate_celeba_spoof_parquet_fixture(
            tmp_path / "pq_full", n_subjects=6, imgs_per_class=2, bare_paths=False
        )
        with _w.catch_warnings():
            _w.simplefilter("error", RuntimeWarning)  # no pseudo-subject warning
            train = CelebASpoofDataset(root, split="train")
            val = CelebASpoofDataset(root, split="val")
        assert all(s.isdigit() for s in train.subjects() | val.subjects())
        assert train.subjects().isdisjoint(val.subjects())


REAL_MIRROR = Path("/mnt/ml/datasets/celeba-spoof-parquet")


@pytest.mark.skipif(
    not (HAVE_PYARROW and list(REAL_MIRROR.glob("data/test-*.parquet"))),
    reason="real CelebA-Spoof parquet mirror not (yet) present",
)
class TestCelebASpoofRealMirror:
    """Opt-in integration check against the actual local HF mirror sync —
    validates the loader on real shards, a few samples only."""

    def test_reads_real_shards(self):
        import warnings as _w

        with _w.catch_warnings():
            _w.simplefilter("ignore", RuntimeWarning)  # pseudo-subject warning
            ds = CelebASpoofDataset(REAL_MIRROR, split="train",
                                    source_partition="test", max_samples=6)
        samples = list(ds)
        _assert_contract(samples)
        assert all(s.frames[0].shape[2] == 3 for s in samples)


class TestCeFA:
    def test_contract_and_ethnicity_meta(self, cefa_root):
        ds = CeFADataset(cefa_root, split="train")
        samples = list(ds)
        _assert_contract(samples)
        assert all(s.meta["ethnicity"] in ("african", "central_asian", "east_asian")
                   for s in samples)
        assert all(s.skin_tone is None for s in samples)  # ethnicity != Monk tone

    def test_ethnicity_filter(self, cefa_root):
        ds = CeFADataset(cefa_root, split="train", ethnicities=("african",))
        assert all(s.meta["ethnicity"] == "african" for s in ds)

    def test_subject_disjoint_and_race_namespacing(self, cefa_root):
        train = CeFADataset(cefa_root, split="train")
        val = CeFADataset(cefa_root, split="val")
        assert train.subjects().isdisjoint(val.subjects())
        assert all(s.startswith("cefa_") for s in train.subjects() | val.subjects())


class TestSyntheticAndAdapter:
    def test_synthetic_contract(self):
        _assert_contract(list(SyntheticPadDataset(n_subjects=8, samples_per_subject=2)))

    def test_synthetic_split_disjoint(self):
        t = SyntheticPadDataset(n_subjects=16, split="train")
        v = SyntheticPadDataset(n_subjects=16, split="val")
        assert t.subjects().isdisjoint(v.subjects())
        assert t.subjects() and v.subjects()

    def test_frame_adapter_deployment_convention(self):
        import torch

        from liveness_training.datasets.base import FramePadDataset

        ds = FramePadDataset(SyntheticPadDataset(n_subjects=6), size=80)
        item = ds[0]
        x = item["image"]
        assert isinstance(x, torch.Tensor) and x.shape == (3, 80, 80)
        assert x.dtype == torch.float32
        assert 0.0 <= float(x.min()) and float(x.max()) <= 255.0
        assert float(x.max()) > 1.5  # raw range, NOT normalized to [0,1]
        assert set(item) == {"image", "label", "attack_type", "skin_tone", "subject_id"}
