"""Pure-logic tests for the PP-OCR engine (engine/ppocr.py).

Charset mapping and line clustering are stdlib-only; CTC decoding needs
numpy and self-skips without it (same pattern as test_liveness.py). The
full det/rec pipeline needs model files and is exercised against the
running service.
"""
from __future__ import annotations

import pytest

from engine.ppocr import (
    charset_for_classes,
    cluster_lines,
    load_charset,
)

try:
    import numpy as np

    HAVE_NUMPY = True
except ImportError:
    HAVE_NUMPY = False


class TestCharset:
    def test_load_charset_skips_trailing_blank_lines(self):
        assert load_charset("a\nb\nc\n\n") == ["a", "b", "c"]

    def test_space_class_appended_when_model_asks(self):
        cs = charset_for_classes(["a", "b"], num_classes=4)
        assert cs == ["a", "b", " "]

    def test_no_space_class(self):
        assert charset_for_classes(["a", "b"], num_classes=3) == ["a", "b"]

    def test_mismatched_dict_raises(self):
        with pytest.raises(ValueError, match="does not match"):
            charset_for_classes(["a", "b"], num_classes=7)


@pytest.mark.skipif(not HAVE_NUMPY, reason="numpy not installed")
class TestCtcDecode:
    def _probs(self, rows):
        return np.array(rows, dtype=np.float32)

    def test_collapses_repeats_and_drops_blanks(self):
        from engine.ppocr import ctc_greedy_decode

        # classes: 0=blank, 1='H', 2='I' — sequence H H blank I
        probs = self._probs(
            [
                [0.1, 0.8, 0.1],
                [0.1, 0.8, 0.1],
                [0.9, 0.05, 0.05],
                [0.1, 0.1, 0.8],
            ]
        )
        text, conf = ctc_greedy_decode(probs, ["H", "I"])
        assert text == "HI"
        assert conf == pytest.approx(0.8, abs=1e-6)

    def test_repeat_after_blank_is_kept(self):
        from engine.ppocr import ctc_greedy_decode

        # A blank A -> "AA" (CTC: blank separates genuine repeats)
        probs = self._probs(
            [[0.1, 0.9], [0.9, 0.1], [0.1, 0.9]]
        )
        text, _ = ctc_greedy_decode(probs, ["A"])
        assert text == "AA"

    def test_all_blank_is_empty(self):
        from engine.ppocr import ctc_greedy_decode

        probs = self._probs([[0.9, 0.1], [0.9, 0.1]])
        assert ctc_greedy_decode(probs, ["A"]) == ("", 0.0)


class TestClusterLines:
    def test_same_row_grouped_left_to_right(self):
        # two boxes on one visual line, given right-first
        boxes = [(100, 10, 150, 30), (10, 12, 60, 28)]
        assert cluster_lines(boxes) == [[1, 0]]

    def test_separate_rows_ordered_top_to_bottom(self):
        boxes = [(10, 100, 60, 120), (10, 10, 60, 30)]
        assert cluster_lines(boxes) == [[1], [0]]

    def test_empty(self):
        assert cluster_lines([]) == []

    def test_ragged_baselines_still_one_line(self):
        # centre of one box falls inside the other's span despite offset tops
        boxes = [(10, 10, 60, 40), (70, 20, 120, 44)]
        assert cluster_lines(boxes) == [[0, 1]]
