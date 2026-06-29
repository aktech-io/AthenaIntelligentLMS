"""
Finance Audit Controls — IFRS-readiness test suite.
Tests maker-checker workflow, fiscal periods, audit trail, and chart of accounts.
"""
import pytest
import requests
import time

ACCOUNTING_URL = "http://localhost:28091/api/v1/accounting"
AUTH_URL = "http://localhost:28086/api/auth/login"
TIMEOUT = 15


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
def get_token(username, password):
    """Authenticate and return JWT token."""
    try:
        resp = requests.post(
            AUTH_URL,
            json={"username": username, "password": password},
            timeout=TIMEOUT,
        )
        if resp.status_code == 200:
            data = resp.json()
            return data.get("token") or data.get("accessToken")
    except requests.ConnectionError:
        return None
    return None


def auth_header(token):
    return {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}


def get_account_id_by_code(token, code):
    """Look up a GL account ID by its code."""
    resp = requests.get(
        f"{ACCOUNTING_URL}/accounts/code/{code}",
        headers=auth_header(token),
        timeout=TIMEOUT,
    )
    if resp.status_code == 200:
        return resp.json().get("id")
    return None


def create_test_entry(token, cash_id, loans_id, amount=1000, entry_date=None):
    """Create a balanced journal entry (DR cash / CR loans)."""
    payload = {
        "reference": f"TEST-{int(time.time() * 1000)}",
        "description": "Test journal entry",
        "lines": [
            {"accountId": cash_id, "debitAmount": amount, "creditAmount": 0, "currency": "KES"},
            {"accountId": loans_id, "debitAmount": 0, "creditAmount": amount, "currency": "KES"},
        ],
    }
    if entry_date:
        # Go expects RFC3339 format for time.Time
        if "T" not in entry_date:
            entry_date = entry_date + "T00:00:00Z"
        payload["entryDate"] = entry_date
    resp = requests.post(
        f"{ACCOUNTING_URL}/journal-entries",
        json=payload,
        headers=auth_header(token),
        timeout=TIMEOUT,
    )
    return resp


def service_available():
    """Check if the accounting service is reachable."""
    try:
        resp = requests.get(f"{ACCOUNTING_URL}/accounts", timeout=5)
        return resp.status_code in (200, 401, 403)
    except requests.ConnectionError:
        return False


