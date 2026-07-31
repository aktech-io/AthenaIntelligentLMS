"""
Test Suite: Deposit Products & Account Management System
Tests deposit product CRUD, account opening workflow, interest accrual, dormancy, and EOD.
"""
import os
import pytest
import requests
import time

BASE_AUTH = "http://localhost:28086"
BASE_PRODUCT = "http://localhost:28087"

# ─── Helpers ──────────────────────────────────────────────────────────────────

def login(username="admin", password=None):
    password = password or os.getenv("LMS_ADMIN_PASSWORD", "admin123")
    resp = requests.post(f"{BASE_AUTH}/api/auth/login", json={
        "username": username, "password": password
    })
    assert resp.status_code == 200, f"Login failed: {resp.text}"
    token = resp.json().get("token")
    assert token, "No token in login response"
    return {"Authorization": f"Bearer {token}"}

@pytest.fixture(scope="module")
def headers():
    return login()

# ─── Deposit Product CRUD ─────────────────────────────────────────────────────

def find_product_by_code(headers, code):
    """Find an existing deposit product by code."""
    resp = requests.get(f"{BASE_PRODUCT}/api/v1/deposit-products?size=100", headers=headers)
    if resp.status_code == 200:
        for p in resp.json().get("content", []):
            if p["productCode"] == code:
                return p
    return None

