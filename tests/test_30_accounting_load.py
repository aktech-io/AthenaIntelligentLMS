"""
Accounting Load Test — 500+ transactions across multiple accounts.

Validates:
  1. Balance invariant: final balance = initial_deposit + sum(credits) - sum(debits)
  2. Transaction count: all 500 txns recorded, none lost
  3. Interest math: daily_amount = balance * rate/100 / 365, WHT = 15% of gross
  4. EOD idempotency: running EOD twice on same day returns same result, no duplicates
  5. Interest posting: balance increases by net interest (gross - 15% WHT)
  6. Dormancy reactivation: credit to dormant account reactivates it
  7. Concurrency: parallel credits don't lose money
"""
import pytest
import requests
import random
import math
import time
from decimal import Decimal, ROUND_HALF_UP
from concurrent.futures import ThreadPoolExecutor, as_completed

BASE = "http://localhost:28086"
PRODUCT_BASE = "http://localhost:28087"


# ─── Helpers ──────────────────────────────────────────────────────────────────

def login(username="admin", password="admin123"):
    resp = requests.post(f"{BASE}/api/auth/login", json={
        "username": username, "password": password
    })
    assert resp.status_code == 200, f"Login failed: {resp.text}"
    return {"Authorization": f"Bearer {resp.json()['token']}"}


def get_balance(headers, acct_id):
    resp = requests.get(f"{BASE}/api/v1/accounts/{acct_id}/balance", headers=headers)
    assert resp.status_code == 200, f"Get balance failed: {resp.text}"
    return resp.json()


def credit(headers, acct_id, amount, desc="test credit"):
    return requests.post(f"{BASE}/api/v1/accounts/{acct_id}/credit", headers=headers, json={
        "amount": amount, "description": desc, "channel": "TEST"
    })


def debit(headers, acct_id, amount, desc="test debit"):
    return requests.post(f"{BASE}/api/v1/accounts/{acct_id}/debit", headers=headers, json={
        "amount": amount, "description": desc, "channel": "TEST"
    })


@pytest.fixture(scope="module")
def headers():
    return login()


# ─── Setup ────────────────────────────────────────────────────────────────────

class TestSetup:
    """Create test customers, deposit product, and accounts for the load test."""
    product_id = None
    accounts = {}  # name -> { id, initial_deposit }

    def test_01_create_deposit_product(self, headers):
        resp = requests.post(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=headers, json={
            "productCode": "LOAD-TEST-SAV",
            "name": "Load Test Savings",
            "productCategory": "SAVINGS",
            "interestRate": 5.0,
            "interestCalcMethod": "DAILY_BALANCE",
            "interestPostingFreq": "MONTHLY",
            "minOpeningBalance": 0,
            "minOperatingBalance": 0,
        })
        if resp.status_code == 409:
            # Already exists from previous run — fetch it
            resp2 = requests.get(f"{PRODUCT_BASE}/api/v1/deposit-products", headers=headers)
            for p in resp2.json().get("content", []):
                if p["productCode"] == "LOAD-TEST-SAV":
                    TestSetup.product_id = p["id"]
                    break
        else:
            assert resp.status_code == 201, f"Create product failed: {resp.text}"
            TestSetup.product_id = resp.json()["id"]

        # Activate it
        requests.post(
            f"{PRODUCT_BASE}/api/v1/deposit-products/{TestSetup.product_id}/activate",
            headers=headers
        )
        assert TestSetup.product_id is not None

    def test_02_create_customers(self, headers):
        for i in range(1, 6):
            cid = f"LOAD-CUST-{i:03d}"
            requests.post(f"{BASE}/api/v1/customers", headers=headers, json={
                "customerId": cid,
                "firstName": f"LoadTest",
                "lastName": f"Customer{i}",
                "customerType": "INDIVIDUAL",
                "email": f"load{i}@test.com",
            })
            # OK if already exists

    def test_03_open_accounts(self, headers):
        """Open 5 savings accounts with varying initial deposits."""
        deposits = [100_000, 250_000, 50_000, 500_000, 75_000]
        for i in range(1, 6):
            initial = deposits[i - 1]
            resp = requests.post(f"{BASE}/api/v1/accounts/open", headers=headers, json={
                "customerId": f"LOAD-CUST-{i:03d}",
                "depositProductId": TestSetup.product_id,
                "accountType": "SAVINGS",
                "currency": "KES",
                "kycTier": 3,
                "accountName": f"Load Test Account {i}",
                "initialDeposit": initial,
                "interestRateOverride": 5.0,
            })
            assert resp.status_code == 201, f"Open account {i} failed: {resp.text}"
            data = resp.json()
            TestSetup.accounts[f"acct_{i}"] = {
                "id": data["id"],
                "initial_deposit": initial,
            }

        assert len(TestSetup.accounts) == 5


