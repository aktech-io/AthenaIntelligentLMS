"""Profile-dispatch tests (pure logic, no tesseract)."""
from engine.fields import OcrWord
from engine.mrz import check_digit
from engine.profiles import extract_with_profile, normalize_fan


def words_from_lines(*lines, conf=0.9):
    out = []
    for i, line in enumerate(lines):
        for tok in line.split():
            out.append(OcrWord(text=tok, confidence=conf, line=i))
    return out


def make_td3(
    doc="B1234567",
    country="KEN",
    surname="MWANGI",
    given="JANE<WANJIKU",
    nationality="KEN",
    birth="900214",
    sex="F",
    expiry="300101",
    personal="",
):
    """Construct a TD3 MRZ with correct ICAO 9303 check digits."""
    l1 = (f"P<{country}{surname}<<{given}").ljust(44, "<")[:44]
    docf = doc.ljust(9, "<")
    persf = personal.ljust(14, "<")
    l2 = (
        docf + str(check_digit(docf))
        + nationality
        + birth + str(check_digit(birth))
        + sex
        + expiry + str(check_digit(expiry))
        + persf + str(check_digit(persf))
    )
    l2 += str(check_digit(l2[0:10] + l2[13:20] + l2[21:43]))
    assert len(l1) == 44 and len(l2) == 44
    return l1, l2


KE_ID_LINES = (
    "REPUBLIC OF KENYA",
    "NATIONAL IDENTITY CARD",
    "FULL NAMES JANE WANJIKU MWANGI",
    "ID NUMBER 23456789",
    "DATE OF BIRTH 14.02.1990",
)


class TestDispatch:
    def test_missing_profile_is_default_behaviour(self):
        words = words_from_lines(*KE_ID_LINES)
        r = extract_with_profile(words, "\n".join(KE_ID_LINES), None)
        assert r.fields["fullName"].value == "JANE WANJIKU MWANGI"
        assert r.fields["documentNumber"].value == "23456789"
        assert r.fields["dateOfBirth"].value == "1990-02-14"
        assert r.document_type_detected == ""

    def test_unknown_profile_falls_back_to_default(self):
        words = words_from_lines(*KE_ID_LINES)
        text = "\n".join(KE_ID_LINES)
        assert extract_with_profile(words, text, "ug-national-id") == \
            extract_with_profile(words, text, None)

    def test_ke_national_id_is_alias_for_default(self):
        words = words_from_lines(*KE_ID_LINES)
        text = "\n".join(KE_ID_LINES)
        assert extract_with_profile(words, text, "ke-national-id") == \
            extract_with_profile(words, text, None)