class TestDepositProducts:
    product_id = None

    def test_create_savings_product(self, headers):
        resp = requests.post(f"{BASE_PRODUCT}/api/v1/deposit-products", headers=headers, json={
            "productCode": "SAV-JIJENGE",
            "name": "Jijenge Savings",
            "description": "Tiered savings account for Kenya",
            "productCategory": "SAVINGS",
            "currency": "KES",
            "interestRate": 3.5,
            "interestCalcMethod": "DAILY_BALANCE",
            "interestPostingFreq": "MONTHLY",
            "interestCompoundFreq": "MONTHLY",
            "accrualFrequency": "DAILY",
            "minOpeningBalance": 500,
            "minOperatingBalance": 200,
            "minBalanceForInterest": 1000,
            "dormancyDaysThreshold": 365,
            "interestTiers": [
                {"fromAmount": 0, "toAmount": 50000, "rate": 3.0},
                {"fromAmount": 50000, "toAmount": 500000, "rate": 4.0},
                {"fromAmount": 500000, "toAmount": 99999999, "rate": 5.5}
            ]
        })
        if resp.status_code == 409:
            existing = find_product_by_code(headers, "SAV-JIJENGE")
            assert existing is not None, "Product exists but couldn't find it"
            TestDepositProducts.product_id = existing["id"]
        else:
            assert resp.status_code == 201, f"Create savings product failed: {resp.text}"
            data = resp.json()
            assert data["productCode"] == "SAV-JIJENGE"
            TestDepositProducts.product_id = data["id"]

    def test_create_current_product(self, headers):
        resp = requests.post(f"{BASE_PRODUCT}/api/v1/deposit-products", headers=headers, json={
            "productCode": "CUR-BASIC",
            "name": "Basic Current Account",
            "productCategory": "CURRENT",
            "interestRate": 0,
            "minOperatingBalance": 1000,
        })
        assert resp.status_code in (201, 409)

    def test_create_fd_product(self, headers):
        resp = requests.post(f"{BASE_PRODUCT}/api/v1/deposit-products", headers=headers, json={
            "productCode": "FD-90DAY",
            "name": "Fixed Deposit 90-Day",
            "productCategory": "FIXED_DEPOSIT",
            "interestRate": 8.0,
            "interestPostingFreq": "ON_MATURITY",
            "minOpeningBalance": 10000,
            "minTermDays": 90,
            "maxTermDays": 365,
            "earlyWithdrawalPenaltyRate": 2.0,
            "autoRenew": True,
        })
        assert resp.status_code in (201, 409)

    def test_list_deposit_products(self, headers):
        resp = requests.get(f"{BASE_PRODUCT}/api/v1/deposit-products", headers=headers)
        assert resp.status_code == 200
        data = resp.json()
        assert data["totalElements"] >= 3

    def test_get_deposit_product(self, headers):
        assert TestDepositProducts.product_id is not None
        resp = requests.get(
            f"{BASE_PRODUCT}/api/v1/deposit-products/{TestDepositProducts.product_id}",
            headers=headers
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["productCode"] == "SAV-JIJENGE"

    def test_activate_deposit_product(self, headers):
        resp = requests.post(
            f"{BASE_PRODUCT}/api/v1/deposit-products/{TestDepositProducts.product_id}/activate",
            headers=headers
        )
        assert resp.status_code == 200
        assert resp.json()["status"] == "ACTIVE"

    def test_update_deposit_product(self, headers):
        resp = requests.put(
            f"{BASE_PRODUCT}/api/v1/deposit-products/{TestDepositProducts.product_id}",
            headers=headers,
            json={
                "name": "Jijenge Savings Plus",
                "productCategory": "SAVINGS",
                "interestRate": 4.0,
                "interestTiers": [
                    {"fromAmount": 0, "toAmount": 100000, "rate": 3.5},
                    {"fromAmount": 100000, "toAmount": 99999999, "rate": 5.0}
                ]
            }
        )
        assert resp.status_code == 200
        data = resp.json()
        assert data["name"] == "Jijenge Savings Plus"
        assert len(data["interestTiers"]) == 2

    def test_duplicate_code_rejected(self, headers):
        resp = requests.post(f"{BASE_PRODUCT}/api/v1/deposit-products", headers=headers, json={
            "productCode": "SAV-JIJENGE",
            "name": "Duplicate",
            "productCategory": "SAVINGS",
        })
        assert resp.status_code == 409


# ─── Account Opening ──────────────────────────────────────────────────────────

class TestAccountOpening:
    customer_id = None
    savings_account_id = None
    fd_account_id = None

    def test_create_customer(self, headers):
        resp = requests.post(f"{BASE_AUTH}/api/v1/customers", headers=headers, json={
            "customerId": "CUST-DEP-001",
            "firstName": "Jane",
            "lastName": "Wanjiku",
            "customerType": "INDIVIDUAL",
            "email": "jane.w@example.com",
            "phone": "+254712345678",
        })
        # OK if already exists (re-run)
        assert resp.status_code in (200, 201, 400), f"Create customer failed: {resp.text}"
        TestAccountOpening.customer_id = "CUST-DEP-001"

    def test_open_savings_account(self, headers):
        # Check if we already have a savings account from a previous run
        resp = requests.get(f"{BASE_AUTH}/api/v1/accounts/customer/CUST-DEP-001", headers=headers)
        if resp.status_code == 200:
            for acct in resp.json():
                if acct.get("accountType") == "SAVINGS" and acct.get("depositProductId"):
                    TestAccountOpening.savings_account_id = acct["id"]
                    return

        resp = requests.post(f"{BASE_AUTH}/api/v1/accounts/open", headers=headers, json={
            "customerId": "CUST-DEP-001",
            "depositProductId": TestDepositProducts.product_id,
            "accountType": "SAVINGS",
            "currency": "KES",
            "kycTier": 2,
            "accountName": "Jane Wanjiku Savings",
            "initialDeposit": 50000,
            "interestRateOverride": 4.0,
        })
        assert resp.status_code == 201, f"Open savings failed: {resp.text}"
        data = resp.json()
        assert data["accountType"] == "SAVINGS"
        assert data["status"] == "ACTIVE"
        TestAccountOpening.savings_account_id = data["id"]

    def test_open_fd_account(self, headers):
        # Check if we already have an FD from a previous run
        resp = requests.get(f"{BASE_AUTH}/api/v1/accounts/customer/CUST-DEP-001", headers=headers)
        if resp.status_code == 200:
            for acct in resp.json():
                if acct.get("accountType") == "FIXED_DEPOSIT":
                    TestAccountOpening.fd_account_id = acct["id"]
                    return

        resp = requests.post(f"{BASE_AUTH}/api/v1/accounts/open", headers=headers, json={
            "customerId": "CUST-DEP-001",
            "accountType": "FIXED_DEPOSIT",
            "currency": "KES",
            "kycTier": 2,
            "accountName": "Jane FD 90-Day",
            "initialDeposit": 100000,
            "termDays": 90,
            "autoRenew": True,
            "interestRateOverride": 8.0,
        })
        assert resp.status_code == 201, f"Open FD failed: {resp.text}"
        data = resp.json()
        assert data["accountType"] == "FIXED_DEPOSIT"
        TestAccountOpening.fd_account_id = data["id"]

    def test_get_account_with_new_fields(self, headers):
        assert TestAccountOpening.savings_account_id is not None, "No savings account to test"
        resp = requests.get(
            f"{BASE_AUTH}/api/v1/accounts/{TestAccountOpening.savings_account_id}",
            headers=headers
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "balance" in data


# ─── Account Operations ───────────────────────────────────────────────────────

class TestAccountOperations:

    def test_credit_account(self, headers):
        acct_id = TestAccountOpening.savings_account_id
        resp = requests.post(f"{BASE_AUTH}/api/v1/accounts/{acct_id}/credit", headers=headers, json={
            "amount": 25000,
            "description": "Salary deposit",
            "channel": "MPESA",
        })
        assert resp.status_code == 200

    def test_debit_account(self, headers):
        acct_id = TestAccountOpening.savings_account_id
        resp = requests.post(f"{BASE_AUTH}/api/v1/accounts/{acct_id}/debit", headers=headers, json={
            "amount": 5000,
            "description": "ATM withdrawal",
            "channel": "ATM",
        })
        assert resp.status_code == 200

    def test_get_balance(self, headers):
        acct_id = TestAccountOpening.savings_account_id
        resp = requests.get(f"{BASE_AUTH}/api/v1/accounts/{acct_id}/balance", headers=headers)
        assert resp.status_code == 200
        bal = resp.json()
        assert bal["availableBalance"] > 0  # has some balance

    def test_get_transactions(self, headers):
        acct_id = TestAccountOpening.savings_account_id
        resp = requests.get(f"{BASE_AUTH}/api/v1/accounts/{acct_id}/transactions", headers=headers)
        assert resp.status_code == 200
        data = resp.json()
        assert data["totalElements"] >= 3  # initial deposit + credit + debit

    def test_interest_summary(self, headers):
        acct_id = TestAccountOpening.savings_account_id
        resp = requests.get(f"{BASE_AUTH}/api/v1/accounts/{acct_id}/interest-summary", headers=headers)
        assert resp.status_code == 200
        data = resp.json()
        assert "unpostedTotal" in data
        assert "recentAccruals" in data
        assert "postingHistory" in data


# ─── EOD Processing ───────────────────────────────────────────────────────────

class TestEOD:

    def test_run_eod(self, headers):
        resp = requests.post(f"{BASE_AUTH}/api/v1/eod/run", headers=headers)
        assert resp.status_code == 200
        data = resp.json()
        assert "runDate" in data
        assert "accountsAccrued" in data
        assert "dormantDetected" in data
        assert "maturedProcessed" in data
        assert data["status"] in ("COMPLETED", "PARTIAL")


# ─── Account Lifecycle ─────────────────────────────────────────────────────────

class TestAccountLifecycle:

    def test_freeze_account(self, headers):
        acct_id = TestAccountOpening.savings_account_id
        resp = requests.put(f"{BASE_AUTH}/api/v1/accounts/{acct_id}/status", headers=headers, json={
            "status": "FROZEN"
        })
        assert resp.status_code == 200
        assert resp.json()["status"] == "FROZEN"

    def test_unfreeze_account(self, headers):
        acct_id = TestAccountOpening.savings_account_id
        resp = requests.put(f"{BASE_AUTH}/api/v1/accounts/{acct_id}/status", headers=headers, json={
            "status": "ACTIVE"
        })
        assert resp.status_code == 200
        assert resp.json()["status"] == "ACTIVE"

    def test_close_account_with_balance_fails(self, headers):
        acct_id = TestAccountOpening.savings_account_id
        resp = requests.post(f"{BASE_AUTH}/api/v1/accounts/{acct_id}/close", headers=headers, json={
            "reason": "Customer request"
        })
        # Should fail because account has balance
        assert resp.status_code in (400, 422)

    def test_search_accounts(self, headers):
        resp = requests.get(f"{BASE_AUTH}/api/v1/accounts/search?q=Wanjiku", headers=headers)
        assert resp.status_code == 200
        accounts = resp.json()
        assert len(accounts) >= 1