pytestmark = pytest.mark.skipif(
    not service_available(),
    reason="Accounting service not available at localhost:28091",
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------
# Session-scoped so conftest's session-scoped, autouse `disable_maker_checker`
# fixture (which depends on admin_token) can consume it without a ScopeMismatch.
@pytest.fixture(scope="session")
def admin_token():
    token = get_token("admin", "admin123")
    if not token:
        pytest.skip("Cannot authenticate as admin")
    return token


# Maker-checker on GL journal entries requires two *distinct finance-role* users
# (RBAC gates posting to ADMIN/MANAGER/ACCOUNTANT — officers cannot touch the
# GL). Maker = MANAGER, checker = ADMIN; different users satisfy the
# approved_by != created_by segregation-of-duties rule.
@pytest.fixture(scope="module")
def maker_token():
    token = get_token("manager", "manager123")
    if not token:
        pytest.skip("Cannot authenticate as manager")
    return token


@pytest.fixture(scope="module")
def checker_token():
    token = get_token("admin", "admin123")
    if not token:
        pytest.skip("Cannot authenticate as admin")
    return token


@pytest.fixture(scope="module")
def all_accounts(admin_token):
    """Fetch the full chart of accounts once for the module."""
    resp = requests.get(
        f"{ACCOUNTING_URL}/accounts",
        headers=auth_header(admin_token),
        timeout=TIMEOUT,
    )
    assert resp.status_code == 200, f"List accounts failed: {resp.status_code} {resp.text[:300]}"
    return resp.json()


@pytest.fixture(scope="module")
def cash_account_id(admin_token):
    """Resolve the Cash (1000) account ID."""
    aid = get_account_id_by_code(admin_token, "1000")
    if not aid:
        pytest.skip("Cash account 1000 not found")
    return aid


@pytest.fixture(scope="module")
def loans_account_id(admin_token):
    """Resolve the Loans Receivable (1100) account ID."""
    aid = get_account_id_by_code(admin_token, "1100")
    if not aid:
        pytest.skip("Loans Receivable account 1100 not found")
    return aid


# =========================================================================
# 1. Chart of Accounts (tests 1-5)
# =========================================================================
class TestChartOfAccounts:
    """Verify the IFRS-aligned chart of accounts is complete."""

    def test_ifrs_accounts_exist(self, all_accounts):
        """GET /accounts returns at least 30 accounts spanning 1000-6000 range."""
        assert len(all_accounts) >= 30, (
            f"Expected >= 30 GL accounts, got {len(all_accounts)}"
        )
        codes = {a["code"] for a in all_accounts}
        # Must have accounts in every major range
        for prefix in ["1", "2", "3", "4", "5"]:
            matching = [c for c in codes if c.startswith(prefix)]
            assert len(matching) > 0, f"No accounts found in the {prefix}xxx range"

    def test_asset_accounts_complete(self, admin_token, all_accounts):
        """Filter by type=ASSET and verify key accounts exist."""
        resp = requests.get(
            f"{ACCOUNTING_URL}/accounts?type=ASSET",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        assets = resp.json()
        codes = {a["code"] for a in assets}
        expected_asset_codes = ["1000", "1100", "1150", "1410", "1500", "1600"]
        for code in expected_asset_codes:
            assert code in codes, f"Missing ASSET account {code}"

    def test_liability_accounts_complete(self, admin_token):
        """Filter by type=LIABILITY and verify key accounts exist."""
        resp = requests.get(
            f"{ACCOUNTING_URL}/accounts?type=LIABILITY",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        liabilities = resp.json()
        codes = {a["code"] for a in liabilities}
        expected = ["2000", "2100", "2200", "2300", "2400", "2500", "2600"]
        for code in expected:
            assert code in codes, f"Missing LIABILITY account {code}"

    def test_income_expense_accounts(self, all_accounts):
        """Verify income (4xxx) and expense (5xxx, 6xxx) accounts."""
        codes = {a["code"] for a in all_accounts}
        income_codes = ["4000", "4100", "4200", "4300", "4400"]
        for code in income_codes:
            assert code in codes, f"Missing INCOME account {code}"
        expense_codes = ["5000", "5100", "5200", "5300", "5400", "5500", "5600", "5700", "6000"]
        for code in expense_codes:
            assert code in codes, f"Missing EXPENSE account {code}"

    def test_equity_accounts(self, all_accounts):
        """Verify equity accounts exist."""
        codes = {a["code"] for a in all_accounts}
        for code in ["3000", "3100", "3200"]:
            assert code in codes, f"Missing EQUITY account {code}"


# =========================================================================
# 2. Maker-Checker Workflow (tests 6-15)
# =========================================================================
class TestMakerCheckerWorkflow:
    """Test the maker-checker approval workflow for journal entries."""

    def test_create_journal_entry_starts_as_draft(self, maker_token, cash_account_id, loans_account_id):
        """POST /journal-entries -> entry should have status DRAFT."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=500)
        assert resp.status_code == 201, f"Create entry failed: {resp.status_code} {resp.text[:300]}"
        body = resp.json()
        assert body.get("status") == "DRAFT", (
            f"Expected status DRAFT, got {body.get('status')}"
        )

    def test_submit_entry_for_approval(self, maker_token, cash_account_id, loans_account_id):
        """POST /journal-entries/{id}/submit -> status becomes PENDING_APPROVAL."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=600)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        submit_resp = requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/submit",
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )
        assert submit_resp.status_code == 200, (
            f"Submit failed: {submit_resp.status_code} {submit_resp.text[:300]}"
        )
        assert submit_resp.json().get("status") == "PENDING_APPROVAL"

    def test_approve_entry_by_different_user(self, maker_token, checker_token, cash_account_id, loans_account_id):
        """Officer creates + submits, manager approves -> POSTED."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=700)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        # Submit
        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/submit",
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )

        # Approve by different user
        approve_resp = requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/approve",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )
        assert approve_resp.status_code == 200, (
            f"Approve failed: {approve_resp.status_code} {approve_resp.text[:300]}"
        )
        body = approve_resp.json()
        assert body.get("status") == "POSTED"
        assert body.get("approvedBy") is not None

    def test_self_approval_blocked(self, maker_token, cash_account_id, loans_account_id):
        """Officer creates + submits, then tries to self-approve -> 403 or 422."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=800)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/submit",
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )

        approve_resp = requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/approve",
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )
        assert approve_resp.status_code in (403, 422), (
            f"Self-approval should be blocked, got {approve_resp.status_code}"
        )

    def test_reject_entry(self, maker_token, checker_token, cash_account_id, loans_account_id):
        """Create -> submit -> reject with reason -> REJECTED."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=900)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/submit",
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )

        reject_resp = requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/reject",
            json={"reason": "Incorrect account mapping"},
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )
        assert reject_resp.status_code == 200, (
            f"Reject failed: {reject_resp.status_code} {reject_resp.text[:300]}"
        )
        body = reject_resp.json()
        assert body.get("status") == "REJECTED"
        assert body.get("rejectionReason") is not None

    def test_approve_requires_pending_status(self, maker_token, checker_token, cash_account_id, loans_account_id):
        """Try to approve a DRAFT entry (not submitted) -> 422."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=1100)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        approve_resp = requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/approve",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )
        assert approve_resp.status_code == 422, (
            f"Approve on DRAFT should return 422, got {approve_resp.status_code}"
        )

    def test_reverse_posted_entry(self, maker_token, checker_token, cash_account_id, loans_account_id):
        """Approve entry then reverse -> original becomes REVERSED + mirror entry created."""
        # Create, submit, approve
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=1200)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/submit",
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )
        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/approve",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )

        # Reverse
        reverse_resp = requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/reverse",
            json={"reason": "Duplicate entry"},
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )
        assert reverse_resp.status_code in (200, 201), (
            f"Reverse failed: {reverse_resp.status_code} {reverse_resp.text[:300]}"
        )
        body = reverse_resp.json()
        # The original entry should now be REVERSED
        original = requests.get(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        ).json()
        assert original.get("status") == "REVERSED"

    def test_reverse_creates_mirror_entry(self, maker_token, checker_token, cash_account_id, loans_account_id):
        """Verify the reversal creates a mirror entry with flipped debits/credits."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=1300)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/submit",
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )
        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/approve",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )

        reverse_resp = requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/reverse",
            json={"reason": "Test reversal mirror"},
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )
        assert reverse_resp.status_code in (200, 201)
        reversal = reverse_resp.json()

        # The reversal entry should reference the original
        assert reversal.get("originalEntryId") == entry_id or reversal.get("originalEntryId") is not None

        # Verify lines are flipped: original debit becomes credit and vice versa
        original = requests.get(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        ).json()

        reversal_id = reversal.get("id")
        mirror = requests.get(
            f"{ACCOUNTING_URL}/journal-entries/{reversal_id}",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        ).json()

        orig_lines = sorted(original.get("lines", []), key=lambda l: l.get("lineNo", 0))
        mirror_lines = sorted(mirror.get("lines", []), key=lambda l: l.get("lineNo", 0))

        assert len(mirror_lines) == len(orig_lines), "Mirror entry should have same number of lines"
        for orig_line, mirror_line in zip(orig_lines, mirror_lines):
            # Debits and credits should be swapped
            assert float(mirror_line.get("debitAmount", 0)) == float(orig_line.get("creditAmount", 0))
            assert float(mirror_line.get("creditAmount", 0)) == float(orig_line.get("debitAmount", 0))

    def test_cannot_reverse_draft(self, maker_token, cash_account_id, loans_account_id):
        """Try to reverse a DRAFT entry -> 422."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=1400)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        reverse_resp = requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/reverse",
            json={"reason": "Should fail"},
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )
        assert reverse_resp.status_code == 422, (
            f"Reverse on DRAFT should return 422, got {reverse_resp.status_code}"
        )

    def test_entry_has_sequential_numbers(self, maker_token, cash_account_id, loans_account_id):
        """Create 2 entries and verify entryNumber is sequential."""
        resp1 = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=100)
        assert resp1.status_code == 201
        num1 = resp1.json().get("entryNumber", 0)

        time.sleep(0.1)  # ensure different timestamps

        resp2 = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=200)
        assert resp2.status_code == 201
        num2 = resp2.json().get("entryNumber", 0)

        assert num1 > 0, f"First entry number should be > 0, got {num1}"
        assert num2 > num1, (
            f"Entry numbers should be sequential: {num1} -> {num2}"
        )


