"""Name matcher + screening tests (pure logic + demo CSV files)."""
import os

from engine.names import normalize_name, similarity
from engine.screening import Screener

DATA_DIR = os.path.join(os.path.dirname(os.path.dirname(__file__)), "data")


class TestNormalizeName:
    def test_case_and_whitespace(self):
        assert normalize_name("  JANE   Mwangi ") == "jane mwangi"

    def test_diacritics(self):
        assert normalize_name("José Müller-Ávila") == "jose muller avila"

    def test_punctuation(self):
        assert normalize_name("O'Brien, Anne-Marie") == "o brien anne marie"

    def test_empty(self):
        assert normalize_name("  .,- ") == ""


class TestSimilarity:
    def test_identical(self):
        assert similarity("Jane Mwangi", "JANE MWANGI") == 1.0

    def test_token_order_insensitive(self):
        assert similarity("Mwangi Jane Wanjiku", "Jane Wanjiku Mwangi") == 1.0

    def test_missing_middle_name_scores_high(self):
        s = similarity("Jane Mwangi", "Jane Wanjiku Mwangi")
        assert s >= 0.8

    def test_different_names_score_low(self):
        assert similarity("Jane Mwangi", "Peter Ochieng") < 0.5

    def test_minor_ocr_typo_scores_high(self):
        assert similarity("Jane Mwangi", "Jane Mwangl") >= 0.85

    def test_single_shared_common_token_not_a_match(self):
        # sharing only a first name must not clear typical thresholds
        assert similarity("Jane Mwangi", "Jane Ochieng") < 0.85

    def test_empty_scores_zero(self):
        assert similarity("", "Jane Mwangi") == 0.0
        assert similarity("Jane Mwangi", "") == 0.0

    def test_symmetry(self):
        a, b = "Viktor Nikolayevich Orlov", "Viktor Orlov"
        assert similarity(a, b) == similarity(b, a)


class TestScreener:
    def setup_method(self):
        self.s = Screener(DATA_DIR)

    def test_loads_demo_lists(self):
        assert "sanctions_demo.csv" in self.s.files
        assert "pep_demo.csv" in self.s.files
        assert len(self.s.entries) >= 8

    def test_exact_sanctions_hit(self):
        matches = self.s.screen("Viktor Nikolayevich Orlov")
        assert matches and matches[0].list_name == "sanctions"
        assert matches[0].score == 1.0

    def test_alias_hit(self):
        matches = self.s.screen("Viktor Orlov")
        assert any(m.entry_id == "DEMO-SDN-002" for m in matches)

    def test_reordered_name_hit(self):
        matches = self.s.screen("Warsame Abdi Farah")
        assert any(m.entry_id == "DEMO-SDN-001" for m in matches)

    def test_pep_hit_is_separate_list(self):
        matches = self.s.screen("Margaret Wanjiru Kamau")
        assert matches and all(m.list_name == "pep" for m in matches)

    def test_clean_name_no_hit(self):
        assert self.s.screen("Grace Njeri Kariuki") == []

    def test_threshold_is_respected(self):
        loose = self.s.screen("Viktor Orlof", threshold=0.7)
        strict = self.s.screen("Viktor Orlof", threshold=0.99)
        assert len(loose) >= len(strict)

    def test_matches_sorted_by_score(self):
        matches = self.s.screen("Test Sanctioned Person", threshold=0.6)
        scores = [m.score for m in matches]
        assert scores == sorted(scores, reverse=True)

    def test_empty_name_no_hits(self):
        assert self.s.screen("") == []
