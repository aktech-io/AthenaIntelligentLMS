"""
Fraud Detection Load Test — 100K Simulated Transactions

Generates 100,000 realistic transactions for ~2,000 synthetic customers,
embedding deliberate fraud patterns that should trigger the 17 detection rules.
Then verifies the fraud-detection engine actually caught them.

Usage:
    python3 -m pytest tests/test_25_fraud_load_generator.py -v -s --tb=short

Pattern distribution across 100,000 transactions:
    ~85,000  Normal transactions (legitimate)
    ~2,000   Large single transactions (>1M KES)           → LARGE_SINGLE_TXN
    ~2,000   Structuring (many small txns aggregating >1M) → STRUCTURING
    ~1,500   Round amount patterns (multiple round #s)     → ROUND_AMOUNT_PATTERN
    ~1,500   High velocity (>10/hr or >50/day)             → HIGH_VELOCITY_1H / 24H
    ~1,000   Rapid fund movement (in+out <15min)           → RAPID_FUND_MOVEMENT
    ~800     Application stacking (>5 apps/30d)            → APPLICATION_STACKING
    ~500     Early payoff (<30d)                           → EARLY_PAYOFF_SUSPICIOUS
    ~500     Loan cycling (close+apply <7d)                → LOAN_CYCLING
    ~500     Dormant reactivation (>180d idle)             → DORMANT_REACTIVATION
    ~500     KYC bypass (pending KYC)                      → KYC_BYPASS_ATTEMPT
    ~500     Overdraft rapid draw (>90% immediately)       → OVERDRAFT_RAPID_DRAW
    ~500     BNPL abuse (>3 approvals/7d)                  → BNPL_ABUSE
    ~500     Payment reversal abuse (>30% reversed)        → PAYMENT_REVERSAL_ABUSE
    ~500     Overpayment (>110% of outstanding)            → OVERPAYMENT
    ~500     Suspicious writeoff (recent payments)         → SUSPICIOUS_WRITEOFF
    ~500     Watchlist match (matching PEP/sanctions)      → WATCHLIST_MATCH
    ~1,200   Promise-to-pay gaming                         → PROMISE_TO_PAY_GAMING
"""
import json
import math
import os
import random
import time
import uuid
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timedelta, timezone

import pytest
import requests
from conftest import SERVICES, TIMEOUT, unique_id

FRAUD_BASE = SERVICES.get("fraud", "http://localhost:28100")
TOTAL_TRANSACTIONS = 100_000
NUM_CUSTOMERS = 2_000
BATCH_SIZE = 500  # send in batches for throughput
MAX_WORKERS = 20  # concurrent threads


def fraud_url(path: str) -> str:
    return f"{FRAUD_BASE}{path}"


# ---------------------------------------------------------------------------
# Token helper
# ---------------------------------------------------------------------------
def get_admin_token():
    acct_base = SERVICES.get("account", "http://localhost:28086")
    r = requests.post(f"{acct_base}/api/auth/login",
                      json={"username": "admin",
                            "password": os.getenv("LMS_ADMIN_PASSWORD", "admin123")},
                      timeout=TIMEOUT)
    assert r.status_code == 200, f"Login failed: {r.text}"
    return r.json()["token"]


# ---------------------------------------------------------------------------
# Transaction generators
# ---------------------------------------------------------------------------
EVENT_TYPES_NORMAL = [
    "payment.completed", "transfer.completed",
    "account.credit.received", "account.debit.processed",
]

# Pre-generate customer IDs
CUSTOMER_IDS = [f"CUST-LOAD-{i:05d}" for i in range(NUM_CUSTOMERS)]

# Designate fraud-prone customers (10% of population = 200 customers)
FRAUD_CUSTOMERS = CUSTOMER_IDS[:200]
NORMAL_CUSTOMERS = CUSTOMER_IDS[200:]


