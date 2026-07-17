"""Field-extraction post-processing tests (pure logic, no tesseract)."""
from engine.fields import (
    FieldValue,
    OcrWord,
    clean_name,
    extract_fields,
    merge_mrz,
    normalize_docnum,
    parse_date_any,
)
from engine.mrz import parse_mrz


def words_from_lines(*lines, conf=0.9):
    out = []
    for i, line in enumerate(lines):
        for tok in line.split():
            out.append(OcrWord(text=tok, confidence=conf, line=i))
    return out


class TestParseDate:
    def test_common_formats(self):
        assert parse_date_any("14.02.1990") == "1990-02-14"
        assert parse_date_any("14/02/1990") == "1990-02-14"
        assert parse_date_any("1990-02-14") == "1990-02-14"
        assert parse_date_any("14 Feb 1990") == "1990-02-14"
        assert parse_date_any("14 February 1990") == "1990-02-14"

    def test_two_digit_year_birthdate_heuristic(self):
        # 90 must resolve to 1990, not 2090
        assert parse_date_any("14.02.90") == "1990-02-14"

    def test_not_a_date(self):
        assert parse_date_any("KENYA") == ""
        assert parse_date_any("12345678") == ""


class TestCleanName:
    def test_strips_ocr_noise(self):
        assert clean_name("J0HN| DOE_") == "HN DOE"  # digits/punct are noise
        assert clean_name("JOHN   DOE.") == "JOHN DOE"

    def test_keeps_hyphens_apostrophes(self):
        assert clean_name("ANNE-MARIE O'BRIEN") == "ANNE-MARIE O'BRIEN"

    def test_drops_single_char_fragments(self):
        assert clean_name("| JOHN x DOE") == "JOHN DOE"


class TestNormalizeDocnum:
    def test_alphanumeric_only(self):
        assert normalize_docnum(" 12-345 678/a ") == "12345678A"


class TestExtractFields:
    def test_labelled_kenyan_id_layout(self):
        words = words_from_lines(
            "REPUBLIC OF KENYA",
            "NATIONAL IDENTITY CARD",
            "FULL NAMES JANE WANJIKU MWANGI",
            "ID NUMBER 23456789",
            "DATE OF BIRTH 14.02.1990",
        )
        fields = extract_fields(words)
        assert fields["fullName"].value == "JANE WANJIKU MWANGI"
        assert fields["documentNumber"].value == "23456789"
        assert fields["dateOfBirth"].value == "1990-02-14"
        for f in fields.values():
            assert 0.0 < f.confidence <= 1.0

    def test_label_value_on_next_line(self):
        words = words_from_lines(
            "FULL NAMES",
            "JOHN KAMAU NJOROGE",
            "ID NUMBER",
            "11223344",
        )
        fields = extract_fields(words)
        assert fields["fullName"].value == "JOHN KAMAU NJOROGE"
        assert fields["documentNumber"].value == "11223344"

    def test_confidence_reflects_ocr_confidence(self):
        low = extract_fields(
            words_from_lines("FULL NAMES JANE MWANGI", conf=0.4)
        )
        high = extract_fields(
            words_from_lines("FULL NAMES JANE MWANGI", conf=0.95)
        )
        assert low["fullName"].confidence < high["fullName"].confidence
        assert abs(high["fullName"].confidence - 0.95) < 1e-6

    def test_unlabelled_layout_falls_back_with_discount(self):
        words = words_from_lines(
            "REPUBLIC OF KENYA",
            "JANE WANJIKU MWANGI",
            "23456789",
            conf=0.9,
        )
        fields = extract_fields(words)
        assert fields["fullName"].value == "JANE WANJIKU MWANGI"
        assert fields["fullName"].confidence < 0.9  # fallback is discounted
        assert fields["documentNumber"].value == "23456789"

    def test_spaced_document_number(self):
        words = words_from_lines("ID NUMBER 234 567 89")
        assert extract_fields(words)["documentNumber"].value == "23456789"

    def test_empty_input(self):
        assert extract_fields([]) == {}


class TestMergeMrz:
    TD3 = [
        "P<UTOERIKSSON<<ANNA<MARIA<<<<<<<<<<<<<<<<<<<",
        "L898902C36UTO7408122F1204159ZE184226B<<<<<10",
    ]

    def test_valid_mrz_overrides_visual_zone(self):
        fields = {"fullName": FieldValue("ANNA ERIKSSON", 0.5)}
        merged = merge_mrz(fields, parse_mrz(self.TD3))
        assert merged["fullName"].value == "ANNA MARIA ERIKSSON"
        assert merged["fullName"].confidence == 0.99
        assert merged["documentNumber"].value == "L898902C3"
        assert merged["dateOfBirth"].value == "1974-08-12"

    def test_invalid_mrz_is_ignored(self):
        bad = [self.TD3[0], self.TD3[1][:9] + "7" + self.TD3[1][10:]]
        fields = {"fullName": FieldValue("ANNA ERIKSSON", 0.5)}
        merged = merge_mrz(fields, parse_mrz(bad))
        assert merged == fields

    def test_no_mrz_is_noop(self):
        fields = {"fullName": FieldValue("ANNA ERIKSSON", 0.5)}
        assert merge_mrz(fields, None) == fields
