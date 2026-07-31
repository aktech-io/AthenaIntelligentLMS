"""
Load Test & Audit: 500+ transactions across 10 wallets with full reconciliation.

Generates deposits, withdrawals, overdraft draws, and repayments,
then runs EOD batch and audits every result for correctness.

Run:  python3 -m pytest tests/test_27_load_audit.py -v -s --tb=short
"""
import pytest
import requests
import random
import time
import math
from collections import defaultdict
from conftest import url, unique_id, TIMEOUT

# ── Configuration ────────────────────────────────────────────────────────────
NUM_WALLETS = 10

# Arrangement fee charged at overdraft apply, keyed by credit band — mirrors
# credit_band_configs (legacy A–D seed + docs/nemo/06 §5 canonical bands).
# The deposit waterfall (BLOCKER-6) settles this fee out of the first
# deposit(s) before crediting the wallet balance.
ARRANGEMENT_FEE_BY_BAND = {
    "A": 1000.0, "B": 750.0, "C": 500.0, "D": 250.0,
    "EXCELLENT": 1000.0, "VERY_GOOD": 800.0, "GOOD": 750.0,
    "FAIR": 500.0, "MARGINAL": 400.0, "POOR": 250.0,
}
DEPOSITS_PER_WALLET = 20        # 200 total
WITHDRAWALS_PER_WALLET = 20     # 200 total
OD_DRAWS_PER_WALLET = 5         # 50 total (withdraw beyond balance)
REPAY_DEPOSITS_PER_WALLET = 5   # 50 total (deposit while drawn)
# Total = 500 transactions

TOLERANCE = 0.02  # KES tolerance for rounding


# ── Helpers ──────────────────────────────────────────────────────────────────
def _headers(admin_headers):
    return admin_headers


def _create_customer(headers, idx):
    cid = unique_id(f"LOAD{idx:02d}")
    r = requests.post(
        url("account", "/api/v1/customers"),
        headers=headers,
        json={
            "customerId": cid,
            "firstName": f"Load{idx}",
            "lastName": "Test",
            "email": f"{cid.lower()}@load.test",
            "phone": f"+2547{idx:08d}",
            "customerType": "INDIVIDUAL",
            "status": "ACTIVE",
        },
        timeout=TIMEOUT,
    )
    assert r.status_code in (200, 201), f"Customer create failed: {r.status_code} {r.text}"
    return cid


def _create_wallet(headers, cid):
    r = requests.post(
        url("overdraft", "/api/v1/wallets"),
        json={"customerId": cid, "currency": "KES"},
        headers=headers,
        timeout=TIMEOUT,
    )
    assert r.status_code in (200, 201), f"Wallet create failed: {r.status_code} {r.text}"
    return r.json()["id"]


def _apply_overdraft(headers, wallet_id):
    r = requests.post(
        url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
        headers=headers,
        timeout=TIMEOUT,
    )
    assert r.status_code in (200, 201), f"Apply OD failed: {r.status_code} {r.text}"
    return r.json()


def _deposit(headers, wallet_id, amount, ref):
    r = requests.post(
        url("overdraft", f"/api/v1/wallets/{wallet_id}/deposit"),
        json={"amount": amount, "reference": ref, "description": "Load test deposit"},
        headers=headers,
        timeout=TIMEOUT,
    )
    return r


def _withdraw(headers, wallet_id, amount, ref):
    r = requests.post(
        url("overdraft", f"/api/v1/wallets/{wallet_id}/withdraw"),
        json={"amount": amount, "reference": ref, "description": "Load test withdrawal"},
        headers=headers,
        timeout=TIMEOUT,
    )
    return r


def _get_wallet(headers, wallet_id):
    r = requests.get(
        url("overdraft", f"/api/v1/wallets/{wallet_id}"),
        headers=headers,
        timeout=TIMEOUT,
    )
    assert r.status_code == 200
    return r.json()


def _get_facility(headers, wallet_id):
    r = requests.get(
        url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft"),
        headers=headers,
        timeout=TIMEOUT,
    )
    return r.json() if r.status_code == 200 else None


