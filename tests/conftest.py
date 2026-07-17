"""
AthenaLMS E2E Test Suite — Shared Fixtures & Helpers
"""
import os
import time
import uuid
import pytest
import requests

# ---------------------------------------------------------------------------
# Base URLs — override via env vars when running outside Docker host
# ---------------------------------------------------------------------------
BASE = os.getenv("LMS_BASE", "http://localhost")

SERVICES = {
    "account":          f"{BASE}:28086",
    "product":          f"{BASE}:28087",
    "loan_origination": f"{BASE}:28088",
    "loan_management":  f"{BASE}:28089",
    "payment":          f"{BASE}:28090",
    "accounting":       f"{BASE}:28091",
    "float":            f"{BASE}:28092",
    "collections":      f"{BASE}:28093",
    "compliance":       f"{BASE}:28094",
    "reporting":        f"{BASE}:28095",
    "scoring":          f"{BASE}:28096",
    "overdraft":        f"{BASE}:28097",
    "media":            f"{BASE}:28098",
    "notification":     f"{BASE}:28099",
    "gateway":          f"{BASE}:28105",
    "fraud":            f"{BASE}:28100",
    "fraud_ml":         f"{BASE}:18101",
    "decision":         f"{BASE}:28106",
}

SERVICE_KEY = os.getenv(
    "LMS_SERVICE_KEY",
    "athena-internal-key",
)

DEMO_USERS = {
    "admin":   {"username": "admin",              "password": "admin123"},
    "manager": {"username": "manager",            "password": "manager123"},
    "officer": {"username": "officer",            "password": "officer123"},
    "teller":  {"username": "teller@athena.com",  "password": "teller123"},
}

TIMEOUT = int(os.getenv("LMS_TIMEOUT", "15"))


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
def url(service: str, path: str = "") -> str:
    return f"{SERVICES[service]}{path}"


def service_headers() -> dict:
    return {
        "Content-Type": "application/json",
        "X-Service-Key": SERVICE_KEY,
        "X-Service-Tenant": "admin",
        "X-Service-User": "pytest-runner",
    }


def unique_id(prefix: str = "TST") -> str:
    return f"{prefix}-{uuid.uuid4().hex[:8].upper()}"


def wait_for(fn, *, retries=10, delay=2, desc="condition"):
    """Poll *fn* until it returns a truthy value or retries exhausted."""
    for attempt in range(1, retries + 1):
        result = fn()
        if result:
            return result
        time.sleep(delay)
    pytest.fail(f"Timed out waiting for {desc} after {retries * delay}s")


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------
@pytest.fixture(scope="session")
def admin_token():
    """Login as admin and return JWT token (cached for entire session)."""
    r = requests.post(
        url("account", "/api/auth/login"),
        json=DEMO_USERS["admin"],
        timeout=TIMEOUT,
    )
    assert r.status_code == 200, f"Admin login failed: {r.status_code} {r.text}"
    token = r.json().get("token")
    assert token, "No token in login response"
    return token


@pytest.fixture(scope="session")
def admin_headers(admin_token):
    """Auth headers for admin user."""
    return {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {admin_token}",
    }


@pytest.fixture(scope="session")
def svc_headers():
    """Service-key auth headers."""
    return service_headers()


@pytest.fixture(scope="session", autouse=True)
def disable_maker_checker(admin_token):
    """Functional regression tests exercise core operations directly. Dual
    control (maker-checker / segregation of duties) is a separate concern with
    its own coverage, so disable it for the test session — otherwise large
    credits/transfers are queued (HTTP 202) and single-operator loan approval
    is blocked (SoD 422). Sensible defaults are restored on teardown."""
    hdr = {"Content-Type": "application/json", "Authorization": f"Bearer {admin_token}"}
    acct_ops = ["ACCOUNT_CREDIT", "ACCOUNT_DEBIT", "TRANSFER", "ACCOUNT_CLOSE"]
    loan_ops = ["LOAN_APPROVE", "LOAN_DISBURSE"]

    def _set(enabled, thresholds):
        for op in acct_ops:
            try:
                requests.put(url("account", "/api/v1/control-config"), headers=hdr,
                             json={"operation": op, "enabled": enabled,
                                   "threshold": thresholds.get(op, 0)}, timeout=TIMEOUT)
            except requests.RequestException:
                pass
        for op in loan_ops:
            try:
                requests.put(url("loan_origination", "/api/v1/control-config"), headers=hdr,
                             json={"operation": op, "enabled": enabled, "threshold": 0}, timeout=TIMEOUT)
            except requests.RequestException:
                pass

    _set(False, {})
    yield
    _set(True, {"ACCOUNT_CREDIT": 100000, "ACCOUNT_DEBIT": 100000, "TRANSFER": 100000})


@pytest.fixture(scope="session")
def test_customer(admin_headers):
    """Create a reusable test customer and return its data."""
    cid = unique_id("CUST")
    payload = {
        "customerId": cid,
        "firstName": "Pytest",
        "lastName": "Runner",
        "email": f"{cid.lower()}@test.athena.com",
        "phone": "+254700000000",
        "customerType": "INDIVIDUAL",
        "status": "ACTIVE",
    }
    r = requests.post(
        url("account", "/api/v1/customers"),
        json=payload,
        headers=admin_headers,
        timeout=TIMEOUT,
    )
    assert r.status_code == 201, f"Customer create failed: {r.status_code} {r.text}"
    data = r.json()
    data["_customerId"] = cid
    return data


@pytest.fixture(scope="session")
def test_account(admin_headers, test_customer):
    """Create a SAVINGS account seeded with 100 000 KES."""
    payload = {
        "customerId": test_customer["_customerId"],
        "accountType": "SAVINGS",
        "currency": "KES",
        "name": "Pytest Savings",
    }
    r = requests.post(
        url("account", "/api/v1/accounts"),
        json=payload,
        headers=admin_headers,
        timeout=TIMEOUT,
    )
    assert r.status_code == 201, f"Account create failed: {r.status_code} {r.text}"
    acct = r.json()

    # Seed balance
    r2 = requests.post(
        url("account", f"/api/v1/accounts/{acct['id']}/credit"),
        json={"amount": 100000, "description": "Test seed", "reference": unique_id("SEED")},
        headers=admin_headers,
        timeout=TIMEOUT,
    )
    assert r2.status_code == 200, f"Credit failed: {r2.status_code} {r2.text}"
    acct["_customerId"] = test_customer["_customerId"]
    return acct


# ---------------------------------------------------------------------------
# Report metadata
# ---------------------------------------------------------------------------
def pytest_html_report_title(report):
    report.title = "AthenaLMS E2E Test Report"


def pytest_configure(config):
    config._metadata = {
        "Project": "AthenaIntelligentLMS",
        "Services": "15 microservices",
        "Base URL": BASE,
    }
