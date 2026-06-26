"""
Audit-readiness regression tests.

Locks in the audit controls shipped 2026-06-26:
  - tamper-evident (hash-chained, append-only) audit trail on 9 services, each
    with a `.../audit-log/verify` endpoint returning {intact, brokenSeq?, total}
  - audit-log read endpoints on the newly-covered services
  - PAR / ageing report and IFRS 9 ECL provisioning report (loan-management)

Runs against the live services (localhost:28xxx via port-forwards).
"""
import os
import requests
import pytest

BASE = os.getenv("LMS_BASE", "http://localhost")
T = int(os.getenv("LMS_TIMEOUT", "20"))

VERIFY_ENDPOINTS = {
    "account":     f"{BASE}:28086/api/v1/audit-log/verify",
    "loans":       f"{BASE}:28089/api/v1/audit-log/verify",
    "accounting":  f"{BASE}:28091/api/v1/accounting/audit-log/verify",
    "overdraft":   f"{BASE}:28097/api/v1/overdraft/audit/verify",
    "fraud":       f"{BASE}:28100/api/v1/fraud/audit/verify",
    "payment":     f"{BASE}:28090/api/v1/audit-log/verify",
    "collections": f"{BASE}:28093/api/v1/audit-log/verify",
    "float":       f"{BASE}:28092/api/v1/audit-log/verify",
    "product":     f"{BASE}:28087/api/v1/audit-log/verify",
}

# Services exposing a plain GET /api/v1/audit-log read endpoint.
READ_ENDPOINTS = {
    "account":     f"{BASE}:28086/api/v1/audit-log",
    "payment":     f"{BASE}:28090/api/v1/audit-log",
    "collections": f"{BASE}:28093/api/v1/audit-log",
    "float":       f"{BASE}:28092/api/v1/audit-log",
    "product":     f"{BASE}:28087/api/v1/audit-log",
}


@pytest.fixture(scope="module")
def headers():
    r = requests.post(f"{BASE}:28086/api/auth/login",
                      json={"username": "admin", "password": "admin123"}, timeout=T)
    assert r.status_code == 200, f"login failed: {r.status_code} {r.text[:120]}"
    return {"Content-Type": "application/json",
            "Authorization": f"Bearer {r.json()['token']}"}


@pytest.mark.parametrize("svc,u", list(VERIFY_ENDPOINTS.items()), ids=list(VERIFY_ENDPOINTS))
def test_audit_chain_intact(svc, u, headers):
    """Every tamper-evident audit chain verifies intact."""
    r = requests.get(u, headers=headers, timeout=T)
    assert r.status_code == 200, f"{svc} verify: {r.status_code} {r.text[:120]}"
    body = r.json()
    assert body.get("intact") is True, f"{svc} chain NOT intact: {body}"
    assert body.get("brokenSeq") in (None, 0), f"{svc} reported a broken seq: {body}"
    assert isinstance(body.get("total"), int)


def test_audit_grows_and_stays_intact(headers):
    """An audited money operation extends the chain and keeps it intact."""
    before = requests.get(VERIFY_ENDPOINTS["account"], headers=headers, timeout=T).json()
    assert before["intact"] is True
    n0 = before["total"]

    cid = f"AUDITTEST-{os.urandom(4).hex()}"
    rc = requests.post(f"{BASE}:28086/api/v1/customers", headers=headers,
                       json={"customerId": cid, "firstName": "Audit", "lastName": "Test",
                             "email": f"{cid.lower()}@test.local", "phone": "+254700000123",
                             "customerType": "INDIVIDUAL", "status": "ACTIVE"}, timeout=T)
    assert rc.status_code == 201, f"customer create: {rc.status_code} {rc.text[:120]}"
    acct = requests.post(f"{BASE}:28086/api/v1/accounts", headers=headers,
                         json={"customerId": cid, "accountType": "SAVINGS",
                               "currency": "KES", "name": "audit test"}, timeout=T).json()["id"]
    # sub-threshold credit → executes immediately and is audit-logged
    rcr = requests.post(f"{BASE}:28086/api/v1/accounts/{acct}/credit", headers=headers,
                        json={"amount": 500, "description": "audit chain test",
                              "reference": cid}, timeout=T)
    assert rcr.status_code == 200, f"credit: {rcr.status_code} {rcr.text[:120]}"

    after = requests.get(VERIFY_ENDPOINTS["account"], headers=headers, timeout=T).json()
    assert after["intact"] is True, f"chain broke after a credit: {after}"
    assert after["total"] > n0, f"audit chain did not grow ({n0} -> {after['total']})"


@pytest.mark.parametrize("svc,u", list(READ_ENDPOINTS.items()), ids=list(READ_ENDPOINTS))
def test_audit_log_readable(svc, u, headers):
    """Audit-log read endpoints return a list/paged structure."""
    r = requests.get(u, headers=headers, params={"size": 5}, timeout=T)
    assert r.status_code == 200, f"{svc} audit-log read: {r.status_code} {r.text[:120]}"
    body = r.json()
    assert isinstance(body, (list, dict)), f"{svc} unexpected audit-log shape"


def test_par_report(headers):
    r = requests.get(f"{BASE}:28089/api/v1/loans/par-report", headers=headers, timeout=T)
    assert r.status_code == 200, f"par-report: {r.status_code} {r.text[:120]}"
    b = r.json()
    buckets = {x["bucket"] for x in b["buckets"]}
    assert {"Current", "1-30", "31-60", "61-90", "90+"} <= buckets
    for k in ("par1", "par30", "par60", "par90"):
        assert isinstance(b[k], (int, float))
    assert "totalOutstanding" in b


def test_ecl_provision_report(headers):
    r = requests.get(f"{BASE}:28089/api/v1/loans/ecl-provision", headers=headers, timeout=T)
    assert r.status_code == 200, f"ecl-provision: {r.status_code} {r.text[:120]}"
    b = r.json()
    assert len(b["stages"]) == 3
    assert isinstance(b["coverageRatio"], (int, float))
    # provision == round(grossOutstanding * provisionRate, 2) for populated stages
    for s in b["stages"]:
        gross = float(s["grossOutstanding"])
        rate = float(s["provisionRate"])
        prov = float(s["provision"])
        assert abs(prov - round(gross * rate, 2)) < 0.01, f"stage {s['stage']} provision mismatch: {s}"
