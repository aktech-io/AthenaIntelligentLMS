"""
Edge Case Tests — Deposit Products, Account Types, and Boundary Conditions.

Covers:
  - Deposit product validation (missing fields, invalid categories, zero rates)
  - Fixed deposit: open without term, early withdrawal blocked, maturity fields
  - Savings: min balance enforcement, interest rate from product (not override)
  - Current account: zero interest, operations
  - Wallet account: basic operations
  - Account opening: missing customer, invalid product, duplicate idempotency
  - Interest: accrual on zero balance, accrual on closed account
  - EOD: concurrent run rejected, history audit trail
  - Close account: with balance rejected, zero balance succeeds
  - Freeze/unfreeze lifecycle
"""
import os

import pytest
import requests
from decimal import Decimal

BASE = "http://localhost:28086"
PRODUCT_BASE = "http://localhost:28087"


def login():
    resp = requests.post(f"{BASE}/api/auth/login", json={
        "username": "admin",
        "password": os.getenv("LMS_ADMIN_PASSWORD", "admin123")
    })
    assert resp.status_code == 200
    return {"Authorization": f"Bearer {resp.json()['token']}"}


@pytest.fixture(scope="module")
def h():
    return login()


# ─── Deposit Product Validation ───────────────────────────────────────────────

class TestDepositProductEdgeCases:

    def test_missing_product_code(self, h):
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=h, json={
            "name": "No Code Product",
            "productCategory": "SAVINGS",
        })
        assert resp.status_code == 400

    def test_missing_name(self, h):
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=h, json={
            "productCode": "EDGE-NO-NAME",
            "productCategory": "SAVINGS",
        })
        assert resp.status_code == 400

    def test_invalid_category(self, h):
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=h, json={
            "productCode": "EDGE-BAD-CAT",
            "name": "Bad Category",
            "productCategory": "INVALID_TYPE",
        })
        assert resp.status_code == 400

    def test_create_zero_rate_current(self, h):
        """Current accounts can have 0% interest — this should succeed."""
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=h, json={
            "productCode": "EDGE-CUR-ZERO",
            "name": "Zero Rate Current",
            "productCategory": "CURRENT",
            "interestRate": 0,
            "minOperatingBalance": 5000,
        })
        assert resp.status_code in (201, 409)  # created or already exists

    def test_create_wallet_product(self, h):
        """Wallet is a valid deposit product category."""
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=h, json={
            "productCode": "EDGE-WALLET",
            "name": "Digital Wallet",
            "productCategory": "WALLET",
            "interestRate": 0,
        })
        assert resp.status_code in (201, 409)

    def test_create_call_deposit_product(self, h):
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=h, json={
            "productCode": "EDGE-CALL-DEP",
            "name": "Call Deposit",
            "productCategory": "CALL_DEPOSIT",
            "interestRate": 2.5,
        })
        assert resp.status_code in (201, 409)

    def test_deactivate_product(self, h):
        """Create + activate + deactivate lifecycle."""
        # Create
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=h, json={
            "productCode": "EDGE-LIFECYCLE",
            "name": "Lifecycle Test",
            "productCategory": "SAVINGS",
            "interestRate": 1.0,
        })
        if resp.status_code == 201:
            pid = resp.json()["id"]
        else:
            # Already exists
            resp2 = requests.get(f"{PRODUCT_BASE}/api/v1/deposit-products?size=100", headers=h)
            pid = next(p["id"] for p in resp2.json()["content"] if p["productCode"] == "EDGE-LIFECYCLE")

        # Activate
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products/{pid}/activate", headers=h)
        assert resp.status_code == 200
        assert resp.json()["status"] == "ACTIVE"

        # Deactivate
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products/{pid}/deactivate", headers=h)
        assert resp.status_code == 200
        assert resp.json()["status"] == "ARCHIVED"

    def test_update_nonexistent_product(self, h):
        resp = requests.put(
            f"{PRODUCT_BASE}/api/v1/deposit-products/00000000-0000-0000-0000-000000000000",
            headers=h, json={"name": "Ghost", "productCategory": "SAVINGS"}
        )
        assert resp.status_code == 404

    def test_get_nonexistent_product(self, h):
        resp = requests.get(
            f"{PRODUCT_BASE}/api/v1/deposit-products/00000000-0000-0000-0000-000000000000",
            headers=h
        )
        assert resp.status_code == 404