# =========================================================================
# 3. Fiscal Periods (tests 16-21)
# =========================================================================
class TestFiscalPeriods:
    """Test fiscal period management — open, close, reopen."""

    def test_list_periods(self, admin_token):
        """GET /periods returns a list (may be empty initially)."""
        resp = requests.get(
            f"{ACCOUNTING_URL}/periods",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200, (
            f"List periods failed: {resp.status_code} {resp.text[:300]}"
        )
        assert isinstance(resp.json(), list)

    def test_close_period(self, admin_token):
        """POST /periods/{year}/{month}/close -> period status becomes CLOSED."""
        # Reopen first in case a previous run closed it
        requests.post(
            f"{ACCOUNTING_URL}/periods/2025/1/reopen",
            json={"reason": "Reset for test"},
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        resp = requests.post(
            f"{ACCOUNTING_URL}/periods/2025/1/close",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200, (
            f"Close period failed: {resp.status_code} {resp.text[:300]}"
        )
        body = resp.json()
        assert body.get("status") == "CLOSED"
        assert body.get("periodYear") == 2025
        assert body.get("periodMonth") == 1

    def test_post_to_closed_period_blocked(self, admin_token, cash_account_id, loans_account_id):
        """Creating an entry dated in a closed period should fail with 422."""
        # Ensure 2025/1 is closed
        requests.post(
            f"{ACCOUNTING_URL}/periods/2025/1/close",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )

        resp = create_test_entry(
            admin_token, cash_account_id, loans_account_id,
            amount=500, entry_date="2025-01-15",
        )
        assert resp.status_code == 422, (
            f"Posting to closed period should return 422, got {resp.status_code}"
        )

    def test_reopen_period(self, admin_token):
        """POST /periods/2025/1/reopen with reason -> status becomes OPEN."""
        # Close first to ensure it's closed
        requests.post(
            f"{ACCOUNTING_URL}/periods/2025/1/close",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )

        resp = requests.post(
            f"{ACCOUNTING_URL}/periods/2025/1/reopen",
            json={"reason": "Correction needed for January adjustments"},
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200, (
            f"Reopen period failed: {resp.status_code} {resp.text[:300]}"
        )
        body = resp.json()
        assert body.get("status") == "OPEN"

    def test_post_after_reopen_succeeds(self, admin_token, cash_account_id, loans_account_id):
        """After reopening a period, posting an entry in that period should succeed."""
        # Ensure period is open
        requests.post(
            f"{ACCOUNTING_URL}/periods/2025/1/reopen",
            json={"reason": "Test reopen for posting"},
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )

        resp = create_test_entry(
            admin_token, cash_account_id, loans_account_id,
            amount=300, entry_date="2025-01-20",
        )
        assert resp.status_code == 201, (
            f"Post after reopen failed: {resp.status_code} {resp.text[:300]}"
        )

    def test_close_already_closed_period(self, admin_token):
        """Closing an already-closed period should return 422."""
        # Close first
        requests.post(
            f"{ACCOUNTING_URL}/periods/2025/1/close",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )

        # Try to close again
        resp = requests.post(
            f"{ACCOUNTING_URL}/periods/2025/1/close",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 422, (
            f"Closing already-closed period should return 422, got {resp.status_code}"
        )


# =========================================================================
# 4. Audit Trail (tests 22-26)
# =========================================================================
class TestAuditTrail:
    """Test the financial audit log captures all actions."""

    def test_audit_log_records_entry_creation(self, admin_token, cash_account_id, loans_account_id):
        """Create an entry and verify CREATE_ENTRY appears in the audit log."""
        resp = create_test_entry(admin_token, cash_account_id, loans_account_id, amount=111)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        time.sleep(0.5)  # allow async log write

        log_resp = requests.get(
            f"{ACCOUNTING_URL}/audit-log",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert log_resp.status_code == 200, (
            f"Audit log fetch failed: {log_resp.status_code} {log_resp.text[:300]}"
        )
        logs = log_resp.json()
        if isinstance(logs, dict):
            logs = logs.get("content", logs.get("items", []))

        create_actions = [
            l for l in logs
            if l.get("action") == "CREATE_ENTRY" and l.get("entityId") == str(entry_id)
        ]
        assert len(create_actions) > 0, (
            f"No CREATE_ENTRY audit log found for entry {entry_id}"
        )

    def test_audit_log_records_approval(self, maker_token, checker_token, cash_account_id, loans_account_id):
        """Approve an entry and verify APPROVE_ENTRY appears in the audit log."""
        resp = create_test_entry(maker_token, cash_account_id, loans_account_id, amount=222)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/submit",
            headers=auth_header(maker_token),
            timeout=TIMEOUT,
        )
        requests.post(
            f"{ACCOUNTING_URL}/journal-entries/{entry_id}/approve",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )

        time.sleep(0.5)

        log_resp = requests.get(
            f"{ACCOUNTING_URL}/audit-log",
            headers=auth_header(checker_token),
            timeout=TIMEOUT,
        )
        assert log_resp.status_code == 200
        logs = log_resp.json()
        if isinstance(logs, dict):
            logs = logs.get("content", logs.get("items", []))

        approve_actions = [
            l for l in logs
            if l.get("action") == "APPROVE_ENTRY" and l.get("entityId") == str(entry_id)
        ]
        assert len(approve_actions) > 0, (
            f"No APPROVE_ENTRY audit log found for entry {entry_id}"
        )

    def test_audit_log_entity_trail(self, admin_token, cash_account_id, loans_account_id):
        """GET /audit-log/JournalEntry/{id} returns the audit trail for a specific entry."""
        resp = create_test_entry(admin_token, cash_account_id, loans_account_id, amount=333)
        assert resp.status_code == 201
        entry_id = resp.json()["id"]

        time.sleep(0.5)

        trail_resp = requests.get(
            f"{ACCOUNTING_URL}/audit-log/JournalEntry/{entry_id}",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert trail_resp.status_code == 200, (
            f"Entity trail fetch failed: {trail_resp.status_code} {trail_resp.text[:300]}"
        )
        trail = trail_resp.json()
        if isinstance(trail, dict):
            trail = trail.get("content", trail.get("items", []))
        assert len(trail) > 0, f"Expected at least one audit log entry for JournalEntry/{entry_id}"

    def test_audit_log_records_period_close(self, admin_token):
        """Close a period and verify CLOSE_PERIOD appears in the audit log."""
        # Use a different month to avoid conflicts with other tests
        requests.post(
            f"{ACCOUNTING_URL}/periods/2025/2/close",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )

        time.sleep(0.5)

        log_resp = requests.get(
            f"{ACCOUNTING_URL}/audit-log",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert log_resp.status_code == 200
        logs = log_resp.json()
        if isinstance(logs, dict):
            logs = logs.get("content", logs.get("items", []))

        close_actions = [l for l in logs if l.get("action") == "CLOSE_PERIOD"]
        assert len(close_actions) > 0, "No CLOSE_PERIOD audit log found"

    def test_audit_log_pagination(self, admin_token):
        """Verify page/size parameters work on the audit log endpoint."""
        resp = requests.get(
            f"{ACCOUNTING_URL}/audit-log?page=0&size=5",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200, (
            f"Audit log pagination failed: {resp.status_code} {resp.text[:300]}"
        )
        body = resp.json()
        # Should respect size limit — either a paginated wrapper or a list
        if isinstance(body, dict):
            items = body.get("content", body.get("items", []))
            assert len(items) <= 5, f"Pagination size=5 returned {len(items)} items"
        elif isinstance(body, list):
            assert len(body) <= 5, f"Pagination size=5 returned {len(body)} items"


# =========================================================================
# 5. System Entries (tests 27-29)
# =========================================================================
class TestSystemEntries:
    """Test system-generated entries and reporting endpoints."""

    def test_system_entries_bypass_approval(self, admin_token):
        """Verify system-generated entries (from events) are POSTED and isSystemGenerated=true."""
        resp = requests.get(
            f"{ACCOUNTING_URL}/journal-entries?page=0&size=50",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200
        body = resp.json()
        entries = body.get("content", body) if isinstance(body, dict) else body

        system_entries = [
            e for e in entries
            if e.get("isSystemGenerated") is True
        ]

        if len(system_entries) == 0:
            pytest.skip("No system-generated entries found — events may not have fired yet")

        for entry in system_entries:
            assert entry.get("status") == "POSTED", (
                f"System entry {entry.get('id')} has status {entry.get('status')}, expected POSTED"
            )
            assert entry.get("isSystemGenerated") is True

    def test_trial_balance_with_period(self, admin_token):
        """GET /trial-balance?year=2026&month=3 returns structured data."""
        resp = requests.get(
            f"{ACCOUNTING_URL}/trial-balance?year=2026&month=3",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200, (
            f"Trial balance failed: {resp.status_code} {resp.text[:300]}"
        )
        body = resp.json()
        assert "periodYear" in body
        assert "periodMonth" in body
        assert "accounts" in body
        assert "totalDebits" in body
        assert "totalCredits" in body
        assert "balanced" in body

    def test_cash_flow_endpoint(self, admin_token):
        """GET /cash-flow returns structured response with operating/investing/financing."""
        resp = requests.get(
            f"{ACCOUNTING_URL}/cash-flow",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200, (
            f"Cash flow failed: {resp.status_code} {resp.text[:300]}"
        )
        body = resp.json()
        assert "operatingItems" in body or "totalOperating" in body, (
            f"Cash flow response missing expected fields: {list(body.keys())}"
        )
        # Verify the three activity categories are present
        for key in ["totalOperating", "totalInvesting", "totalFinancing", "netCashFlow"]:
            assert key in body, f"Cash flow missing '{key}'"


# =========================================================================
# 6. GL Account Ledger (test 30)
# =========================================================================
class TestAccountLedger:
    """Test the GL account ledger endpoint."""

    def test_get_account_ledger(self, admin_token, cash_account_id):
        """GET /accounts/{id}/ledger returns journal lines for the Cash account."""
        resp = requests.get(
            f"{ACCOUNTING_URL}/accounts/{cash_account_id}/ledger",
            headers=auth_header(admin_token),
            timeout=TIMEOUT,
        )
        assert resp.status_code == 200, (
            f"Ledger fetch failed: {resp.status_code} {resp.text[:300]}"
        )
        ledger = resp.json()
        assert isinstance(ledger, list), "Ledger should return a list of journal lines"
        if len(ledger) > 0:
            line = ledger[0]
            assert "accountId" in line
            assert "debitAmount" in line
            assert "creditAmount" in line
            assert "currency" in line