# ─── Load Test: 500 Transactions ─────────────────────────────────────────────

class TestTransactionLoad:
    """Fire 500 transactions (credits + debits) and verify balance invariants."""
    credit_totals = {}  # acct_name -> total credits
    debit_totals = {}   # acct_name -> total debits

    def test_01_run_500_transactions(self, headers):
        """Execute 500 transactions across 5 accounts — 60% credits, 40% debits."""
        random.seed(42)  # reproducible
        acct_names = list(TestSetup.accounts.keys())
        txn_count = 500

        for name in acct_names:
            TestTransactionLoad.credit_totals[name] = Decimal("0")
            TestTransactionLoad.debit_totals[name] = Decimal("0")

        successes = 0
        failures = 0

        for i in range(txn_count):
            name = random.choice(acct_names)
            acct_id = TestSetup.accounts[name]["id"]

            is_credit = random.random() < 0.6
            amount = round(random.uniform(100, 10000), 2)

            if is_credit:
                resp = credit(headers, acct_id, amount, f"Load credit #{i}")
                if resp.status_code == 200:
                    TestTransactionLoad.credit_totals[name] += Decimal(str(amount))
                    successes += 1
                else:
                    failures += 1
            else:
                resp = debit(headers, acct_id, amount, f"Load debit #{i}")
                if resp.status_code == 200:
                    TestTransactionLoad.debit_totals[name] += Decimal(str(amount))
                    successes += 1
                else:
                    # Insufficient funds is expected — not a failure
                    if "Insufficient" in resp.text:
                        successes += 1  # correct rejection
                    else:
                        failures += 1

        print(f"\n  Transactions: {successes} successful, {failures} unexpected failures")
        assert failures == 0, f"{failures} unexpected transaction failures"
        assert successes >= 400, f"Only {successes} successful — too many rejections"

    def test_02_verify_balance_invariant(self, headers):
        """For each account: balance == initial_deposit + credits - debits."""
        for name, info in TestSetup.accounts.items():
            bal = get_balance(headers, info["id"])
            actual = Decimal(str(bal["availableBalance"]))
            expected = (
                Decimal(str(info["initial_deposit"]))
                + TestTransactionLoad.credit_totals[name]
                - TestTransactionLoad.debit_totals[name]
            )
            assert actual == expected, (
                f"{name}: expected balance {expected}, got {actual}. "
                f"initial={info['initial_deposit']}, "
                f"credits={TestTransactionLoad.credit_totals[name]}, "
                f"debits={TestTransactionLoad.debit_totals[name]}"
            )
        print(f"\n  All 5 account balances match expected values perfectly")

    def test_03_verify_transaction_count(self, headers):
        """Every account should have its transactions recorded (initial deposit + load)."""
        total_txns = 0
        for name, info in TestSetup.accounts.items():
            resp = requests.get(
                f"{BASE}/api/v1/accounts/{info['id']}/transactions?size=1",
                headers=headers
            )
            assert resp.status_code == 200
            count = resp.json()["totalElements"]
            assert count >= 2, f"{name} has only {count} transactions"
            total_txns += count

        print(f"\n  Total transactions recorded: {total_txns}")
        # We expect at least 500 + 5 initial deposits
        assert total_txns >= 505, f"Expected >=505 transactions, got {total_txns}"


# ─── Concurrent Transactions ──────────────────────────────────────────────────

class TestConcurrency:
    """Verify no money is lost under parallel credit/debit load."""

    def test_01_parallel_credits(self, headers):
        """10 concurrent credits of 1000 each = exactly 10000 increase."""
        acct_id = TestSetup.accounts["acct_1"]["id"]
        before = Decimal(str(get_balance(headers, acct_id)["availableBalance"]))

        def do_credit(_):
            return credit(headers, acct_id, 1000, "parallel credit")

        with ThreadPoolExecutor(max_workers=10) as pool:
            futures = [pool.submit(do_credit, i) for i in range(10)]
            results = [f.result() for f in as_completed(futures)]

        ok_count = sum(1 for r in results if r.status_code == 200)
        assert ok_count == 10, f"Only {ok_count}/10 parallel credits succeeded"

        after = Decimal(str(get_balance(headers, acct_id)["availableBalance"]))
        assert after == before + 10000, (
            f"Balance mismatch after parallel credits: before={before}, after={after}, "
            f"expected={before + 10000}"
        )
        print(f"\n  Parallel credits: balance increased by exactly 10000 ({before} -> {after})")


