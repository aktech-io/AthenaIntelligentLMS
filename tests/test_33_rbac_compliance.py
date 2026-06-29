"""
RBAC on compliance regulated decisions (SAR filing, AML alert resolution,
KYC pass/fail) — see go-services/internal/compliance/handler/handler.go.

The role guard (auth.RequireRole) runs as chi middleware *before* the handler,
so a forbidden role is rejected with 403 even against a non-existent resource
id, while an allowed role passes the gate (and then gets a 4xx for the fake id).
This lets us assert the authorization boundary without seeding data.

Gated to ADMIN/MANAGER:
  POST /compliance/alerts/{id}/resolve
  POST /compliance/alerts/{id}/sar
  POST /compliance/kyc/{customerId}/pass
  POST /compliance/kyc/{customerId}/fail
Open to all staff (data entry / reads):
  POST /compliance/kyc, GET /compliance/**
"""
import uuid
import pytest
import requests

from conftest import url, DEMO_USERS, TIMEOUT


def _login(role: str) -> str:
    r = requests.post(
        url("account", "/api/auth/login"), json=DEMO_USERS[role], timeout=TIMEOUT
    )
    assert r.status_code == 200, f"{role} login failed: {r.status_code} {r.text}"
    tok = r.json().get("token")
    assert tok, f"no token for {role}"
    return tok


def _hdr(token: str) -> dict:
    return {"Content-Type": "application/json", "Authorization": f"Bearer {token}"}


@pytest.fixture(scope="module")
def officer_h():
    return _hdr(_login("officer"))


@pytest.fixture(scope="module")
def manager_h():
    return _hdr(_login("manager"))


# Gated decision endpoints, keyed by a fake resource id.
GATED = [
    ("POST", lambda: f"/api/v1/compliance/alerts/{uuid.uuid4()}/resolve"),
    ("POST", lambda: f"/api/v1/compliance/alerts/{uuid.uuid4()}/sar"),
    ("POST", lambda: f"/api/v1/compliance/kyc/CUST-{uuid.uuid4().hex[:8]}/pass"),
    ("POST", lambda: f"/api/v1/compliance/kyc/CUST-{uuid.uuid4().hex[:8]}/fail"),
]


@pytest.mark.parametrize("method,path_fn", GATED)
def test_officer_forbidden_on_decisions(officer_h, method, path_fn):
    """OFFICER must be rejected (403) on every regulated compliance decision."""
    r = requests.request(
        method, url("compliance", path_fn()), headers=officer_h, json={}, timeout=TIMEOUT
    )
    assert r.status_code == 403, (
        f"OFFICER should be forbidden on {path_fn()}, got {r.status_code}: {r.text}"
    )


@pytest.mark.parametrize("method,path_fn", GATED)
def test_manager_passes_gate_on_decisions(manager_h, method, path_fn):
    """MANAGER must pass the role gate (i.e. NOT 403). The fake id then yields a
    4xx from the handler, which still proves authorization succeeded."""
    r = requests.request(
        method, url("compliance", path_fn()), headers=manager_h, json={}, timeout=TIMEOUT
    )
    assert r.status_code != 403, (
        f"MANAGER should pass the gate on {path_fn()}, got 403: {r.text}"
    )


def test_officer_can_read(officer_h):
    """Reads stay open to staff."""
    r = requests.get(url("compliance", "/api/v1/compliance/summary"), headers=officer_h, timeout=TIMEOUT)
    assert r.status_code == 200, f"OFFICER read blocked: {r.status_code} {r.text}"


def test_unauthenticated_rejected():
    """No token at all is rejected (401), never silently allowed."""
    r = requests.post(
        url("compliance", f"/api/v1/compliance/alerts/{uuid.uuid4()}/sar"),
        json={}, timeout=TIMEOUT,
    )
    assert r.status_code == 401, f"expected 401 unauthenticated, got {r.status_code}"
