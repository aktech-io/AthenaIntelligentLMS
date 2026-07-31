"""Parser tests for scripts/sync-screening-lists.py (repo root).

Pure stdlib, small inline fixtures — no network. The script lives outside
this package (it's an ops tool for any box with Python), so it is loaded by
file path; if the repo layout ever changes the tests skip rather than fail.
"""
import importlib.util
import os

import pytest

_SCRIPT = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
    "scripts",
    "sync-screening-lists.py",
)


def _load_module():
    if not os.path.isfile(_SCRIPT):
        pytest.skip(f"sync script not found at {_SCRIPT}")
    spec = importlib.util.spec_from_file_location("sync_screening_lists", _SCRIPT)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


sync = _load_module()


OFAC_FIXTURE = (
    '36,"AEROCARIBBEAN AIRLINES","-0-","CUBA","-0-","-0-","-0-","-0-","-0-",'
    '"-0-","-0-","a.k.a. \'AERO-CARIBBEAN\'."\n'
    '173,"ANGLO-CARIBBEAN CO., LTD.","-0-","CUBA","-0-","-0-","-0-","-0-",'
    '"-0-","-0-","-0-","-0-"\n'
    '540,"-0-","-0-","CUBA","-0-","-0-","-0-","-0-","-0-","-0-","-0-","-0-"\n'
)

UN_FIXTURE = b"""<?xml version="1.0" encoding="utf-8"?>
<CONSOLIDATED_LIST dateGenerated="2026-08-01">
  <INDIVIDUALS>
    <INDIVIDUAL>
      <DATAID>6908555</DATAID>
      <FIRST_NAME>RI</FIRST_NAME>
      <SECOND_NAME>WON HO</SECOND_NAME>
      <THIRD_NAME/>
      <UN_LIST_TYPE>DPRK</UN_LIST_TYPE>
      <REFERENCE_NUMBER>KPi.033</REFERENCE_NUMBER>
      <NATIONALITY><VALUE>DPRK</VALUE></NATIONALITY>
      <INDIVIDUAL_ALIAS><QUALITY>Good</QUALITY><ALIAS_NAME>Ri Won-ho</ALIAS_NAME></INDIVIDUAL_ALIAS>
      <INDIVIDUAL_ALIAS><QUALITY>Low</QUALITY><ALIAS_NAME/></INDIVIDUAL_ALIAS>
    </INDIVIDUAL>
  </INDIVIDUALS>
  <ENTITIES>
    <ENTITY>
      <DATAID>110000</DATAID>
      <FIRST_NAME>EXAMPLE TRADING COMPANY</FIRST_NAME>
      <UN_LIST_TYPE>Al-Qaida</UN_LIST_TYPE>
      <REFERENCE_NUMBER>QDe.001</REFERENCE_NUMBER>
      <ENTITY_ALIAS><QUALITY>a.k.a.</QUALITY><ALIAS_NAME>ETC Ltd</ALIAS_NAME></ENTITY_ALIAS>
    </ENTITY>
  </ENTITIES>
</CONSOLIDATED_LIST>
"""

EU_FIXTURE = b"""<?xml version="1.0" encoding="UTF-8"?>
<export xmlns="http://eu.europa.ec/fpi/fsd/export" generationDate="2026-08-01">
  <sanctionEntity logicalId="13">
    <regulation regulationType="regulation" programme="LBY">reg</regulation>
    <subjectType code="person" classificationCode="P"/>
    <nameAlias wholeName="Example Person" firstName="Example" lastName="Person"/>
    <nameAlias wholeName="E. Person"/>
    <nameAlias wholeName=""/>
    <citizenship countryIso2Code="LY" countryDescription="LIBYA"/>
    <citizenship countryIso2Code="00" countryDescription="UNKNOWN"/>
  </sanctionEntity>
  <sanctionEntity logicalId="14">
    <regulation programme="TERR">reg</regulation>
    <subjectType code="enterprise" classificationCode="E"/>
    <nameAlias wholeName="Example Org"/>
  </sanctionEntity>
  <sanctionEntity logicalId="15">
    <regulation programme="TERR">reg</regulation>
  </sanctionEntity>
</export>
"""


class TestOfacParser:
    def test_rows_and_aka_aliases(self):
        rows = sync.parse_ofac_csv(OFAC_FIXTURE, "OFAC-SDN")
        assert len(rows) == 2  # the "-0-" name row is dropped
        first = rows[0]
        assert first.entry_id == "OFAC-SDN-36"
        assert first.name == "AEROCARIBBEAN AIRLINES"
        assert first.aliases == ["AERO-CARIBBEAN"]
        assert first.program == "CUBA"
        assert rows[1].aliases == []

    def test_header_row_is_skipped(self):
        text = "ent_num,name,type,program\n" + OFAC_FIXTURE
        assert len(sync.parse_ofac_csv(text, "OFAC-CONS")) == 2

    def test_null_program_becomes_empty(self):
        text = '77,"SOME NAME","-0-","-0-"\n'
        (row,) = sync.parse_ofac_csv(text, "OFAC-SDN")
        assert row.program == ""

    def test_empty_input(self):
        assert sync.parse_ofac_csv("", "OFAC-SDN") == []