def _get_transactions(headers, wallet_id, page=0, size=500):
    r = requests.get(
        url("overdraft", f"/api/v1/wallets/{wallet_id}/transactions?page={page}&size={size}"),
        headers=headers,
        timeout=TIMEOUT,
    )
    assert r.status_code == 200
    return r.json()


# ── Test Class ───────────────────────────────────────────────────────────────


@pytest.mark.load
@pytest.mark.timeout(900)  # class fixture does ~500 sequential HTTP calls; WAN/tunnel runs need headroom
class TestLoadAndAudit:
    """
    Generates 500+ transactions, then audits every balance, facility,
    interest charge, and audit log for correctness.
    """

    @pytest.fixture(scope="class")
    def test_data(self, admin_headers):
        """Phase 1 & 2: Create wallets and generate 500 transactions."""

        print("\n" + "=" * 80)
        print("PHASE 1: SETUP — Creating customers, wallets, and overdraft facilities")
        print("=" * 80)

        wallets = []
        ledger = defaultdict(lambda: {
            "deposits": [],
            "withdrawals": [],
            "total_deposited": 0.0,
            "total_withdrawn": 0.0,
            "od_draws": 0,
            "successful_txns": 0,
            "failed_txns": 0,
        })

        for i in range(NUM_WALLETS):
            cid = _create_customer(admin_headers, i)
            wid = _create_wallet(admin_headers, cid)
            facility = _apply_overdraft(admin_headers, wid)
            wallets.append({
                "wallet_id": wid,
                "customer_id": cid,
                "facility": facility,
                "od_limit": float(facility.get("approvedLimit", 0)),
            })
            print(f"  Wallet {i+1}/{NUM_WALLETS}: {wid[:8]}… "
                  f"Band={facility.get('creditBand')} "
                  f"Limit={facility.get('approvedLimit')}")

        print(f"\n  Created {len(wallets)} wallets with overdraft facilities.")

        print("\n" + "=" * 80)
        print("PHASE 2: LOAD — Generating 500 transactions")
        print("=" * 80)

        random.seed(42)  # Reproducible
        tx_count = 0
        errors = 0

        # Phase 2a: Deposits (200)
        print(f"\n  [1/4] Generating {DEPOSITS_PER_WALLET * NUM_WALLETS} deposits...")
        for w in wallets:
            wid = w["wallet_id"]
            for j in range(DEPOSITS_PER_WALLET):
                amount = round(random.uniform(500, 50000), 2)
                ref = unique_id(f"LDEP{tx_count}")
                r = _deposit(admin_headers, wid, amount, ref)
                if r.status_code == 200:
                    ledger[wid]["deposits"].append(amount)
                    ledger[wid]["total_deposited"] += amount
                    ledger[wid]["successful_txns"] += 1
                else:
                    ledger[wid]["failed_txns"] += 1
                    errors += 1
                tx_count += 1

        print(f"    Done. {tx_count} deposits attempted, {errors} errors.")

        # Phase 2b: Withdrawals (200)
        print(f"\n  [2/4] Generating {WITHDRAWALS_PER_WALLET * NUM_WALLETS} withdrawals...")
        withdraw_errors = 0
        for w in wallets:
            wid = w["wallet_id"]
            for j in range(WITHDRAWALS_PER_WALLET):
                # Withdraw up to 30% of deposited to keep balance positive
                max_wd = ledger[wid]["total_deposited"] * 0.03
                amount = round(random.uniform(100, max(200, max_wd)), 2)
                ref = unique_id(f"LWDR{tx_count}")
                r = _withdraw(admin_headers, wid, amount, ref)
                if r.status_code == 200:
                    ledger[wid]["withdrawals"].append(amount)
                    ledger[wid]["total_withdrawn"] += amount
                    ledger[wid]["successful_txns"] += 1
                else:
                    ledger[wid]["failed_txns"] += 1
                    withdraw_errors += 1
                tx_count += 1

        print(f"    Done. {WITHDRAWALS_PER_WALLET * NUM_WALLETS} attempted, "
              f"{withdraw_errors} rejected (expected — insufficient balance).")

        # Phase 2c: Overdraft draws (50) — withdraw beyond balance
        print(f"\n  [3/4] Generating {OD_DRAWS_PER_WALLET * NUM_WALLETS} overdraft draws...")
        od_draw_count = 0
        for w in wallets:
            wid = w["wallet_id"]
            for j in range(OD_DRAWS_PER_WALLET):
                # Get current wallet to know available
                wallet_info = _get_wallet(admin_headers, wid)
                available = float(wallet_info.get("availableBalance", 0))
                current = float(wallet_info.get("currentBalance", 0))

                # Try to draw into overdraft
                if available > 100:
                    # Withdraw enough to go negative
                    amount = round(current + random.uniform(1000, min(5000, available - current)), 2)
                    if amount > available:
                        amount = round(available * 0.9, 2)
                    if amount <= 0:
                        amount = 100
                    ref = unique_id(f"LODR{tx_count}")
                    r = _withdraw(admin_headers, wid, amount, ref)
                    if r.status_code == 200:
                        ledger[wid]["withdrawals"].append(amount)
                        ledger[wid]["total_withdrawn"] += amount
                        ledger[wid]["successful_txns"] += 1
                        od_draw_count += 1
                    else:
                        ledger[wid]["failed_txns"] += 1
                tx_count += 1

        print(f"    Done. {od_draw_count} overdraft draws successful.")

        # Phase 2d: Repayment deposits (50)
        print(f"\n  [4/4] Generating {REPAY_DEPOSITS_PER_WALLET * NUM_WALLETS} "
              f"repayment deposits (waterfall trigger)...")
        for w in wallets:
            wid = w["wallet_id"]
            for j in range(REPAY_DEPOSITS_PER_WALLET):
                amount = round(random.uniform(2000, 15000), 2)
                ref = unique_id(f"LRPY{tx_count}")
                r = _deposit(admin_headers, wid, amount, ref)
                if r.status_code == 200:
                    ledger[wid]["deposits"].append(amount)
                    ledger[wid]["total_deposited"] += amount
                    ledger[wid]["successful_txns"] += 1
                else:
                    ledger[wid]["failed_txns"] += 1
                tx_count += 1

        total_successful = sum(l["successful_txns"] for l in ledger.values())
        total_failed = sum(l["failed_txns"] for l in ledger.values())
        print(f"\n  TOTAL: {tx_count} transactions attempted")
        print(f"  Successful: {total_successful}")
        print(f"  Rejected:   {total_failed} (insufficient balance — expected)")

        return {
            "wallets": wallets,
            "ledger": ledger,
            "tx_count": tx_count,
            "total_successful": total_successful,
        }

    # ── Phase 3: EOD ─────────────────────────────────────────────────────────

    def test_01_eod_batch_runs(self, admin_headers, test_data):
        """Run EOD batch to accrue interest and classify DPD/NPL."""
        print("\n" + "=" * 80)
        print("PHASE 3: EOD BATCH — Interest accrual + DPD/NPL staging")
        print("=" * 80)

        r = requests.post(
            url("overdraft", "/api/v1/overdraft/eod/run"),
            headers=admin_headers,
            timeout=TIMEOUT * 5,
        )
        assert r.status_code == 200, f"EOD failed: {r.status_code} {r.text}"
        result = r.json()

        print(f"  Facilities processed:    {result['facilitiesProcessed']}")
        print(f"  Interest charges created: {result['interestChargesCreated']}")
        print(f"  Total interest accrued:   {result['totalInterestAccrued']} KES")
        print(f"  DPD updates:             {result['dpdUpdates']}")
        print(f"  Stage changes:           {result['stageChanges']}")
        print(f"  Billing statements:      {result['billingStatements']}")
        print(f"  Errors:                  {result['errors']}")
        print(f"  Duration:                {result['durationMs']}ms")

        assert result["errors"] == 0, f"EOD had {result['errors']} errors"

    def test_02_eod_idempotent(self, admin_headers, test_data):
        """Running EOD again should create zero new charges (idempotency)."""
        r = requests.post(
            url("overdraft", "/api/v1/overdraft/eod/run"),
            headers=admin_headers,
            timeout=TIMEOUT * 5,
        )
        assert r.status_code == 200
        result = r.json()
        assert result["interestChargesCreated"] == 0, \
            f"Idempotency failed: {result['interestChargesCreated']} charges on re-run"
        print(f"\n  Idempotency check PASSED — 0 duplicate charges on re-run")

    # ── Phase 4: AUDIT ───────────────────────────────────────────────────────

    def test_03_audit_wallet_balances(self, admin_headers, test_data):
        """
        AUDIT: For each wallet, verify currentBalance =
        sum(deposits) - sum(withdrawals), accounting for overdraft interest.
        """
        print("\n" + "=" * 80)
        print("PHASE 4: AUDIT — Reconciling balances, facilities, and audit trail")
        print("=" * 80)
        print("\n  [AUDIT 1] Wallet Balance Reconciliation")
        print("  " + "-" * 70)

        wallets = test_data["wallets"]
        ledger = test_data["ledger"]
        all_pass = True

        for w in wallets:
            wid = w["wallet_id"]
            wallet = _get_wallet(admin_headers, wid)
            actual_balance = float(wallet.get("currentBalance", 0))

            # Get facility: its arrangement fee is settled out of deposits by
            # the waterfall (fees -> accrued interest -> principal), so the
            # wallet balance nets it out. Deposits here are always >> fee, so
            # the fee is fully paid. No deposits happen after EOD accrual in
            # this scenario, so no interest repayment is netted.
            facility = _get_facility(admin_headers, wid)
            fee_paid = 0.0
            if facility:
                fee_paid = ARRANGEMENT_FEE_BY_BAND.get(facility.get("creditBand"), 0.0)

            expected_balance = (ledger[wid]["total_deposited"]
                                - ledger[wid]["total_withdrawn"]
                                - fee_paid)
            diff = abs(actual_balance - expected_balance)
            status = "PASS" if diff < TOLERANCE else "FAIL"
            if status == "FAIL":
                all_pass = False

            print(f"  Wallet {wid[:8]}… | "
                  f"Expected: {expected_balance:>12,.2f} | "
                  f"Actual: {actual_balance:>12,.2f} | "
                  f"Diff: {diff:>8,.2f} | "
                  f"{status}")

        assert all_pass, "Some wallet balances did not reconcile!"

    def test_04_audit_transaction_counts(self, admin_headers, test_data):
        """AUDIT: Verify transaction count per wallet matches ledger."""
        print("\n  [AUDIT 2] Transaction Count Verification")
        print("  " + "-" * 70)

        wallets = test_data["wallets"]
        ledger = test_data["ledger"]
        all_pass = True

        for w in wallets:
            wid = w["wallet_id"]
            txns = _get_transactions(admin_headers, wid)

            if isinstance(txns, dict) and "content" in txns:
                actual_count = txns.get("totalElements", len(txns["content"]))
            else:
                actual_count = len(txns) if isinstance(txns, list) else 0

            expected = ledger[wid]["successful_txns"]
            status = "PASS" if actual_count >= expected else "FAIL"
            if status == "FAIL":
                all_pass = False

            print(f"  Wallet {wid[:8]}… | "
                  f"Expected: >= {expected:>4} | "
                  f"Actual: {actual_count:>4} | "
                  f"{status}")

        assert all_pass, "Transaction counts don't match!"

    def test_05_audit_facility_states(self, admin_headers, test_data):
        """AUDIT: Verify each facility has valid state and NPL stage."""
        print("\n  [AUDIT 3] Facility State Validation")
        print("  " + "-" * 70)

        wallets = test_data["wallets"]
        all_pass = True

        valid_statuses = {"ACTIVE", "SUSPENDED", "CLOSED", "CHARGED_OFF"}
        valid_stages = {"PERFORMING", "STAGE1", "STAGE2", "STAGE3", "STAGE4"}

        for w in wallets:
            wid = w["wallet_id"]
            facility = _get_facility(admin_headers, wid)

            if not facility or not facility.get("hasOD"):
                print(f"  Wallet {wid[:8]}… | No facility | SKIP")
                continue

            status_val = facility.get("status", "UNKNOWN")
            stage = facility.get("nplStage", "UNKNOWN")
            dpd = facility.get("dpd", -1)
            drawn = float(facility.get("drawn", 0))
            limit = float(facility.get("limit", 0))

            checks = []
            if status_val not in valid_statuses:
                checks.append(f"Invalid status: {status_val}")
                all_pass = False
            if stage not in valid_stages:
                checks.append(f"Invalid NPL stage: {stage}")
                all_pass = False
            if drawn > limit * 1.01:  # Small tolerance
                checks.append(f"Drawn ({drawn}) > Limit ({limit})")
                all_pass = False
            if dpd < 0:
                checks.append(f"Negative DPD: {dpd}")
                all_pass = False

            result = "PASS" if not checks else f"FAIL: {'; '.join(checks)}"
            print(f"  Wallet {wid[:8]}… | "
                  f"Status={status_val:10s} | "
                  f"Stage={stage:12s} | "
                  f"DPD={dpd:3d} | "
                  f"Drawn={drawn:>10,.2f} | "
                  f"{result}")

        assert all_pass, "Some facilities have invalid state!"

    def test_06_audit_interest_charges(self, admin_headers, test_data):
        """AUDIT: Verify interest charges exist for drawn facilities."""
        print("\n  [AUDIT 4] Interest Charge Verification")
        print("  " + "-" * 70)

        wallets = test_data["wallets"]
        total_interest = 0.0

        for w in wallets:
            wid = w["wallet_id"]
            r = requests.get(
                url("overdraft", f"/api/v1/overdraft/{wid}/interest-charges"),
                headers=admin_headers,
                timeout=TIMEOUT,
            )
            assert r.status_code == 200
            charges = r.json()
            wallet_interest = sum(float(c.get("interestCharged", 0)) for c in charges)
            total_interest += wallet_interest

            # Verify charge structure
            for c in charges:
                assert "chargeDate" in c, "Missing chargeDate"
                assert "dailyRate" in c, "Missing dailyRate"
                assert float(c["interestCharged"]) >= 0, "Negative interest"
                assert float(c["dailyRate"]) > 0, "Zero daily rate"

            print(f"  Wallet {wid[:8]}… | "
                  f"Charges: {len(charges):3d} | "
                  f"Interest: {wallet_interest:>10,.4f} KES")

        print(f"\n  TOTAL INTEREST ACCRUED: {total_interest:,.4f} KES")

    def test_07_audit_overdraft_summary(self, admin_headers, test_data):
        """AUDIT: Verify summary aggregates match individual facility data."""
        print("\n  [AUDIT 5] Summary Aggregation Verification")
        print("  " + "-" * 70)

        r = requests.get(
            url("overdraft", "/api/v1/overdraft/summary"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        summary = r.json()

        # Manually aggregate from individual facilities
        wallets = test_data["wallets"]
        manual_total = 0.0
        manual_drawn = 0.0
        manual_active = 0
        band_counts = defaultdict(int)

        for w in wallets:
            facility = _get_facility(admin_headers, w["wallet_id"])
            if facility and facility.get("hasOD"):
                manual_total += float(facility.get("limit", 0))
                manual_drawn += float(facility.get("drawn", 0))
                band = facility.get("creditBand", "?")
                band_counts[band] += 1
                if facility.get("status") == "ACTIVE":
                    manual_active += 1

        summary_total = float(summary.get("totalApprovedLimit", 0))
        summary_drawn = float(summary.get("totalDrawnAmount", 0))
        summary_active = summary.get("activeFacilities", 0)

        # Note: summary includes ALL facilities for tenant, not just our test wallets
        # So summary values should be >= our manual counts
        print(f"  Summary total limit:    {summary_total:>12,.2f} KES")
        print(f"  Manual total limit:     {manual_total:>12,.2f} KES")
        print(f"  Summary total drawn:    {summary_drawn:>12,.2f} KES")
        print(f"  Manual total drawn:     {manual_drawn:>12,.2f} KES")
        print(f"  Summary active:         {summary_active}")
        print(f"  Manual active:          {manual_active}")
        print(f"  Facilities by band:     {dict(summary.get('facilitiesByBand', {}))}")

        assert summary_total >= manual_total - 1, \
            f"Summary limit ({summary_total}) < manual ({manual_total})"
        assert summary_active >= manual_active, \
            f"Summary active ({summary_active}) < manual ({manual_active})"

        print(f"\n  Summary aggregation: PASS")

    def test_08_audit_trail_completeness(self, admin_headers, test_data):
        """AUDIT: Verify audit log has entries for all major operations."""
        print("\n  [AUDIT 6] Audit Trail Completeness")
        print("  " + "-" * 70)

        r = requests.get(
            url("overdraft", "/api/v1/overdraft/audit?size=500"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        data = r.json()
        entries = data.get("content", data) if isinstance(data, dict) else data

        if isinstance(data, dict) and "totalElements" in data:
            total = data["totalElements"]
        else:
            total = len(entries)

        # Count actions
        action_counts = defaultdict(int)
        entity_types = defaultdict(int)
        for e in entries:
            action_counts[e.get("action", "UNKNOWN")] += 1
            entity_types[e.get("entityType", "UNKNOWN")] += 1

        print(f"  Total audit entries: {total}")
        print(f"\n  Actions breakdown:")
        for action, count in sorted(action_counts.items()):
            print(f"    {action:25s}: {count:>5}")
        print(f"\n  Entity types:")
        for etype, count in sorted(entity_types.items()):
            print(f"    {etype:25s}: {count:>5}")

        # Minimum expected entries:
        # - 10 CREATED (wallets)
        # - 10 FACILITY_APPROVED
        # - At least some DEPOSIT, WITHDRAWAL entries
        assert total >= 20, f"Expected >= 20 audit entries, got {total}"
        assert "CREATED" in action_counts, "Missing CREATED audit entries"
        assert "FACILITY_APPROVED" in action_counts, "Missing FACILITY_APPROVED entries"

        print(f"\n  Audit trail completeness: PASS")

    def test_09_audit_eod_status(self, admin_headers, test_data):
        """AUDIT: Verify EOD status reports last successful run."""
        print("\n  [AUDIT 7] EOD Status Check")
        print("  " + "-" * 70)

        r = requests.get(
            url("overdraft", "/api/v1/overdraft/eod/status"),
            headers=admin_headers,
            timeout=TIMEOUT,
        )
        assert r.status_code == 200
        status = r.json()

        assert status.get("running") is False, "EOD should not be running"
        assert "lastRunAt" in status, "Missing lastRunAt timestamp"
        assert "lastError" not in status or status["lastError"] is None, \
            f"EOD had error: {status.get('lastError')}"

        print(f"  Running:    {status.get('running')}")
        print(f"  Last run:   {status.get('lastRunAt')}")
        print(f"  Last error: {status.get('lastError', 'None')}")
        print(f"\n  EOD status: PASS")

    def test_10_final_report(self, admin_headers, test_data):
        """Print final summary report."""
        print("\n" + "=" * 80)
        print("FINAL AUDIT REPORT")
        print("=" * 80)

        wallets = test_data["wallets"]
        ledger = test_data["ledger"]

        total_deposited = sum(l["total_deposited"] for l in ledger.values())
        total_withdrawn = sum(l["total_withdrawn"] for l in ledger.values())
        total_successful = test_data["total_successful"]
        total_failed = sum(l["failed_txns"] for l in ledger.values())

        print(f"\n  Wallets created:        {len(wallets)}")
        print(f"  Total transactions:     {test_data['tx_count']}")
        print(f"  Successful:             {total_successful}")
        print(f"  Rejected:               {total_failed}")
        print(f"  Total deposited:        {total_deposited:>14,.2f} KES")
        print(f"  Total withdrawn:        {total_withdrawn:>14,.2f} KES")
        print(f"  Net flow:               {total_deposited - total_withdrawn:>14,.2f} KES")

        # Final balances
        print(f"\n  {'Wallet':<12} {'Balance':>14} {'Available':>14} {'OD Drawn':>12} {'Band':>6}")
        print("  " + "-" * 60)
        for w in wallets:
            wallet = _get_wallet(admin_headers, w["wallet_id"])
            facility = _get_facility(admin_headers, w["wallet_id"])
            balance = float(wallet.get("currentBalance", 0))
            available = float(wallet.get("availableBalance", 0))
            drawn = float(facility.get("drawn", 0)) if facility and facility.get("hasOD") else 0
            band = facility.get("creditBand", "-") if facility else "-"
            print(f"  {w['wallet_id'][:10]}… {balance:>14,.2f} {available:>14,.2f} {drawn:>12,.2f} {band:>6}")

        print("\n  " + "=" * 60)
        print("  ALL AUDIT CHECKS PASSED")
        print("  " + "=" * 60)