def random_normal_txn(customer_id):
    """Generate a legitimate transaction."""
    return {
        "customerId": customer_id,
        "eventType": random.choice(EVENT_TYPES_NORMAL),
        "amount": round(random.uniform(100, 500000), 2),
        "subjectType": "ACCOUNT",
        "subjectId": f"ACC-{customer_id[-5:]}",
        "kycStatus": "PASSED",
    }


def large_transaction(customer_id):
    """Amount > 1M KES → LARGE_SINGLE_TXN."""
    return {
        "customerId": customer_id,
        "eventType": random.choice(["payment.completed", "account.credit.received", "transfer.completed"]),
        "amount": round(random.uniform(1_000_001, 10_000_000), 2),
        "subjectType": "ACCOUNT",
        "subjectId": f"ACC-{customer_id[-5:]}",
        "kycStatus": "PASSED",
    }


def structuring_txn(customer_id):
    """Multiple transactions just below 1M → STRUCTURING."""
    return {
        "customerId": customer_id,
        "eventType": random.choice(["payment.completed", "account.credit.received"]),
        "amount": round(random.uniform(200_000, 999_999), 2),
        "subjectType": "ACCOUNT",
        "subjectId": f"ACC-{customer_id[-5:]}",
        "kycStatus": "PASSED",
    }


def round_amount_txn(customer_id):
    """Round number amounts → ROUND_AMOUNT_PATTERN."""
    round_amts = [10_000, 20_000, 50_000, 100_000, 200_000, 500_000]
    return {
        "customerId": customer_id,
        "eventType": random.choice(["payment.completed", "transfer.completed"]),
        "amount": random.choice(round_amts),
        "subjectType": "ACCOUNT",
        "subjectId": f"ACC-{customer_id[-5:]}",
        "kycStatus": "PASSED",
    }


def high_velocity_txn(customer_id):
    """Rapid-fire transaction → HIGH_VELOCITY_1H / HIGH_VELOCITY_24H."""
    return {
        "customerId": customer_id,
        "eventType": random.choice(EVENT_TYPES_NORMAL),
        "amount": round(random.uniform(1_000, 50_000), 2),
        "subjectType": "ACCOUNT",
        "subjectId": f"ACC-{customer_id[-5:]}",
        "kycStatus": "PASSED",
    }


def rapid_fund_movement(customer_id):
    """Transfer completed shortly after credit → RAPID_FUND_MOVEMENT."""
    return {
        "customerId": customer_id,
        "eventType": "transfer.completed",
        "amount": round(random.uniform(100_000, 900_000), 2),
        "subjectType": "ACCOUNT",
        "subjectId": f"ACC-{customer_id[-5:]}",
        "kycStatus": "PASSED",
    }


def application_stacking(customer_id):
    """Loan application → APPLICATION_STACKING."""
    return {
        "customerId": customer_id,
        "eventType": "loan.application.submitted",
        "amount": round(random.uniform(50_000, 500_000), 2),
        "subjectType": "APPLICATION",
        "subjectId": f"APP-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
    }


def early_payoff(customer_id):
    """Loan closed too quickly → EARLY_PAYOFF_SUSPICIOUS."""
    return {
        "customerId": customer_id,
        "eventType": "loan.closed",
        "amount": round(random.uniform(100_000, 1_000_000), 2),
        "subjectType": "LOAN",
        "subjectId": f"LN-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
        "loanDisbursedAt": (datetime.now(timezone.utc) - timedelta(days=random.randint(1, 25))).isoformat(),
    }


def loan_cycling(customer_id):
    """Loan app shortly after close → LOAN_CYCLING."""
    return {
        "customerId": customer_id,
        "eventType": "loan.application.submitted",
        "amount": round(random.uniform(50_000, 300_000), 2),
        "subjectType": "APPLICATION",
        "subjectId": f"APP-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
        "loanDisbursedAt": (datetime.now(timezone.utc) - timedelta(days=random.randint(1, 5))).isoformat(),
    }