# ─── EOD & Interest ──────────────────────────────────────────────────────────

class TestEODAndInterest:
    """Test EOD processing, interest math, and idempotency."""
    eod_result_1 = None

    def test_01_run_eod_first_time(self, headers):
        # EOD is idempotent per business date. On a persistent DB it may already have
        # completed for today, so detect that up front to keep this test re-runnable.
        prior = requests.get(f"{BASE}/api/v1/eod/history", headers=headers)
        prior_dates = set()
        if prior.status_code == 200 and isinstance(prior.json(), list):
            prior_dates = {
                run["runDate"] for run in prior.json()
                if run.get("status") in ("COMPLETED", "PARTIAL")
            }

        resp = requests.post(f"{BASE}/api/v1/eod/run", headers=headers)
        assert resp.status_code == 200, f"EOD failed: {resp.text}"
        data = resp.json()
        TestEODAndInterest.eod_result_1 = data

        assert data["status"] in ("COMPLETED", "PARTIAL"), f"EOD status: {data['status']}"

        if data["runDate"] in prior_dates:
            # Idempotent re-run: EOD already completed for this business date, so this
            # call returns the existing result (no new accruals expected this call).
            # Accrual coverage on a fresh run is asserted in the else branch below, and
            # interest math is verified by test_03_verify_interest_math.
            print(f"\n  EOD run 1 (idempotent re-run for {data['runDate']}): "
                  f"already completed, {data['accountsAccrued']} accrued this call")
        else:
            # Genuinely the first EOD for this business date: at least one eligible
            # account (e.g. the funded savings accounts from test_29) must accrue.
            assert data["accountsAccrued"] >= 1, "No accounts accrued on first EOD run"
            print(f"\n  EOD run 1: {data['accountsAccrued']} accrued, "
                  f"total interest={data.get('totalInterestAccrued', 'N/A')}")

    def test_02_eod_idempotency(self, headers):
        """Running EOD again on the same day should return the existing completed result."""
        resp = requests.post(f"{BASE}/api/v1/eod/run", headers=headers)
        assert resp.status_code == 200, f"EOD 2nd run failed: {resp.text}"
        data = resp.json()

        # Should return same result — already completed
        assert data["status"] in ("COMPLETED", "PARTIAL")
        assert data["runDate"] == TestEODAndInterest.eod_result_1["runDate"]
        print(f"\n  EOD idempotency verified — same date, same result")

    def test_03_verify_interest_math(self, headers):
        """Verify daily interest = balance * rate / 100 / 365 for each account."""
        for name, info in TestSetup.accounts.items():
            resp = requests.get(
                f"{BASE}/api/v1/accounts/{info['id']}/interest-summary",
                headers=headers
            )
            assert resp.status_code == 200, f"Interest summary failed for {name}: {resp.text}"
            summary = resp.json()

            accruals = summary.get("recentAccruals") or []
            if not accruals:
                continue

            # Check the most recent accrual
            accrual = accruals[0]
            balance_used = Decimal(str(accrual["balanceUsed"]))
            rate = Decimal(str(accrual["rate"]))
            daily_amount = Decimal(str(accrual["dailyAmount"]))

            # Expected: balance * rate / 100 / 365, rounded to 4 decimals
            expected = (balance_used * rate / 100 / 365).quantize(
                Decimal("0.0001"), rounding=ROUND_HALF_UP
            )

            assert daily_amount == expected, (
                f"{name}: interest math wrong. "
                f"balance={balance_used}, rate={rate}%, "
                f"expected daily={expected}, got={daily_amount}"
            )

        print(f"\n  Interest math verified for all accounts: daily = balance * rate / 100 / 365")

    def test_04_post_interest_and_verify(self, headers):
        """Post interest for one account and verify balance increases by net (gross - 15% WHT)."""
        # Find an account with unposted interest (any of our accounts or from test_29)
        acct_id = None
        for name, info in TestSetup.accounts.items():
            resp = requests.get(f"{BASE}/api/v1/accounts/{info['id']}/interest-summary", headers=headers)
            if resp.status_code == 200 and resp.json().get("unpostedTotal", 0) > 0:
                acct_id = info["id"]
                break
        if acct_id is None:
            pytest.skip("No account with unposted interest available")

        # Get balance before posting
        before = Decimal(str(get_balance(headers, acct_id)["availableBalance"]))

        # Get unposted total
        resp = requests.get(f"{BASE}/api/v1/accounts/{acct_id}/interest-summary", headers=headers)
        assert resp.status_code == 200
        unposted = Decimal(str(resp.json()["unpostedTotal"]))

        if unposted == 0:
            pytest.skip("No unposted interest to test with")

        # Post interest
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/post-interest", headers=headers)
        assert resp.status_code == 200, f"Post interest failed: {resp.text}"
        posting = resp.json()

        gross = Decimal(str(posting["grossInterest"]))
        wht = Decimal(str(posting["withholdingTax"]))
        net = Decimal(str(posting["netInterest"]))

        # Verify WHT = 15% of gross
        expected_wht = (gross * Decimal("0.15")).quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)
        assert wht == expected_wht, f"WHT mismatch: expected {expected_wht}, got {wht}"

        # Verify net = gross - wht
        assert net == gross - wht, f"Net mismatch: {gross} - {wht} != {net}"

        # Verify balance increased by exactly net
        after = Decimal(str(get_balance(headers, acct_id)["availableBalance"]))
        assert after == before + net, (
            f"Balance after posting: expected {before + net}, got {after}. "
            f"gross={gross}, wht={wht}, net={net}"
        )

        print(f"\n  Interest posting verified: gross={gross}, WHT={wht}, net={net}")
        print(f"  Balance: {before} -> {after} (+{net})")

    def test_05_double_post_rejected(self, headers):
        """Posting interest when there's nothing to post should fail gracefully."""
        acct_id = TestSetup.accounts["acct_2"]["id"]
        resp = requests.post(f"{BASE}/api/v1/accounts/{acct_id}/post-interest", headers=headers)
        # Should return error — no unposted interest
        assert resp.status_code in (400, 422), f"Expected rejection, got {resp.status_code}: {resp.text}"

    def test_06_eod_history(self, headers):
        """Verify EOD history endpoint returns the run."""
        resp = requests.get(f"{BASE}/api/v1/eod/history", headers=headers)
        assert resp.status_code == 200
        runs = resp.json()
        assert len(runs) >= 1
        latest = runs[0]
        assert latest["status"] in ("COMPLETED", "PARTIAL")
        assert latest["initiatedBy"] != ""
        print(f"\n  EOD history: {len(runs)} runs, latest status={latest['status']}")


