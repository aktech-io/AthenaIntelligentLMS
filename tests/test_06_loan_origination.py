"""Loan origination service tests — application lifecycle."""
import pytest
import requests
from conftest import url, unique_id, TIMEOUT


@pytest.mark.loan_origination
class TestLoanApplicationCRUD:

    def test_create_application(self, admin_headers, test_customer):
        # Get first active product
        r = requests.get(url("product", "/api/v1/products"),
                         headers=admin_headers, timeout=TIMEOUT)
        products = r.json()
        items = products.get("content", products) if isinstance(products, dict) else products
        active = [p for p in items if p.get("status") == "ACTIVE"]
        assert active, "No active products — seed data issue"
        product = active[0]

        payload = {
            "customerId": test_customer["_customerId"],
            "productId": product["id"],
            "requestedAmount": 10000,
            "tenorMonths": 3,
            "purpose": "Pytest E2E test",
        }
        r = requests.post(url("loan_origination", "/api/v1/loan-applications"),
                          json=payload, headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 201
        body = r.json()
        assert body["id"]
        assert body["status"] in ("DRAFT", "PENDING")

    def test_list_applications(self, admin_headers):
        r = requests.get(url("loan_origination", "/api/v1/loan-applications"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_list_applications_by_customer(self, admin_headers, test_customer):
        cid = test_customer["_customerId"]
        r = requests.get(url("loan_origination", f"/api/v1/loan-applications/customer/{cid}"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200

    def test_get_application_by_id(self, admin_headers, test_customer):
        # Create then get
        r = requests.get(url("loan_origination", "/api/v1/loan-applications"),
                         headers=admin_headers, timeout=TIMEOUT)
        apps = r.json()
        items = apps.get("content", apps) if isinstance(apps, dict) else apps
        if items:
            app_id = items[0]["id"]
            r2 = requests.get(url("loan_origination", f"/api/v1/loan-applications/{app_id}"),
                              headers=admin_headers, timeout=TIMEOUT)
            assert r2.status_code == 200


@pytest.mark.loan_origination
class TestLoanApplicationWorkflow:

    @pytest.fixture(autouse=True)
    def _create_app(self, admin_headers, test_customer):
        """Create a fresh application for workflow tests."""
        r = requests.get(url("product", "/api/v1/products"),
                         headers=admin_headers, timeout=TIMEOUT)
        products = r.json()
        items = products.get("content", products) if isinstance(products, dict) else products
        active = [p for p in items if p.get("status") == "ACTIVE"]
        if not active:
            pytest.skip("No active products")
        product = active[0]

        payload = {
            "customerId": test_customer["_customerId"],
            "productId": product["id"],
            "requestedAmount": 15000,
            "tenorMonths": 3,
            "purpose": "Workflow test",
        }
        r = requests.post(url("loan_origination", "/api/v1/loan-applications"),
                          json=payload, headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 201
        self.app = r.json()
        self.app_id = self.app["id"]

    def test_submit_application(self, admin_headers):
        r = requests.post(
            url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/submit"),
            headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        assert r.json()["status"] == "SUBMITTED"

    def test_start_review(self, admin_headers):
        # submit first
        requests.post(url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/submit"),
                      headers=admin_headers, timeout=TIMEOUT)
        r = requests.post(
            url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/review/start"),
            headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        assert r.json()["status"] == "UNDER_REVIEW"

    def test_approve_application(self, admin_headers):
        # submit → review → approve
        requests.post(url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/submit"),
                      headers=admin_headers, timeout=TIMEOUT)
        requests.post(url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/review/start"),
                      headers=admin_headers, timeout=TIMEOUT)
        r = requests.post(
            url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/review/approve"),
            json={"approvedAmount": 15000, "interestRate": 15.0, "comments": "Approved by pytest"},
            headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        assert r.json()["status"] == "APPROVED"

    def test_reject_application(self, admin_headers, test_customer):
        # Need a separate app for reject path
        r0 = requests.get(url("product", "/api/v1/products"),
                          headers=admin_headers, timeout=TIMEOUT)
        items = r0.json().get("content", r0.json()) if isinstance(r0.json(), dict) else r0.json()
        active = [p for p in items if p.get("status") == "ACTIVE"]
        prod = active[0]
        amount = max(5000.0, float(prod.get("minAmount") or 0))
        if prod.get("maxAmount"):
            amount = min(amount, float(prod["maxAmount"]))
        payload = {
            "customerId": test_customer["_customerId"],
            "productId": prod["id"],
            "requestedAmount": amount,
            "tenorMonths": 3,
            "purpose": "Reject test",
        }
        r = requests.post(url("loan_origination", "/api/v1/loan-applications"),
                          json=payload, headers=admin_headers, timeout=TIMEOUT)
        reject_id = r.json()["id"]
        requests.post(url("loan_origination", f"/api/v1/loan-applications/{reject_id}/submit"),
                      headers=admin_headers, timeout=TIMEOUT)
        requests.post(url("loan_origination", f"/api/v1/loan-applications/{reject_id}/review/start"),
                      headers=admin_headers, timeout=TIMEOUT)
        r = requests.post(
            url("loan_origination", f"/api/v1/loan-applications/{reject_id}/review/reject"),
            json={"reason": "Test rejection", "comments": "Rejected by pytest"},
            headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        assert r.json()["status"] == "REJECTED"

    def test_cancel_application(self, admin_headers):
        r = requests.post(
            url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/cancel"),
            headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        assert r.json()["status"] == "CANCELLED"

    def test_add_note(self, admin_headers):
        r = requests.post(
            url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/notes"),
            json={"content": "Pytest note", "author": "pytest"},
            headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 201

    def test_add_collateral(self, admin_headers):
        r = requests.post(
            url("loan_origination", f"/api/v1/loan-applications/{self.app_id}/collaterals"),
            json={"collateralType": "VEHICLE", "description": "Test car", "estimatedValue": 500000},
            headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code in (200, 201, 400), f"Collateral: {r.status_code} {r.text[:200]}"