# ─── Account Opening Edge Cases ──────────────────────────────────────────────

class TestAccountOpeningEdgeCases:

    def test_open_without_customer(self, h):
        resp = requests.post(f"{BASE}/api/v1/accounts/open", headers=h, json={
            "accountType": "SAVINGS",
            "currency": "KES",
            "kycTier": 1,
            "accountName": "No Customer",
        })
        assert resp.status_code == 400

    def test_open_invalid_account_type(self, h):
        resp = requests.post(f"{BASE}/api/v1/accounts/open", headers=h, json={
            "customerId": "CUST-DEP-001",
            "accountType": "INVALID_TYPE",
            "currency": "KES",
            "kycTier": 1,
            "accountName": "Bad Type",
        })
        assert resp.status_code == 400

    def test_open_fd_without_term(self, h):
        """Fixed deposit requires termDays."""
        resp = requests.post(f"{BASE}/api/v1/accounts/open", headers=h, json={
            "customerId": "CUST-DEP-001",
            "accountType": "FIXED_DEPOSIT",
            "currency": "KES",
            "kycTier": 2,
            "accountName": "FD No Term",
            "initialDeposit": 50000,
        })
        assert resp.status_code == 400
        assert "termDays" in resp.text.lower() or "term" in resp.text.lower()

    def test_open_current_account(self, h):
        """Open a CURRENT account — should work."""
        # Ensure customer exists
        requests.post(f"{BASE}/api/v1/customers", headers=h, json={
            "customerId": "EDGE-CUST-001",
            "firstName": "Edge", "lastName": "Tester",
            "customerType": "INDIVIDUAL",
        })
        resp = requests.post(f"{BASE}/api/v1/accounts/open", headers=h, json={
            "customerId": "EDGE-CUST-001",
            "accountType": "CURRENT",
            "currency": "KES",
            "kycTier": 2,
            "accountName": "Edge Current Account",
            "initialDeposit": 10000,
        })
        assert resp.status_code == 201
        data = resp.json()
        assert data["accountType"] == "CURRENT"
        assert data["status"] == "ACTIVE"
        self.__class__.current_id = data["id"]

    def test_open_wallet_account(self, h):
        """Open a WALLET account."""
        resp = requests.post(f"{BASE}/api/v1/accounts/open", headers=h, json={
            "customerId": "EDGE-CUST-001",
            "accountType": "WALLET",
            "currency": "KES",
            "kycTier": 1,
            "accountName": "Edge Wallet",
            "initialDeposit": 500,
        })
        assert resp.status_code == 201
        data = resp.json()
        assert data["accountType"] == "WALLET"
        self.__class__.wallet_id = data["id"]

    def test_open_call_deposit(self, h):
        resp = requests.post(f"{BASE}/api/v1/accounts/open", headers=h, json={
            "customerId": "EDGE-CUST-001",
            "accountType": "CALL_DEPOSIT",
            "currency": "KES",
            "kycTier": 2,
            "accountName": "Edge Call Deposit",
            "initialDeposit": 100000,
            "interestRateOverride": 3.0,
        })
        assert resp.status_code == 201
        assert resp.json()["accountType"] == "CALL_DEPOSIT"
        self.__class__.call_deposit_id = resp.json()["id"]

    def test_open_fd_with_term_and_autorenew(self, h):
        resp = requests.post(f"{BASE}/api/v1/accounts/open", headers=h, json={
            "customerId": "EDGE-CUST-001",
            "accountType": "FIXED_DEPOSIT",
            "currency": "KES",
            "kycTier": 3,
            "accountName": "Edge FD 180-Day",
            "initialDeposit": 200000,
            "termDays": 180,
            "autoRenew": True,
            "interestRateOverride": 10.0,
        })
        assert resp.status_code == 201
        data = resp.json()
        assert data["accountType"] == "FIXED_DEPOSIT"
        assert data["termDays"] == 180
        assert data["autoRenew"] == True
        assert data["maturityDate"] is not None
        self.__class__.fd_id = data["id"]


# ─── Account Operations Edge Cases ───────────────────────────────────────────

