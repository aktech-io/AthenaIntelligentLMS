"""MRZ parsing + ICAO 9303 check-digit tests (pure logic, no OCR)."""
from engine.mrz import check_digit, parse_mrz, verify_check_digit

# ICAO Doc 9303 specimen passport (TD3)
TD3_L1 = "P<UTOERIKSSON<<ANNA<MARIA<<<<<<<<<<<<<<<<<<<"
TD3_L2 = "L898902C36UTO7408122F1204159ZE184226B<<<<<10"

# ICAO Doc 9303 specimen ID card (TD1)
TD1_L1 = "I<UTOD231458907<<<<<<<<<<<<<<<"
TD1_L2 = "7408122F1204159UTO<<<<<<<<<<<6"
TD1_L3 = "ERIKSSON<<ANNA<MARIA<<<<<<<<<<"


class TestCheckDigit:
    def test_icao_example_number(self):
        # Doc 9303 worked example: document number L898902C3 -> 6
        assert check_digit("L898902C3") == 6

    def test_date_check_digits(self):
        assert check_digit("740812") == 2
        assert check_digit("120415") == 9

    def test_filler_only_field_accepts_filler_digit(self):
        assert verify_check_digit("<<<<<<<<<<<<<<", "<")

    def test_filler_digit_rejected_for_populated_field(self):
        assert not verify_check_digit("ZE184226B<<<<<", "<")

    def test_wrong_digit_rejected(self):
        assert not verify_check_digit("L898902C3", "7")


class TestTD3:
    def test_specimen_parses_valid(self):
        r = parse_mrz([TD3_L1, TD3_L2])
        assert r is not None
        assert r.format == "TD3"
        assert r.valid, r.checks
        assert r.surname == "ERIKSSON"
        assert r.given_names == "ANNA MARIA"
        assert r.full_name == "ANNA MARIA ERIKSSON"
        assert r.document_number == "L898902C3"
        assert r.nationality == "UTO"
        assert r.date_of_birth == "1974-08-12"
        assert r.sex == "F"
        assert r.expiry_date == "2012-04-15"

    def test_corrupted_check_digit_flags_invalid(self):
        bad = TD3_L2[:9] + "7" + TD3_L2[10:]
        r = parse_mrz([TD3_L1, bad])
        assert r is not None
        assert not r.valid
        assert not r.checks["document_number"]

    def test_ocr_noise_lines_are_skipped(self):
        text = "REPUBLIC OF UTOPIA\nPASSPORT\n" + TD3_L1 + "\n" + TD3_L2 + "\n"
        r = parse_mrz(text)
        assert r is not None and r.valid

    def test_spaces_from_ocr_are_stripped(self):
        r = parse_mrz([TD3_L1.replace("<<", "< <", 1), TD3_L2])
        assert r is not None and r.valid


class TestTD1:
    def test_specimen_parses_valid(self):
        r = parse_mrz([TD1_L1, TD1_L2, TD1_L3])
        assert r is not None
        assert r.format == "TD1"
        assert r.valid, r.checks
        assert r.document_number == "D23145890"
        assert r.surname == "ERIKSSON"
        assert r.given_names == "ANNA MARIA"
        assert r.date_of_birth == "1974-08-12"
        assert r.expiry_date == "2012-04-15"
        assert r.nationality == "UTO"

    def test_corrupted_composite_flags_invalid(self):
        bad = TD1_L2[:29] + "0"
        r = parse_mrz([TD1_L1, bad, TD1_L3])
        assert r is not None
        assert not r.valid
        assert not r.checks["composite"]


class TestNoMrz:
    def test_plain_text_returns_none(self):
        assert parse_mrz("REPUBLIC OF KENYA\nNATIONAL IDENTITY CARD") is None

    def test_empty_returns_none(self):
        assert parse_mrz("") is None
