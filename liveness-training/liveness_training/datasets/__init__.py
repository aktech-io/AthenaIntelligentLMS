"""Dataset loaders with one common contract.

NOTE ON THE PACKAGE NAME: this package is ``datasets`` (not ``data``) on
purpose — the repo's root .gitignore contains ``**/data/`` and has silently
eaten source directories named ``data/`` before.

Every loader yields :class:`liveness_training.datasets.base.PadSample`
(frames, label, attack_type, skin_tone|None, subject_id) and every loader
splits train/val **subject-disjoint** — a subject's samples never straddle
shards, so there is no identity leakage between train and val.
"""
from liveness_training.datasets.base import (  # noqa: F401
    ATTACK_TYPES,
    PadSample,
    FramePadDataset,
    subject_shard,
)
from liveness_training.datasets.celeba_spoof import CelebASpoofDataset  # noqa: F401
from liveness_training.datasets.cefa import CeFADataset  # noqa: F401
from liveness_training.datasets.nldea import NLDEADataset  # noqa: F401
from liveness_training.datasets.synthetic import (  # noqa: F401
    SyntheticPadDataset,
    generate_celeba_spoof_fixture,
    generate_cefa_fixture,
    generate_nldea_fixture,
)