class TestPassportMrz:
    def test_valid_td3_mrz_values_are_the_result(self):
        l1, l2 = make_td3()
        # visual zone disagrees on purpose — the checksummed MRZ must win
        words = words_from_lines("SURNAME WRONG NAME", "PASSPORT NO X9999999")
        r = extract_with_profile(words, l1 + "\n" + l2, "passport-mrz")
        assert r.mrz is not None and r.mrz.valid
        assert r.fields["fullName"].value == "JANE WANJIKU MWANGI"
        assert r.fields["documentNumber"].value == "B1234567"
        assert r.fields["dateOfBirth"].value == "1990-02-14"
        assert r.fields["expiryDate"].value == "2030-01-01"
        assert r.fields["nationality"].value == "KEN"
        for k in ("fullName", "documentNumber", "dateOfBirth", "expiryDate"):
            assert r.fields[k].confidence == 0.99
        assert r.document_type_detected == "P"

    def test_visual_zone_fills_only_what_mrz_lacks(self):
        # birth month 00 -> MRZ dateOfBirth unparseable ('' with valid checks)
        l1, l2 = make_td3(birth="900014")
        words = words_from_lines("DATE OF BIRTH 14.02.1990")
        r = extract_with_profile(words, l1 + "\n" + l2, "passport-mrz")
        assert r.mrz is not None and r.mrz.valid
        assert r.fields["documentNumber"].value == "B1234567"  # MRZ kept
        assert r.fields["dateOfBirth"].value == "1990-02-14"  # visual fill
        assert r.fields["dateOfBirth"].confidence < 0.99

    def test_no_mrz_means_low_confidence_not_ke_fallback(self):
        # A KE-ID-looking visual zone must NOT be label-extracted here.
        words = words_from_lines(*KE_ID_LINES)
        r = extract_with_profile(words, "\n".join(KE_ID_LINES), "passport-mrz")
        assert r.mrz is None
        assert r.fields == {}
        # sanity: the same input under the default profile does extract
        assert extract_with_profile(
            words, "\n".join(KE_ID_LINES), None
        ).fields != {}

    def test_invalid_check_digits_yield_low_confidence(self):
        l1, l2 = make_td3()
        bad = l2[:9] + str((int(l2[9]) + 1) % 10) + l2[10:]
        r = extract_with_profile([], l1 + "\n" + bad, "passport-mrz")
        assert r.mrz is not None and not r.mrz.valid
        assert r.fields  # kept for officer review ...
        for f in r.fields.values():
            assert f.confidence < 0.5  # ... but never verification-grade


class TestEtFayda:
    FAYDA_LINES = (
        "FEDERAL DEMOCRATIC REPUBLIC OF ETHIOPIA",
        "NATIONAL ID",
        "Full Name ABEBE KEBEDE BEKELE",
        "FAN 1234 5678 9012 3456",
        "Date of Birth 12.03.1995",
        "Sex M",
    )

    def test_spaced_fan_groups_are_normalized(self):
        words = words_from_lines(*self.FAYDA_LINES)
        r = extract_with_profile(words, "\n".join(self.FAYDA_LINES), "et-fayda")
        assert r.fields["documentNumber"].value == "1234567890123456"
        assert r.fields["fullName"].value == "ABEBE KEBEDE BEKELE"
        assert r.fields["dateOfBirth"].value == "1995-03-12"
        assert r.fields["sex"].value == "M"
        for f in r.fields.values():
            assert 0.0 < f.confidence <= 1.0

    def test_twelve_digit_number_is_rejected(self):
        lines = (
            "Full Name ABEBE KEBEDE BEKELE",
            "FAN 1234 5678 9012",  # FIN-length, not the 16-digit FAN
        )
        r = extract_with_profile(words_from_lines(*lines), "\n".join(lines), "et-fayda")
        assert "documentNumber" not in r.fields
        assert r.fields["fullName"].value == "ABEBE KEBEDE BEKELE"

    def test_unlabelled_fan_found_by_group_pattern_discounted(self):
        lines = ("Full Name ABEBE KEBEDE BEKELE", "1234-5678-9012-3456")
        r = extract_with_profile(
            words_from_lines(*lines, conf=0.9), "\n".join(lines), "et-fayda"
        )
        assert r.fields["documentNumber"].value == "1234567890123456"
        assert r.fields["documentNumber"].confidence < 0.9

    def test_no_ke_style_positional_name_guess(self):
        # Without a Full Name/Name anchor the Fayda profile stays silent
        # instead of guessing the longest letter line like the default does.
        lines = ("ADDIS ABABA BOLE SUBCITY", "FAN 1234 5678 9012 3456")
        r = extract_with_profile(words_from_lines(*lines), "\n".join(lines), "et-fayda")
        assert "fullName" not in r.fields
        assert r.fields["documentNumber"].value == "1234567890123456"


class TestNormalizeFan:
    def test_spaces_and_dashes(self):
        assert normalize_fan("1234 5678 9012 3456") == "1234567890123456"
        assert normalize_fan("1234-5678-9012-3456") == "1234567890123456"
        assert normalize_fan(" 1234567890123456 ") == "1234567890123456"