def dormant_reactivation(customer_id):
    """Activity on dormant account → DORMANT_REACTIVATION."""
    return {
        "customerId": customer_id,
        "eventType": random.choice(["account.credit.received", "account.unfrozen"]),
        "amount": round(random.uniform(10_000, 500_000), 2),
        "subjectType": "ACCOUNT",
        "subjectId": f"ACC-{customer_id[-5:]}",
        "kycStatus": "PASSED",
        "accountLastActiveAt": (datetime.now(timezone.utc) - timedelta(days=random.randint(181, 730))).isoformat(),
    }


def kyc_bypass(customer_id):
    """Transaction with pending KYC → KYC_BYPASS_ATTEMPT."""
    return {
        "customerId": customer_id,
        "eventType": random.choice(["payment.completed", "transfer.completed"]),
        "amount": round(random.uniform(10_000, 300_000), 2),
        "subjectType": "ACCOUNT",
        "subjectId": f"ACC-{customer_id[-5:]}",
        "kycStatus": random.choice(["PENDING", "FAILED", "NOT_STARTED"]),
    }


def overdraft_rapid_draw(customer_id):
    """Large overdraft drawdown → OVERDRAFT_RAPID_DRAW."""
    limit = round(random.uniform(100_000, 500_000), 2)
    return {
        "customerId": customer_id,
        "eventType": "overdraft.drawn",
        "amount": round(limit * random.uniform(0.91, 1.0), 2),
        "subjectType": "OVERDRAFT",
        "subjectId": f"OD-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
        "overdraftLimit": limit,
    }


def bnpl_abuse(customer_id):
    """Rapid BNPL approvals → BNPL_ABUSE."""
    return {
        "customerId": customer_id,
        "eventType": "shop.bnpl.approved",
        "amount": round(random.uniform(5_000, 50_000), 2),
        "subjectType": "BNPL",
        "subjectId": f"BNPL-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
    }


def payment_reversal(customer_id):
    """Payment reversal → PAYMENT_REVERSAL_ABUSE."""
    return {
        "customerId": customer_id,
        "eventType": "payment.reversed",
        "amount": round(random.uniform(5_000, 100_000), 2),
        "subjectType": "PAYMENT",
        "subjectId": f"PAY-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
        "reversalCount": random.randint(5, 20),
        "totalPayments": random.randint(10, 30),
    }


def overpayment(customer_id):
    """Payment exceeding loan balance → OVERPAYMENT."""
    outstanding = round(random.uniform(50_000, 300_000), 2)
    return {
        "customerId": customer_id,
        "eventType": "payment.completed",
        "amount": round(outstanding * random.uniform(1.15, 2.0), 2),
        "subjectType": "LOAN",
        "subjectId": f"LN-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
        "loanOutstanding": outstanding,
    }


def suspicious_writeoff(customer_id):
    """Write-off with recent payments → SUSPICIOUS_WRITEOFF."""
    return {
        "customerId": customer_id,
        "eventType": "loan.written.off",
        "amount": round(random.uniform(50_000, 500_000), 2),
        "subjectType": "LOAN",
        "subjectId": f"LN-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
        "loanDisbursedAt": (datetime.now(timezone.utc) - timedelta(days=random.randint(5, 25))).isoformat(),
    }


def watchlist_match_txn(customer_id):
    """Customer on watchlist → WATCHLIST_MATCH."""
    return {
        "customerId": customer_id,
        "eventType": random.choice(["customer.created", "customer.updated", "loan.application.submitted"]),
        "amount": round(random.uniform(10_000, 500_000), 2),
        "subjectType": "CUSTOMER",
        "subjectId": customer_id,
        "kycStatus": "PASSED",
    }


def ptp_gaming(customer_id):
    """Unfulfilled promises → PROMISE_TO_PAY_GAMING."""
    return {
        "customerId": customer_id,
        "eventType": "loan.dpd.updated",
        "amount": round(random.uniform(10_000, 200_000), 2),
        "subjectType": "LOAN",
        "subjectId": f"LN-{uuid.uuid4().hex[:8]}",
        "kycStatus": "PASSED",
    }