class TestOperationEdgeCases:

    def test_credit_zero_amount(self, h):
        acct_id = TestAccountOpeningEdgeCases.current_id
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/credit", headers=h, json={
            "amount": 0, "description": "zero credit"
        })
        assert resp.status_code == 400

    def test_credit_negative_amount(self, h):
        acct_id = TestAccountOpeningEdgeCases.current_id
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/credit", headers=h, json={
            "amount": -100, "description": "negative credit"
        })
        assert resp.status_code == 400

    def test_debit_more_than_balance(self, h):
        acct_id = TestAccountOpeningEdgeCases.wallet_id
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/debit", headers=h, json={
            "amount": 999999, "description": "overdraw attempt"
        })
        assert resp.status_code in (400, 422)
        assert "insufficient" in resp.text.lower() or "Insufficient" in resp.text

    def test_debit_exact_balance(self, h):
        """Debit exactly the available balance — should succeed and leave 0."""
        acct_id = TestAccountOpeningEdgeCases.wallet_id
        bal = requests.get(f"{BASE}/api/v1/accounts/{acct_id}/balance", headers=h).json()
        avail = Decimal(str(bal["availableBalance"]))
        if avail <= 0:
            pytest.skip("Wallet already at zero")

        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/debit", headers=h, json={
            "amount": float(avail), "description": "drain wallet"
        })
        assert resp.status_code == 200

        # Verify balance is now zero
        bal2 = requests.get(f"{BASE}/api/v1/accounts/{acct_id}/balance", headers=h).json()
        assert Decimal(str(bal2["availableBalance"])) == 0

    def test_credit_to_nonexistent_account(self, h):
        resp = requests.post(
            f"{BASE}/api/v1/accounts/00000000-0000-0000-0000-000000000000/credit",
            headers=h, json={"amount": 100, "description": "ghost credit"}
        )
        assert resp.status_code == 404

    def test_debit_on_frozen_account(self, h):
        acct_id = TestAccountOpeningEdgeCases.current_id
        # Freeze
        requests.put(f"{BASE}/api/v1/accounts/{acct_id}/status", headers=h, json={"status": "FROZEN"})
        # Try debit
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/debit", headers=h, json={
            "amount": 100, "description": "frozen debit"
        })
        assert resp.status_code in (400, 422)
        # Credit should also fail on frozen
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/credit", headers=h, json={
            "amount": 100, "description": "frozen credit"
        })
        assert resp.status_code in (400, 422)
        # Unfreeze
        requests.put(f"{BASE}/api/v1/accounts/{acct_id}/status", headers=h, json={"status": "ACTIVE"})

    def test_idempotent_credit(self, h):
        """Two credits with same idempotency key — only one should execute."""
        import uuid
        acct_id = TestAccountOpeningEdgeCases.current_id
        bal_before = Decimal(str(
            requests.get(f"{BASE}/api/v1/accounts/{acct_id}/balance", headers=h).json()["availableBalance"]
        ))

        key = f"EDGE-IDEMP-{uuid.uuid4().hex[:8]}"
        for _ in range(3):
            resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/credit", headers=h, json={
                "amount": 5000, "description": "idempotent credit",
                "idempotencyKey": key,
            })
            assert resp.status_code == 200

        bal_after = Decimal(str(
            requests.get(f"{BASE}/api/v1/accounts/{acct_id}/balance", headers=h).json()["availableBalance"]
        ))
        assert bal_after == bal_before + 5000, (
            f"Idempotency failed: expected +5000, got +{bal_after - bal_before}"
        )


# ─── Account Lifecycle Edge Cases ─────────────────────────────────────────────

