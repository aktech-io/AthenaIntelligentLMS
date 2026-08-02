"""Attack taxonomy: species registry and the manifest contract's value set."""
from __future__ import annotations

import pytest

from liveness_redteam import taxonomy


def test_level1_species_are_the_ibeta_l1_repertoire():
    assert set(taxonomy.l1_species()) == {
        "print_flat",
        "print_curved",
        "replay_phone",
        "replay_tablet",
        "replay_monitor",
        "cutout_paper",
    }


def test_level2_species_are_the_mask_family_and_marked_future():
    assert set(taxonomy.l2_species()) == {
        "mask_silicone",
        "mask_latex",
        "mask_resin",
        "mask_3d",
    }
    for key in taxonomy.l2_species():
        assert taxonomy.species(key).is_future
        assert taxonomy.species(key).category == "mask"


def test_level1_species_are_active():
    for key in taxonomy.l1_species():
        assert taxonomy.species(key).status == taxonomy.STATUS_ACTIVE


def test_contract_attack_types_are_all_known_species():
    for key in taxonomy.CONTRACT_ATTACK_TYPES:
        assert taxonomy.is_attack_type(key)
    # the shared NLD-EA contract carries the generic mask value
    assert "mask_3d" in taxonomy.CONTRACT_ATTACK_TYPES
    # the red-team superset adds material granularity + tablets
    assert set(taxonomy.EXTRA_ATTACK_TYPES) == {
        "replay_tablet",
        "mask_silicone",
        "mask_latex",
        "mask_resin",
    }


def test_attack_types_is_contract_plus_extras_without_duplicates():
    assert len(set(taxonomy.ATTACK_TYPES)) == len(taxonomy.ATTACK_TYPES)
    assert set(taxonomy.ATTACK_TYPES) == set(taxonomy.SPECIES)


def test_every_species_documents_materials_and_capture_notes():
    for spec in taxonomy.all_species():
        assert spec.description.strip()
        assert spec.materials.strip()
        assert spec.capture_notes.strip()
        assert spec.level in (taxonomy.LEVEL_1, taxonomy.LEVEL_2)


def test_unknown_species_raises():
    assert not taxonomy.is_attack_type("deepfake_injection")
    with pytest.raises(KeyError):
        taxonomy.species("deepfake_injection")


def test_skin_tone_and_lighting_vocabularies_match_the_contract():
    assert taxonomy.SKIN_TONES[0] == "monk_01"
    assert taxonomy.SKIN_TONES[-1] == "monk_10"
    assert len(taxonomy.SKIN_TONES) == 10
    assert taxonomy.LIGHTING == ("daylight", "indoor", "low_light")
    assert taxonomy.CHALLENGES == ("blink", "turn_left", "turn_right", "smile")