class TestUnParser:
    def test_individual_and_entity(self):
        rows = sync.parse_un_xml(UN_FIXTURE)
        assert len(rows) == 2
        person, entity = rows
        assert person.entry_id == "UN-KPi.033"
        assert person.name == "RI WON HO"  # empty THIRD_NAME skipped
        assert person.aliases == ["Ri Won-ho"]  # empty ALIAS_NAME skipped
        assert person.country == "DPRK"
        assert person.program == "DPRK"
        assert entity.entry_id == "UN-QDe.001"
        assert entity.name == "EXAMPLE TRADING COMPANY"
        assert entity.aliases == ["ETC Ltd"]

    def test_csv_shape_roundtrip(self):
        row = sync.parse_un_xml(UN_FIXTURE)[0]
        assert row.as_csv() == [
            "UN-KPi.033", "RI WON HO", "Ri Won-ho", "DPRK", "DPRK",
        ]


class TestEuParser:
    def test_namespaced_entities(self):
        rows = sync.parse_eu_xml(EU_FIXTURE)
        assert len(rows) == 2  # the nameless logicalId=15 entity is dropped
        person, org = rows
        assert person.entry_id == "EU-13"
        assert person.name == "Example Person"
        assert person.aliases == ["E. Person"]  # blank wholeName skipped
        assert person.country == "LIBYA"  # UNKNOWN filtered
        assert person.program == "LBY"
        assert org.name == "Example Org"
        assert org.aliases == []


class TestAtomicWrite:
    def test_writes_engine_compatible_csv(self, tmp_path):
        rows = sync.parse_un_xml(UN_FIXTURE)
        path = sync.write_csv_atomic(str(tmp_path), "sanctions_un.csv", rows)
        # The engine's own loader must accept the output shape.
        from engine.screening import _load_csv

        entries = _load_csv(path, "sanctions")
        assert len(entries) == 2
        assert entries[0].name == "RI WON HO"
        assert entries[0].aliases == ["Ri Won-ho"]
        # no temp droppings left behind
        assert sorted(p.name for p in tmp_path.iterdir()) == ["sanctions_un.csv"]

    def test_atomic_replace_keeps_previous_on_failure(self, tmp_path):
        target = tmp_path / "sanctions_un.csv"
        target.write_text("id,name,aliases,country,program\nX,Keep Me,,,\n")
        # a failing source never reaches write_csv_atomic: simulate via
        # _sync_source with a builder that raises
        ok = sync._sync_source(
            "UN Consolidated", "sanctions_un.csv", str(tmp_path), 1,
            lambda: (_ for _ in ()).throw(ValueError("download exploded")),
        )
        assert ok is False
        assert "Keep Me" in target.read_text()

    def test_min_rows_guard_refuses_tiny_lists(self, tmp_path, capsys):
        target = tmp_path / "sanctions_un.csv"
        target.write_text("id,name,aliases,country,program\nX,Keep Me,,,\n")
        rows = sync.parse_un_xml(UN_FIXTURE)  # only 2 rows
        ok = sync._sync_source(
            "UN Consolidated", "sanctions_un.csv", str(tmp_path), 50,
            lambda: rows,
        )
        assert ok is False
        assert "Keep Me" in target.read_text()
        assert "min-rows" in capsys.readouterr().err


class TestVerify:
    def test_fresh_files_pass(self, tmp_path):
        for name in ("sanctions_ofac.csv", "sanctions_un.csv"):
            (tmp_path / name).write_text(
                "id,name,aliases,country,program\nX,Someone,,,\n"
            )
        assert sync.verify(str(tmp_path), max_age_hours=48, expect_eu=False) == 0

    def test_missing_file_fails(self, tmp_path):
        (tmp_path / "sanctions_ofac.csv").write_text(
            "id,name,aliases,country,program\nX,Someone,,,\n"
        )
        assert sync.verify(str(tmp_path), max_age_hours=48, expect_eu=False) == 1

    def test_stale_file_fails(self, tmp_path):
        for name in ("sanctions_ofac.csv", "sanctions_un.csv"):
            p = tmp_path / name
            p.write_text("id,name,aliases,country,program\nX,Someone,,,\n")
            old = os.path.getmtime(p) - 72 * 3600
            os.utime(p, (old, old))
        assert sync.verify(str(tmp_path), max_age_hours=48, expect_eu=False) == 1

    def test_header_only_file_fails(self, tmp_path):
        for name in ("sanctions_ofac.csv", "sanctions_un.csv"):
            (tmp_path / name).write_text("id,name,aliases,country,program\n")
        assert sync.verify(str(tmp_path), max_age_hours=48, expect_eu=False) == 1

    def test_eu_checked_when_present(self, tmp_path):
        for name in ("sanctions_ofac.csv", "sanctions_un.csv"):
            (tmp_path / name).write_text(
                "id,name,aliases,country,program\nX,Someone,,,\n"
            )
        (tmp_path / "sanctions_eu.csv").write_text(
            "id,name,aliases,country,program\n"  # header only -> EMPTY
        )
        assert sync.verify(str(tmp_path), max_age_hours=48, expect_eu=False) == 1
