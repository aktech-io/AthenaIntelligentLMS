"""
Nemo WS-1/WS-4 — Document taxonomy on onboarding submissions (docs/nemo/07).

The compliance service accepts a `documentType` on the self-service
onboarding submission and validates it against the market pack's
kycDocuments (KE pack: NATIONAL_ID ^[0-9]{7,9}$, PASSPORT ^[A-Z][0-9]{6,7}$):

  - PASSPORT with a KE-pattern number persists with documentType=PASSPORT
    (no media attached, so the provider refers it — persistence and the
    echoed type are what's under test, not the verification outcome).
  - PASSPORT with a number that fails the pack's pattern -> 400.
  - A docType outside the market pack (DRIVING_LICENSE) -> 400.
  - No documentType at all -> legacy behaviour, NATIONAL_ID (backwards compat).

Tenant is resolved from auth, so the standard conftest admin token is the
whole tenancy story. Identity values are timestamp-suffixed per run: the
partial unique index uq_onboarding_open (tenant_id, national_id, open
statuses) would otherwise reject a re-run against the same environment.
"""
import time

import pytest
import requests
from conftest import url, DEMO_USERS, TIMEOUT

pytestmark = pytest.mark.compliance

# One suffix per run — keeps national ids / phones unique across runs while
# staying identical within the run (readable in the officer queue).
RUN = str(int(time.time()))


def _env_unavailable() -> str:
    """Empty string when the target is testable; otherwise the skip reason.

    Two distinct non-failures: the stack isn't running at all (local dev),
    or it is running but with rotated credentials (prod over the tunnel
    without LMS_ADMIN_PASSWORD set). Both skip rather than error."""
    try:
        r = requests.get(url("compliance", "/actuator/health"), timeout=5)
        if r.status_code != 200:
            return f"compliance service unhealthy ({r.status_code})"
        login = requests.post(url("account", "/api/auth/login"),
                              json=DEMO_USERS["admin"], timeout=5)
        if login.status_code != 200:
            return ("admin login rejected — set LMS_ADMIN_PASSWORD for this "
                    "environment")
        return ""
    except requests.RequestException:
        return "compliance service not reachable (start the stack or the SSH tunnel)"


_SKIP_REASON = _env_unavailable()
service_available = pytest.mark.skipif(bool(_SKIP_REASON), reason=_SKIP_REASON)


def _submit(headers, **overrides):
    payload = {
        "phone": "+2547" + RUN[-8:],
        "fullName": "Doc Type Pytest",
    }
    payload.update(overrides)
    return requests.post(url("compliance", "/api/v1/onboarding"),
                         json=payload, headers=headers, timeout=TIMEOUT)


@service_available
class TestOnboardingDocumentTypes:

    def test_passport_submission_persists_document_type(self, admin_headers):
        """PASSPORT + KE-passport-pattern number -> created, type persisted."""
        passport_no = "A" + RUN[-7:]  # ^[A-Z][0-9]{6,7}$ per the KE pack
        r = _submit(admin_headers,
                    nationalId=passport_no,
                    documentType="PASSPORT",
                    phone="+25471" + RUN[-7:])
        assert r.status_code == 201, f"submit: {r.status_code} {r.text}"
        app = r.json()
        assert app["documentType"] == "PASSPORT"
        assert app["nationalId"] == passport_no
        # No document/selfie media -> provider referral is the expected
        # outcome; what matters is the application persisted with its type.
        assert app["status"] in ("AUTO_APPROVED", "REFERRED")

        r2 = requests.get(url("compliance", f"/api/v1/onboarding/{app['id']}"),
                          headers=admin_headers, timeout=TIMEOUT)
        assert r2.status_code == 200, f"fetch: {r2.status_code} {r2.text}"
        assert r2.json()["documentType"] == "PASSPORT"

    def test_passport_with_invalid_number_rejected(self, admin_headers):
        """A number failing the pack's passport pattern is a 400."""
        r = _submit(admin_headers,
                    nationalId="123",
                    documentType="PASSPORT",
                    phone="+25472" + RUN[-7:])
        assert r.status_code == 400, f"expected 400, got {r.status_code} {r.text}"

    def test_document_type_outside_market_pack_rejected(self, admin_headers):
        """DRIVING_LICENSE is not in the KE pack's kycDocuments -> 400."""
        r = _submit(admin_headers,
                    nationalId="3" + RUN[-7:],
                    documentType="DRIVING_LICENSE",
                    phone="+25473" + RUN[-7:])
        assert r.status_code == 400, f"expected 400, got {r.status_code} {r.text}"

    def test_default_document_type_is_national_id(self, admin_headers):
        """No documentType -> legacy submissions still work as NATIONAL_ID."""
        r = _submit(admin_headers,
                    nationalId="4" + RUN[-7:],  # 8 digits, KE national-id pattern
                    phone="+25474" + RUN[-7:])
        assert r.status_code == 201, f"submit: {r.status_code} {r.text}"
        app = r.json()
        assert app["documentType"] == "NATIONAL_ID"
        assert app["status"] in ("AUTO_APPROVED", "REFERRED")
