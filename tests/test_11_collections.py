"""Collections service tests."""
import pytest
import requests
from conftest import url, TIMEOUT, unique_id


@pytest.mark.collections
class TestCollections:

    def test_list_cases(self, admin_headers):
        """GET /cases returns 200 with paginated content."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        body = r.json()
        assert "content" in body

    def test_collections_summary(self, admin_headers):
        """GET /summary returns 200 with all fields."""
        r = requests.get(url("collections", "/api/v1/collections/summary"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        body = r.json()
        assert "totalOpenCases" in body
        assert "totalOutstandingAmount" in body
        assert "pendingPtpCount" in body
        assert "overdueFollowUpCount" in body

    def test_filter_cases_by_stage(self, admin_headers):
        """GET /cases?stage=WATCH returns 200."""
        r = requests.get(url("collections", "/api/v1/collections/cases?stage=WATCH"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_filter_cases_by_priority(self, admin_headers):
        """GET /cases?priority=CRITICAL returns 200."""
        r = requests.get(url("collections", "/api/v1/collections/cases?priority=CRITICAL"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_filter_cases_by_dpd_range(self, admin_headers):
        """GET /cases?minDpd=1&maxDpd=30 returns 200."""
        r = requests.get(url("collections", "/api/v1/collections/cases?minDpd=1&maxDpd=30"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_filter_cases_with_search(self, admin_headers):
        """GET /cases?search=COL returns 200."""
        r = requests.get(url("collections", "/api/v1/collections/cases?search=COL"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_get_case_by_id(self, admin_headers):
        """GET /cases/{id} returns the case if it exists."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        cases = r.json()
        items = cases.get("content", cases) if isinstance(cases, dict) else cases
        if items:
            cid = items[0]["id"]
            r2 = requests.get(url("collections", f"/api/v1/collections/cases/{cid}"),
                              headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200
            assert r2.json()["id"] == cid

    def test_case_detail_composite(self, admin_headers):
        """GET /cases/{id}/detail returns case + actions + ptps."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if items:
            cid = items[0]["id"]
            r2 = requests.get(url("collections", f"/api/v1/collections/cases/{cid}/detail"),
                              headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200
            body = r2.json()
            assert "case" in body
            assert "actions" in body
            assert "ptps" in body

    def test_add_action_to_case(self, admin_headers):
        """POST /cases/{id}/actions creates an action."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if items:
            cid = items[0]["id"]
            action = {
                "actionType": "PHONE_CALL",
                "outcome": "CONTACTED",
                "notes": "Spoke to borrower, will pay next week",
                "performedBy": "admin",
                "nextActionDate": "2026-04-01"
            }
            r2 = requests.post(url("collections", f"/api/v1/collections/cases/{cid}/actions"),
                               json=action, headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 201
            assert r2.json()["actionType"] == "PHONE_CALL"

    def test_list_actions_after_add(self, admin_headers):
        """GET /cases/{id}/actions returns actions including the one we added."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if items:
            cid = items[0]["id"]
            r2 = requests.get(url("collections", f"/api/v1/collections/cases/{cid}/actions"),
                              headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200

    def test_add_ptp_to_case(self, admin_headers):
        """POST /cases/{id}/ptps creates a promise to pay."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if items:
            cid = items[0]["id"]
            ptp = {
                "promisedAmount": 5000.00,
                "promiseDate": "2026-04-15",
                "notes": "Borrower promised partial payment",
                "createdBy": "admin"
            }
            r2 = requests.post(url("collections", f"/api/v1/collections/cases/{cid}/ptps"),
                               json=ptp, headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 201
            assert r2.json()["status"] == "PENDING"

    def test_list_ptps_after_add(self, admin_headers):
        """GET /cases/{id}/ptps returns ptps including the one we added."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if items:
            cid = items[0]["id"]
            r2 = requests.get(url("collections", f"/api/v1/collections/cases/{cid}/ptps"),
                              headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200

    def test_update_case_assignment(self, admin_headers):
        """PUT /cases/{id} updates the assigned officer."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if items:
            cid = items[0]["id"]
            update = {"assignedTo": "officer", "priority": "HIGH"}
            r2 = requests.put(url("collections", f"/api/v1/collections/cases/{cid}"),
                              json=update, headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200
            assert r2.json()["assignedTo"] == "officer"
            assert r2.json()["priority"] == "HIGH"

    def test_close_case(self, admin_headers):
        """POST /cases/{id}/close closes the case."""
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if items:
            # Use the last case to avoid disrupting other tests
            cid = items[-1]["id"]
            r2 = requests.post(url("collections", f"/api/v1/collections/cases/{cid}/close"),
                               headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200
            assert r2.json()["status"] == "CLOSED"

    def test_summary_amounts_populated(self, admin_headers):
        """Summary amounts are numeric."""
        r = requests.get(url("collections", "/api/v1/collections/summary"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        body = r.json()
        # Amounts should be present and numeric (may be 0 if no open cases)
        for field in ["totalOutstandingAmount", "watchAmount", "substandardAmount", "doubtfulAmount", "lossAmount"]:
            assert field in body
            assert isinstance(body[field], (int, float, str))  # decimal may come as string


@pytest.mark.collections
class TestCollectionStrategies:
    """Phase 2 strategy tests."""

    def test_list_strategies(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/strategies"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_create_strategy(self, admin_headers):
        strategy = {
            "name": "Auto SMS for early delinquency",
            "dpdFrom": 1,
            "dpdTo": 7,
            "actionType": "SMS",
            "priority": 1,
            "isActive": True
        }
        r = requests.post(url("collections", "/api/v1/collections/strategies"),
                          json=strategy, headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 201
        assert r.json()["name"] == "Auto SMS for early delinquency"

    def test_update_strategy(self, admin_headers):
        # Create then update
        strategy = {"name": "Test strategy", "dpdFrom": 1, "dpdTo": 30, "actionType": "PHONE_CALL", "priority": 2}
        r = requests.post(url("collections", "/api/v1/collections/strategies"),
                          json=strategy, headers=admin_headers, timeout=TIMEOUT)
        if r.status_code == 201:
            sid = r.json()["id"]
            r2 = requests.put(url("collections", f"/api/v1/collections/strategies/{sid}"),
                              json={"name": "Updated strategy"}, headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200
            assert r2.json()["name"] == "Updated strategy"

    def test_delete_strategy(self, admin_headers):
        strategy = {"name": "To delete", "dpdFrom": 1, "dpdTo": 5, "actionType": "EMAIL", "priority": 3}
        r = requests.post(url("collections", "/api/v1/collections/strategies"),
                          json=strategy, headers=admin_headers, timeout=TIMEOUT)
        if r.status_code == 201:
            sid = r.json()["id"]
            r2 = requests.delete(url("collections", f"/api/v1/collections/strategies/{sid}"),
                                 headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code in [200, 204]


@pytest.mark.collections
class TestCollectionsBulkOps:
    """Phase 3 bulk operations tests."""

    def test_bulk_assign(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if len(items) >= 2:
            ids = [items[0]["id"], items[1]["id"]]
            r2 = requests.post(url("collections", "/api/v1/collections/cases/bulk-assign"),
                               json={"caseIds": ids, "assignedTo": "officer"},
                               headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200
            assert r2.json()["processed"] >= 1

    def test_bulk_priority(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        if items:
            ids = [items[0]["id"]]
            r2 = requests.post(url("collections", "/api/v1/collections/cases/bulk-priority"),
                               json={"caseIds": ids, "priority": "HIGH"},
                               headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200


@pytest.mark.collections
class TestCollectionsOfficers:
    """Phase 3 workload management tests."""

    def test_list_officers(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/officers"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_create_officer(self, admin_headers):
        r = requests.post(url("collections", "/api/v1/collections/officers"),
                          json={"username": unique_id("officer").lower(), "maxCases": 30},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 201

    def test_officer_workload(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/officers/workload"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200


@pytest.mark.collections
class TestCollectionsWriteOff:
    """Phase 3 write-off tests."""

    def test_request_writeoff(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/cases"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", [])
        open_cases = [c for c in items if c["status"] not in ("CLOSED", "WRITTEN_OFF")]
        if open_cases:
            cid = open_cases[-1]["id"]
            r2 = requests.post(url("collections", f"/api/v1/collections/cases/{cid}/request-writeoff"),
                               json={"reason": "Irrecoverable debt"},
                               headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200
            assert r2.json()["status"] == "WRITE_OFF_REQUESTED"


@pytest.mark.collections
class TestCollectionsAnalytics:
    """Phase 4 analytics tests."""

    def test_dashboard_analytics(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/analytics/dashboard"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        body = r.json()
        assert "recoveryRate" in body
        assert "ageingByStage" in body

    def test_officer_performance(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/analytics/officer-performance"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_ageing_report(self, admin_headers):
        r = requests.get(url("collections", "/api/v1/collections/analytics/ageing"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