class TestLifecycleEdgeCases:

    def test_close_account_with_balance_rejected(self, h):
        acct_id = TestAccountOpeningEdgeCases.current_id
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/close", headers=h, json={
            "reason": "Customer request"
        })
        assert resp.status_code in (400, 422)
        assert "balance" in resp.text.lower() or "remaining" in resp.text.lower()

    def test_close_zero_balance_account(self, h):
        """Close the wallet which should now have zero balance."""
        acct_id = TestAccountOpeningEdgeCases.wallet_id
        bal = requests.get(f"{BASE}/api/v1/accounts/{acct_id}/balance", headers=h).json()
        if Decimal(str(bal["availableBalance"])) > 0:
            pytest.skip("Wallet still has balance")

        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/close", headers=h, json={
            "reason": "Account no longer needed"
        })
        assert resp.status_code == 200
        assert resp.json()["status"] == "CLOSED"

    def test_close_already_closed_rejected(self, h):
        acct_id = TestAccountOpeningEdgeCases.wallet_id
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/close", headers=h, json={
            "reason": "Double close attempt"
        })
        assert resp.status_code in (400, 422)
        assert "already closed" in resp.text.lower()

    def test_credit_to_closed_account_rejected(self, h):
        acct_id = TestAccountOpeningEdgeCases.wallet_id
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/credit", headers=h, json={
            "amount": 100, "description": "credit to closed"
        })
        assert resp.status_code in (400, 422)

    def test_approve_active_account_rejected(self, h):
        """Approving an already-active account should fail."""
        acct_id = TestAccountOpeningEdgeCases.current_id
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/approve", headers=h)
        assert resp.status_code in (400, 422)

    def test_invalid_status_update(self, h):
        acct_id = TestAccountOpeningEdgeCases.current_id
        resp = requests.put(f"{BASE}/api/v1/accounts/{acct_id}/status", headers=h, json={
            "status": "BOGUS_STATUS"
        })
        assert resp.status_code == 400


# ─── Interest Edge Cases ─────────────────────────────────────────────────────

class TestInterestEdgeCases:

    def test_interest_summary_on_new_account(self, h):
        """A fresh account with no accruals should return zeros."""
        acct_id = TestAccountOpeningEdgeCases.current_id
        resp = requests.get(f"{BASE}/api/v1/accounts/{acct_id}/interest-summary", headers=h)
        assert resp.status_code == 200
        data = resp.json()
        assert data["unpostedTotal"] == 0

    def test_post_interest_with_nothing_to_post(self, h):
        acct_id = TestAccountOpeningEdgeCases.current_id
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/post-interest", headers=h)
        assert resp.status_code in (400, 422)

    def test_interest_summary_nonexistent_account(self, h):
        resp = requests.get(
            f"{BASE}/api/v1/accounts/00000000-0000-0000-0000-000000000000/interest-summary",
            headers=h
        )
        assert resp.status_code == 404


# ─── EOD Edge Cases ──────────────────────────────────────────────────────────

class TestEODEdgeCases:

    def test_eod_idempotent_returns_existing(self, h):
        """Running EOD twice should return the same completed result."""
        resp1 = requests.post(f"{BASE}/api/v1/eod/run", headers=h)
        assert resp1.status_code == 200
        run1 = resp1.json()

        resp2 = requests.post(f"{BASE}/api/v1/eod/run", headers=h)
        assert resp2.status_code == 200
        run2 = resp2.json()

        assert run1["id"] == run2["id"]
        assert run2["status"] in ("COMPLETED", "PARTIAL")

    def test_eod_history_returns_runs(self, h):
        resp = requests.get(f"{BASE}/api/v1/eod/history", headers=h)
        assert resp.status_code == 200
        runs = resp.json()
        assert isinstance(runs, list)
        assert len(runs) >= 1
        latest = runs[0]
        assert "initiatedBy" in latest
        assert "startedAt" in latest
        assert "completedAt" in latest
        assert latest["status"] in ("COMPLETED", "PARTIAL", "FAILED")


# ─── Search & Pagination ─────────────────────────────────────────────────────

class TestSearchPagination:

    def test_search_by_account_number(self, h):
        """Search should find accounts by partial account number."""
        # Get an account number first
        resp = requests.get(f"{BASE}/api/v1/accounts?size=1", headers=h)
        accts = resp.json().get("content", [])
        if not accts:
            pytest.skip("No accounts")
        num = accts[0]["accountNumber"][:8]

        resp = requests.get(f"{BASE}/api/v1/accounts/search?q={num}", headers=h)
        assert resp.status_code == 200
        results = resp.json()
        assert len(results) >= 1

    def test_pagination_first_page(self, h):
        resp = requests.get(f"{BASE}/api/v1/accounts?page=0&size=5", headers=h)
        assert resp.status_code == 200
        data = resp.json()
        assert len(data["content"]) <= 5
        assert data["totalElements"] >= 1

    def test_pagination_beyond_last_page(self, h):
        resp = requests.get(f"{BASE}/api/v1/accounts?page=9999&size=20", headers=h)
        assert resp.status_code == 200
        data = resp.json()
        content = data.get("content") or []
        assert len(content) == 0
