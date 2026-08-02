import sys
from pathlib import Path

# make `liveness_training` importable without installation
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import pytest  # noqa: E402


@pytest.fixture(scope="session")
def nldea_root(tmp_path_factory):
    from liveness_training.datasets.synthetic import generate_nldea_fixture

    return generate_nldea_fixture(
        tmp_path_factory.mktemp("nldea"),
        n_subjects=12, sessions_per_subject=2, clips_per_session=1,
        frames_per_clip=6, size=96,
    )


@pytest.fixture(scope="session")
def celeba_root(tmp_path_factory):
    from liveness_training.datasets.synthetic import generate_celeba_spoof_fixture

    return generate_celeba_spoof_fixture(
        tmp_path_factory.mktemp("celeba"), n_subjects=8, imgs_per_class=2, size=96
    )


@pytest.fixture(scope="session")
def cefa_root(tmp_path_factory):
    from liveness_training.datasets.synthetic import generate_cefa_fixture

    return generate_cefa_fixture(
        tmp_path_factory.mktemp("cefa"), n_subjects_per_race=3, frames_per_video=3, size=96
    )