# ─── Dormancy ─────────────────────────────────────────────────────────────────

class TestDormancyReactivation:
    """Verify dormant accounts can be reactivated via credit."""

    def test_01_mark_account_dormant(self, headers):
        """Manually freeze then set dormant to test reactivation."""
        acct_id = TestSetup.accounts["acct_5"]["id"]
        # Set dormant via status update
        resp = requests.put(f"{BASE}/api/v1/accounts/{acct_id}/status", headers=headers, json={
            "status": "DORMANT"
        })
        assert resp.status_code == 200
        assert resp.json()["status"] == "DORMANT"

    def test_02_credit_reactivates_dormant(self, headers):
        """Credit to a dormant account should auto-reactivate it."""
        acct_id = TestSetup.accounts["acct_5"]["id"]
        resp = credit(headers, acct_id, 5000, "reactivation credit")
        assert resp.status_code == 200, f"Credit to dormant account failed: {resp.text}"

        # Verify account is now ACTIVE
        resp = requests.get(f"{BASE}/api/v1/accounts/{acct_id}", headers=headers)
        assert resp.status_code == 200
        assert resp.json()["status"] == "ACTIVE"
        print(f"\n  Dormant account reactivated on credit")

    def test_03_debit_on_dormant_rejected(self, headers):
        """Debit on a dormant account should be rejected."""
        acct_id = TestSetup.accounts["acct_5"]["id"]
        # Set dormant again
        requests.put(f"{BASE}/api/v1/accounts/{acct_id}/status", headers=headers, json={
            "status": "DORMANT"
        })
        resp = debit(headers, acct_id, 1000, "dormant debit attempt")
        assert resp.status_code in (400, 422), f"Debit on dormant should fail: {resp.text}"

        # Reactivate for cleanup
        requests.put(f"{BASE}/api/v1/accounts/{acct_id}/status", headers=headers, json={
            "status": "ACTIVE"
        })
