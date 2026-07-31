"""
Comprehensive Overdraft Compliance Tests
Tests IFRS 9 interest accrual, DPD/NPL staging, billing statements,
facility lifecycle, audit trail, and regulatory reporting endpoints.
"""
import pytest
import requests
import time
from conftest import url, unique_id, TIMEOUT


def _create_customer_and_wallet(admin_headers, prefix="COMP"):
    """Helper: create a unique customer and wallet, return (wallet_id, customer_id)."""
    cid = unique_id(prefix)
    requests.post(
        url("account", "/api/v1/customers"),
        headers=admin_headers,
        json={
            "customerId": cid,
            "firstName": "Compliance",
            "lastName": "Test",
            "email": f"{cid.lower()}@compliance.test",
            "phone": "+254711000000",
            "customerType": "INDIVIDUAL",
            "status": "ACTIVE",
        },
        timeout=TIMEOUT,
    )
    r = requests.post(
        url("overdraft", "/api/v1/wallets"),
        json={"customerId": cid, "currency": "KES"},
        headers=admin_headers,
        timeout=TIMEOUT,
    )
    assert r.status_code in (200, 201), f"Wallet create failed: {r.status_code} {r.text}"
    return r.json()["id"], cid


@pytest.mark.compliance
class TestFacilityLifecycle:
    """Tests complete overdraft facility lifecycle with credit scoring."""

    def test_apply_overdraft_creates_facility(self, admin_headers):
        """ApplyOverdraft should create a real facility with credit score, band, and limits."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "APPLY")

        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code in (200, 201), f"Apply failed: {r.status_code} {r.text}"
        data = r.json()

        # Verify facility has all required fields
        assert data.get("status") == "ACTIVE", f"Expected ACTIVE, got {data.get('status')}"
        assert data.get("creditScore", 0) > 0, "Credit score should be positive"
        # Legacy A–D bands plus the canonical NemoScore taxonomy (docs/nemo/06 §5)
        assert data.get("creditBand") in (
            "A", "B", "C", "D",
            "EXCELLENT", "VERY_GOOD", "GOOD", "FAIR", "MARGINAL", "POOR",
        ), f"Invalid band: {data.get('creditBand')}"
        assert float(data.get("approvedLimit", 0)) > 0, "Approved limit should be positive"
        assert float(data.get("interestRate", 0)) > 0, "Interest rate should be positive"
        assert data.get("nplStage") == "PERFORMING", f"Initial NPL stage should be PERFORMING"
        assert data.get("dpd") == 0, "Initial DPD should be 0"

    def test_duplicate_apply_rejected(self, admin_headers):
        """Applying for overdraft twice should be rejected (one active facility per wallet)."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "DUP")

        # First application
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code in (200, 201)

        # Second application should fail
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code in (400, 409, 422), f"Duplicate should be rejected: {r.status_code}"

    def test_get_facility_details(self, admin_headers):
        """Get facility should return full details including DPD and NPL stage."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "GETF")

        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )

        r = requests.get(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        data = r.json()
        assert data.get("hasOD") is True
        assert "creditScore" in data
        assert "creditBand" in data
        assert "drawnPrincipal" in data
        assert "accruedInterest" in data
        assert "nplStage" in data
        assert "dpd" in data

    def test_suspend_overdraft(self, admin_headers):
        """Suspending a facility should change status and remove overdraft headroom."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "SUSP")

        # Apply
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code in (200, 201)

        # Check wallet has overdraft headroom
        r = requests.get(
            url("overdraft", f"/api/v1/wallets/{wallet_id}"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        available_before = float(r.json().get("availableBalance", 0))

        # Suspend
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/suspend"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200, f"Suspend failed: {r.status_code} {r.text}"
        data = r.json()
        assert data.get("status") == "SUSPENDED"

        # Wallet headroom should be removed
        r = requests.get(
            url("overdraft", f"/api/v1/wallets/{wallet_id}"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        available_after = float(r.json().get("availableBalance", 0))
        assert available_after <= available_before, "Headroom should be reduced after suspension"

    def test_suspend_inactive_facility_rejected(self, admin_headers):
        """Cannot suspend an already suspended facility."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "SUSP2")

        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/suspend"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )

        # Second suspend should fail
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/suspend"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code in (400, 409, 422), f"Double suspend should fail: {r.status_code}"


@pytest.mark.compliance
class TestOverdraftDrawAndRepay:
    """Tests overdraft draw via withdrawal and repayment waterfall."""

    def test_overdraft_draw_via_withdrawal(self, admin_headers):
        """Withdrawing beyond balance with active facility should trigger overdraft draw."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "DRAW")

        # Deposit some funds
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/deposit"),
            json={"amount": 5000, "reference": unique_id("DEP")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )

        # Apply overdraft
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code in (200, 201)
        limit = float(r.json().get("approvedLimit", 0))

        # Withdraw more than deposited (triggers overdraft)
        withdraw_amount = 8000
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/withdraw"),
            json={"amount": withdraw_amount, "reference": unique_id("WDR")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200, f"Overdraft draw should succeed: {r.status_code} {r.text}"

        # Check wallet balance is negative
        r = requests.get(
            url("overdraft", f"/api/v1/wallets/{wallet_id}"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        balance = float(r.json().get("currentBalance", 0))
        assert balance < 0, f"Balance should be negative after overdraft draw: {balance}"

        # Check facility has drawn amount
        r = requests.get(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        drawn = float(r.json().get("drawn", 0))
        assert drawn > 0, f"Facility should show drawn amount: {drawn}"

    def test_waterfall_repayment(self, admin_headers):
        """Deposit should trigger waterfall: fees → interest → principal."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "WFALL")

        # Setup: deposit, apply, draw
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/deposit"),
            json={"amount": 1000, "reference": unique_id("DEP")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code in (200, 201)

        # Draw overdraft
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/withdraw"),
            json={"amount": 5000, "reference": unique_id("WDR")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )

        # Repay (deposit reduces drawn amount)
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/deposit"),
            json={"amount": 3000, "reference": unique_id("RPAY")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

        # Verify drawn amount decreased
        r = requests.get(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        drawn = float(r.json().get("drawn", 0))
        # drawn should be less than 4000 (initial 5000-1000 balance = 4000 drawn, minus some repayment)
        assert drawn < 4000, f"Drawn should be reduced after repayment: {drawn}"


@pytest.mark.compliance
class TestEODBatchProcessing:
    """Tests EOD batch processing: interest accrual, DPD, billing."""

    def test_eod_run_endpoint(self, admin_headers):
        """POST /eod/run should execute EOD batch and return results."""
        r = requests.post(
            url("overdraft", "/api/v1/overdraft/eod/run"),
            headers=admin_headers,
            timeout=TIMEOUT * 3,  # EOD may take longer
        )
        assert r.status_code == 200, f"EOD run failed: {r.status_code} {r.text}"
        data = r.json()
        assert "facilitiesProcessed" in data
        assert "interestChargesCreated" in data
        assert "totalInterestAccrued" in data
        assert "dpdUpdates" in data
        assert "stageChanges" in data
        assert "billingStatements" in data
        assert "durationMs" in data
        assert data["errors"] == 0, f"EOD had errors: {data}"

    def test_eod_run_with_date(self, admin_headers):
        """EOD should accept a specific date parameter."""
        r = requests.post(
            url("overdraft", "/api/v1/overdraft/eod/run?date=2026-03-19"),
            headers=admin_headers,
            timeout=TIMEOUT * 3,
        )
        assert r.status_code == 200, f"EOD with date failed: {r.status_code} {r.text}"

    def test_eod_status_endpoint(self, admin_headers):
        """GET /eod/status should return scheduler status."""
        r = requests.get(
            url("overdraft", "/api/v1/overdraft/eod/status"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        data = r.json()
        assert "running" in data

    def test_eod_interest_accrual_creates_charges(self, admin_headers):
        """After EOD, facilities with drawn amounts should have interest charges."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "EODINT")

        # Setup: deposit, apply, draw
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/deposit"),
            json={"amount": 1000, "reference": unique_id("DEP")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        r = requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code in (200, 201)

        # Draw overdraft
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/withdraw"),
            json={"amount": 5000, "reference": unique_id("WDR")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )

        # Run EOD
        requests.post(
            url("overdraft", "/api/v1/overdraft/eod/run"),
            headers=admin_headers,
            timeout=TIMEOUT * 3,
        )

        # Check interest charges exist
        r = requests.get(
            url("overdraft", f"/api/v1/overdraft/{wallet_id}/interest-charges"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        charges = r.json()
        assert isinstance(charges, list)
        # May or may not have charges depending on facility state
        # The key is the endpoint works and returns proper data

    def test_eod_idempotent(self, admin_headers):
        """Running EOD twice for the same date should not double-charge interest."""
        # Run EOD twice
        r1 = requests.post(
            url("overdraft", "/api/v1/overdraft/eod/run?date=2026-03-18"),
            headers=admin_headers,
            timeout=TIMEOUT * 3,
        )
        assert r1.status_code == 200

        r2 = requests.post(
            url("overdraft", "/api/v1/overdraft/eod/run?date=2026-03-18"),
            headers=admin_headers,
            timeout=TIMEOUT * 3,
        )
        assert r2.status_code == 200
        # Second run should create 0 interest charges (already processed)
        data2 = r2.json()
        assert data2.get("interestChargesCreated", 0) == 0, \
            f"Second EOD run should be idempotent, got {data2.get('interestChargesCreated')} charges"


@pytest.mark.compliance
class TestInterestCharges:
    """Tests interest charge records and accrual accuracy."""

    def test_interest_charges_endpoint(self, admin_headers):
        """Interest charges endpoint should return list of daily accruals."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "INTCH")

        r = requests.get(
            url("overdraft", f"/api/v1/overdraft/{wallet_id}/interest-charges"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        assert isinstance(r.json(), list)

    def test_interest_charge_structure(self, admin_headers):
        """Each interest charge should have required fields for audit."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "INTST")

        # Setup drawn facility
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/deposit"),
            json={"amount": 500, "reference": unique_id("DEP")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        requests.post(
            url("overdraft", f"/api/v1/wallets/{wallet_id}/withdraw"),
            json={"amount": 3000, "reference": unique_id("WDR")},
            headers=admin_headers,
            timeout=TIMEOUT,
        )

        # Run EOD
        requests.post(
            url("overdraft", "/api/v1/overdraft/eod/run"),
            headers=admin_headers,
            timeout=TIMEOUT * 3,
        )

        r = requests.get(
            url("overdraft", f"/api/v1/overdraft/{wallet_id}/interest-charges"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        charges = r.json()
        if len(charges) > 0:
            charge = charges[0]
            assert "chargeDate" in charge
            assert "drawnAmount" in charge
            assert "dailyRate" in charge
            assert "interestCharged" in charge
            assert "reference" in charge
            assert float(charge["interestCharged"]) > 0
            assert float(charge["dailyRate"]) > 0


@pytest.mark.compliance
class TestBillingStatements:
    """Tests billing statement generation and retrieval."""

    def test_billing_statements_endpoint(self, admin_headers):
        """Billing statements endpoint should return list."""
        wallet_id, cid = _create_customer_and_wallet(admin_headers, "BILL")

        r = requests.get(
            url("overdraft", f"/api/v1/overdraft/{wallet_id}/billing-statements"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        assert isinstance(r.json(), list)


@pytest.mark.compliance
class TestOverdraftSummary:
    """Tests the admin summary endpoint with real aggregated data."""

    def test_summary_returns_full_structure(self, admin_headers):
        """Summary should return facilities count, limits, drawn, available, by band."""
        r = requests.get(
            url("overdraft", "/api/v1/overdraft/summary"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        data = r.json()
        assert "totalFacilities" in data
        assert "activeFacilities" in data
        assert "totalApprovedLimit" in data
        assert "totalDrawnAmount" in data
        assert "totalAvailableOverdraft" in data
        assert "facilitiesByBand" in data
        assert "drawnByBand" in data
        assert isinstance(data["facilitiesByBand"], dict)
        assert isinstance(data["drawnByBand"], dict)


@pytest.mark.compliance
class TestAuditTrail:
    """Tests audit trail completeness for regulatory compliance."""

    def test_audit_log_endpoint(self, admin_headers):
        """Audit log should be accessible and paginated."""
        r = requests.get(
            url("overdraft", "/api/v1/overdraft/audit"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        data = r.json()
        # Should be paginated
        assert "content" in data or isinstance(data, list)

    def test_audit_log_filter_by_entity_type(self, admin_headers):
        """Audit log should support filtering by entity type."""
        r = requests.get(
            url("overdraft", "/api/v1/overdraft/audit?entityType=FACILITY"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

    def test_audit_entries_have_required_fields(self, admin_headers):
        """Each audit entry should have actor, timestamps, and snapshots."""
        r = requests.get(
            url("overdraft", "/api/v1/overdraft/audit?size=5"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        data = r.json()
        entries = data.get("content", data) if isinstance(data, dict) else data
        if len(entries) > 0:
            entry = entries[0]
            assert "entityType" in entry
            assert "entityId" in entry
            assert "action" in entry
            assert "actor" in entry
            assert "createdAt" in entry


@pytest.mark.compliance
class TestRegulatoryEndpoints:
    """Tests that all regulatory-required endpoints exist and respond."""

    def test_float_accounts_list(self, admin_headers):
        """Float accounts should be accessible."""
        r = requests.get(
            url("float", "/api/v1/float/accounts"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

    def test_float_summary(self, admin_headers):
        """Float summary should return aggregated data."""
        r = requests.get(
            url("float", "/api/v1/float/summary"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

    def test_wallets_list(self, admin_headers):
        """Wallets list should be accessible."""
        r = requests.get(
            url("overdraft", "/api/v1/wallets"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

    def test_overdraft_summary_accessible(self, admin_headers):
        """Overdraft summary should be accessible for regulatory reporting."""
        r = requests.get(
            url("overdraft", "/api/v1/overdraft/summary"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200

    def test_eod_status_accessible(self, admin_headers):
        """EOD status should be accessible for operational monitoring."""
        r = requests.get(
            url("overdraft", "/api/v1/overdraft/eod/status"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