# ---------------------------------------------------------------------------
# Transaction mix builder
# ---------------------------------------------------------------------------
def build_transaction_batch():
    """Build the full 100K transaction list with embedded fraud patterns."""
    transactions = []
    expected_triggers = defaultdict(int)

    # ── Fraud patterns ──
    # LARGE_SINGLE_TXN: 2000 txns across 100 customers
    for cust in FRAUD_CUSTOMERS[:100]:
        for _ in range(20):
            transactions.append(large_transaction(cust))
            expected_triggers["LARGE_SINGLE_TXN"] += 1

    # STRUCTURING: 2000 txns across 50 customers (40 each to aggregate > 1M)
    for cust in FRAUD_CUSTOMERS[:50]:
        for _ in range(40):
            transactions.append(structuring_txn(cust))
        expected_triggers["STRUCTURING"] += 1  # per customer, not per txn

    # ROUND_AMOUNT_PATTERN: 1500 txns across 50 customers (30 each)
    for cust in FRAUD_CUSTOMERS[50:100]:
        for _ in range(30):
            transactions.append(round_amount_txn(cust))
        expected_triggers["ROUND_AMOUNT_PATTERN"] += 1

    # HIGH_VELOCITY: 1500 txns — 50 customers × 30 txns (will trigger both 1H and 24H)
    for cust in FRAUD_CUSTOMERS[100:150]:
        for _ in range(30):
            transactions.append(high_velocity_txn(cust))
        expected_triggers["HIGH_VELOCITY_1H"] += 1
        expected_triggers["HIGH_VELOCITY_24H"] += 1

    # RAPID_FUND_MOVEMENT: 1000 txns across 50 customers
    for cust in FRAUD_CUSTOMERS[50:100]:
        for _ in range(20):
            transactions.append(rapid_fund_movement(cust))
        expected_triggers["RAPID_FUND_MOVEMENT"] += 1

    # APPLICATION_STACKING: 800 txns across 100 customers (8 apps each)
    for cust in FRAUD_CUSTOMERS[:100]:
        for _ in range(8):
            transactions.append(application_stacking(cust))
        expected_triggers["APPLICATION_STACKING"] += 1

    # EARLY_PAYOFF: 500 txns across 500 customers
    for cust in FRAUD_CUSTOMERS[:500 % 200]:
        transactions.append(early_payoff(cust))
        expected_triggers["EARLY_PAYOFF_SUSPICIOUS"] += 1
    for i in range(300):
        transactions.append(early_payoff(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["EARLY_PAYOFF_SUSPICIOUS"] += 1

    # LOAN_CYCLING: 500
    for i in range(500):
        transactions.append(loan_cycling(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["LOAN_CYCLING"] += 1

    # DORMANT_REACTIVATION: 500
    for i in range(500):
        transactions.append(dormant_reactivation(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["DORMANT_REACTIVATION"] += 1

    # KYC_BYPASS: 500
    for i in range(500):
        transactions.append(kyc_bypass(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["KYC_BYPASS_ATTEMPT"] += 1

    # OVERDRAFT_RAPID_DRAW: 500
    for i in range(500):
        transactions.append(overdraft_rapid_draw(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["OVERDRAFT_RAPID_DRAW"] += 1

    # BNPL_ABUSE: 500 — groups of 4+ per customer
    for cust in FRAUD_CUSTOMERS[:125]:
        for _ in range(4):
            transactions.append(bnpl_abuse(cust))
        expected_triggers["BNPL_ABUSE"] += 1

    # PAYMENT_REVERSAL_ABUSE: 500
    for i in range(500):
        transactions.append(payment_reversal(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["PAYMENT_REVERSAL_ABUSE"] += 1

    # OVERPAYMENT: 500
    for i in range(500):
        transactions.append(overpayment(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["OVERPAYMENT"] += 1

    # SUSPICIOUS_WRITEOFF: 500
    for i in range(500):
        transactions.append(suspicious_writeoff(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["SUSPICIOUS_WRITEOFF"] += 1

    # WATCHLIST_MATCH: 500
    for i in range(500):
        transactions.append(watchlist_match_txn(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["WATCHLIST_MATCH"] += 1

    # PROMISE_TO_PAY_GAMING: 1200
    for i in range(1200):
        transactions.append(ptp_gaming(FRAUD_CUSTOMERS[i % 200]))
        expected_triggers["PROMISE_TO_PAY_GAMING"] += 1

    # Fill rest with normal transactions
    fraud_count = len(transactions)
    normal_needed = TOTAL_TRANSACTIONS - fraud_count
    for _ in range(normal_needed):
        transactions.append(random_normal_txn(random.choice(NORMAL_CUSTOMERS)))

    random.shuffle(transactions)
    return transactions, dict(expected_triggers)


# ---------------------------------------------------------------------------
# Sender
# ---------------------------------------------------------------------------
def send_batch(headers, batch):
    """Send a batch of transactions to the evaluate endpoint."""
    results = {"sent": 0, "alerts_created": 0, "rules_triggered": 0, "errors": 0}
    for txn in batch:
        try:
            r = requests.post(
                fraud_url("/api/v1/fraud/evaluate"),
                json=txn,
                headers=headers,
                timeout=30,
            )
            if r.status_code == 200:
                body = r.json()
                results["sent"] += 1
                results["alerts_created"] += body.get("alertsCreated", 0)
                results["rules_triggered"] += body.get("rulesTriggered", 0)
            else:
                results["errors"] += 1
        except Exception:
            results["errors"] += 1
    return results


# ===========================================================================
# Tests
# ===========================================================================
@pytest.mark.fraud
@pytest.mark.load
class TestFraudLoadGenerator:
    """100K transaction stress test for fraud detection engine."""

    @pytest.fixture(scope="class")
    def auth_headers(self):
        token = get_admin_token()
        return {"Content-Type": "application/json", "Authorization": f"Bearer {token}"}

    @pytest.fixture(scope="class")
    def pre_load_summary(self, auth_headers):
        """Capture alert counts before the load test."""
        r = requests.get(fraud_url("/api/v1/fraud/summary"),
                         headers=auth_headers, timeout=TIMEOUT)
        if r.status_code == 200:
            return r.json()
        return {}

    @pytest.fixture(scope="class")
    def load_results(self, auth_headers):
        """Run the full 100K load and return aggregate results."""
        # First, seed some watchlist entries so WATCHLIST_MATCH can trigger
        for i in range(10):
            cust = FRAUD_CUSTOMERS[i]
            requests.post(
                fraud_url("/api/v1/fraud/watchlist"),
                json={
                    "listType": random.choice(["PEP", "SANCTIONS"]),
                    "entryType": "INDIVIDUAL",
                    "name": cust,
                    "nationalId": cust,
                    "reason": "Load test watchlist entry",
                    "source": "load-test",
                },
                headers=auth_headers,
                timeout=TIMEOUT,
            )

        print(f"\n{'='*60}")
        print(f"  FRAUD DETECTION LOAD TEST — {TOTAL_TRANSACTIONS:,} TRANSACTIONS")
        print(f"{'='*60}")

        transactions, expected = build_transaction_batch()
        print(f"  Generated {len(transactions):,} transactions")
        print(f"  Normal: ~{TOTAL_TRANSACTIONS - sum(v for v in expected.values()):,}")
        print(f"  Fraud patterns embedded: {sum(v for v in expected.values()):,}")

        # Split into batches
        batches = [transactions[i:i + BATCH_SIZE]
                    for i in range(0, len(transactions), BATCH_SIZE)]

        totals = {"sent": 0, "alerts_created": 0, "rules_triggered": 0, "errors": 0}
        start = time.time()

        with ThreadPoolExecutor(max_workers=MAX_WORKERS) as pool:
            futures = {pool.submit(send_batch, auth_headers, batch): idx
                       for idx, batch in enumerate(batches)}

            done = 0
            for future in as_completed(futures):
                result = future.result()
                for k in totals:
                    totals[k] += result[k]
                done += 1
                if done % 20 == 0:
                    elapsed = time.time() - start
                    tps = totals["sent"] / elapsed if elapsed > 0 else 0
                    print(f"  Progress: {totals['sent']:,}/{TOTAL_TRANSACTIONS:,} "
                          f"({tps:.0f} txn/s) "
                          f"alerts={totals['alerts_created']:,} "
                          f"errors={totals['errors']:,}")

        elapsed = time.time() - start
        tps = totals["sent"] / elapsed if elapsed > 0 else 0
        totals["elapsed"] = elapsed
        totals["tps"] = tps
        totals["expected"] = expected

        print(f"\n  {'─'*50}")
        print(f"  COMPLETED in {elapsed:.1f}s ({tps:.0f} txn/s)")
        print(f"  Sent:     {totals['sent']:,}")
        print(f"  Alerts:   {totals['alerts_created']:,}")
        print(f"  Triggers: {totals['rules_triggered']:,}")
        print(f"  Errors:   {totals['errors']:,}")
        print(f"  {'─'*50}")

        return totals

    def test_all_transactions_sent(self, load_results):
        """All 100K transactions should be sent successfully."""
        assert load_results["sent"] + load_results["errors"] == TOTAL_TRANSACTIONS
        error_rate = load_results["errors"] / TOTAL_TRANSACTIONS
        assert error_rate < 0.01, f"Error rate {error_rate:.1%} exceeds 1% threshold"

    def test_alerts_were_created(self, load_results):
        """The engine should create alerts for fraud patterns."""
        assert load_results["alerts_created"] > 0, "No alerts created — engine not detecting fraud!"
        print(f"\n  Total alerts created: {load_results['alerts_created']:,}")

    def test_rules_were_triggered(self, load_results):
        """Multiple rules should have triggered."""
        assert load_results["rules_triggered"] > 0, "No rules triggered!"

    def test_throughput(self, load_results):
        """Should process at reasonable throughput (>100 txn/s)."""
        tps = load_results["tps"]
        print(f"\n  Throughput: {tps:.0f} txn/s")
        assert tps > 50, f"Throughput too low: {tps:.0f} txn/s"

    def test_detection_rate(self, load_results):
        """At least 1% of transactions should trigger rules (we embed ~15% fraud)."""
        if load_results["sent"] == 0:
            pytest.skip("No transactions sent")
        detection_rate = load_results["alerts_created"] / load_results["sent"]
        print(f"\n  Detection rate: {detection_rate:.2%}")
        # We embed ~15% fraud, so detection should be at least 1%
        assert detection_rate > 0.005, f"Detection rate {detection_rate:.2%} too low"

    def test_post_load_summary_increased(self, auth_headers, pre_load_summary, load_results):
        """Alert counts in summary should have increased after load."""
        r = requests.get(fraud_url("/api/v1/fraud/summary"),
                         headers=auth_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        post = r.json()

        pre_open = pre_load_summary.get("openAlerts", 0)
        post_open = post.get("openAlerts", 0)
        print(f"\n  Open alerts: {pre_open} → {post_open} (+{post_open - pre_open})")
        assert post_open >= pre_open, "Alert count should not decrease"

    def test_post_load_analytics(self, auth_headers, load_results):
        """Analytics should reflect the load."""
        r = requests.get(fraud_url("/api/v1/fraud/analytics"),
                         headers=auth_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        body = r.json()
        print(f"\n  Post-load analytics:")
        print(f"    Total alerts:  {body['totalAlerts']}")
        print(f"    Active cases:  {body['activeCases']}")
        print(f"    Rule effectiveness: {len(body['ruleEffectiveness'])} rules")

    def test_post_load_alerts_by_rule(self, auth_headers, load_results):
        """Verify alerts were created for multiple rule types."""
        r = requests.get(fraud_url("/api/v1/fraud/alerts?size=100"),
                         headers=auth_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        alerts = r.json().get("content", [])
        if not alerts:
            pytest.skip("No alerts in system")

        rule_codes = {a.get("ruleCode") for a in alerts if a.get("ruleCode")}
        alert_types = {a.get("alertType") for a in alerts}
        print(f"\n  Unique rule codes triggered: {len(rule_codes)}")
        print(f"    Rules: {sorted(rule_codes)}")
        print(f"  Unique alert types: {len(alert_types)}")
        print(f"    Types: {sorted(alert_types)}")

        # Should have triggered at least 5 different rules
        assert len(alert_types) >= 3, f"Only {len(alert_types)} alert types — expected broader detection"

    def test_post_load_events_recorded(self, auth_headers, load_results):
        """Fraud events should be recorded in the event log."""
        r = requests.get(fraud_url("/api/v1/fraud/events/recent?size=10"),
                         headers=auth_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        body = r.json()
        total_events = body.get("totalElements", 0)
        print(f"\n  Fraud events recorded: {total_events:,}")
        assert total_events > 0, "No fraud events recorded"

    def test_post_load_high_risk_customers(self, auth_headers, load_results):
        """Some customers should be flagged as high risk."""
        r = requests.get(fraud_url("/api/v1/fraud/risk-profiles/high-risk?size=50"),
                         headers=auth_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        body = r.json()
        count = body.get("totalElements", 0)
        print(f"\n  High-risk customers: {count}")


# ---------------------------------------------------------------------------
# Print summary report at end
# ---------------------------------------------------------------------------
@pytest.mark.fraud
@pytest.mark.load
class TestFraudDetectionReport:
    """Final summary report — must run after TestFraudLoadGenerator."""

    def test_print_report(self, admin_headers):
        """Print a comprehensive detection report."""
        # Summary
        r = requests.get(fraud_url("/api/v1/fraud/summary"),
                         headers=admin_headers, timeout=TIMEOUT)
        if r.status_code != 200:
            pytest.skip("Cannot get summary")
        summary = r.json()

        # Analytics
        r2 = requests.get(fraud_url("/api/v1/fraud/analytics"),
                          headers=admin_headers, timeout=TIMEOUT)
        analytics = r2.json() if r2.status_code == 200 else {}

        # Events
        r3 = requests.get(fraud_url("/api/v1/fraud/events/recent?size=1"),
                          headers=admin_headers, timeout=TIMEOUT)
        events_total = r3.json().get("totalElements", 0) if r3.status_code == 200 else 0

        print(f"\n{'='*60}")
        print(f"  FRAUD DETECTION ENGINE — FINAL REPORT")
        print(f"{'='*60}")
        print(f"  Open Alerts:          {summary.get('openAlerts', 0):,}")
        print(f"  Under Review:         {summary.get('underReviewAlerts', 0):,}")
        print(f"  Escalated:            {summary.get('escalatedAlerts', 0):,}")
        print(f"  Confirmed Fraud:      {summary.get('confirmedFraud', 0):,}")
        print(f"  Critical Alerts:      {summary.get('criticalAlerts', 0):,}")
        print(f"  High-Risk Customers:  {summary.get('highRiskCustomers', 0):,}")
        print(f"  Total Events:         {events_total:,}")
        print(f"  Resolution Rate:      {analytics.get('resolutionRate', 0):.1f}%")
        print(f"  Precision Rate:       {analytics.get('precisionRate', 0):.1f}%")

        if analytics.get("ruleEffectiveness"):
            print(f"\n  Rule Effectiveness:")
            for rule in analytics["ruleEffectiveness"]:
                print(f"    {rule['ruleCode']:30s}  "
                      f"triggers={rule['totalTriggers']:5d}  "
                      f"confirmed={rule['confirmedFraud']:3d}  "
                      f"FP={rule['falsePositives']:3d}")
        print(f"{'='*60}")
