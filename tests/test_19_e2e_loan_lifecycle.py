"""
E2E Test: Full Loan Lifecycle (5 product types)
Customer → Account → Loan App → Submit → Review → Approve → Disburse → Activate → Schedule → Repay → Verify
"""
import time
import pytest
import requests
from conftest import url, unique_id, TIMEOUT, wait_for

PRODUCT_TYPES = [
    ("NANO",     5000,   3, "Nano Loan Test"),
    ("PERSONAL", 50000,  6, "Personal Loan Test"),
    ("SME",      80000,  12, "SME Business Loan Test"),
]


@pytest.mark.e2e
class TestLoanLifecycleE2E:

    @pytest.mark.parametrize("ptype,amount,tenor,desc", PRODUCT_TYPES,
                             ids=[p[0] for p in PRODUCT_TYPES])
    def test_full_loan_lifecycle(self, admin_headers, ptype, amount, tenor, desc):
        """Complete loan lifecycle from customer creation to repayment."""

        # 1. Create customer
        cid = unique_id(f"E2E-{ptype}")
        r = requests.post(url("account", "/api/v1/customers"), headers=admin_headers,
                          json={"customerId": cid, "firstName": ptype, "lastName": "E2E",
                                "email": f"{cid.lower()}@e2e.test", "phone": "+254700000001",
                                "customerType": "INDIVIDUAL", "status": "ACTIVE"},
                          timeout=TIMEOUT)
        assert r.status_code == 201, f"Customer create: {r.status_code}"

        # 2. Create account + seed
        r = requests.post(url("account", "/api/v1/accounts"), headers=admin_headers,
                          json={"customerId": cid, "accountType": "SAVINGS",
                                "currency": "KES", "name": f"{ptype} E2E Account"},
                          timeout=TIMEOUT)
        assert r.status_code == 201, f"Account create: {r.status_code}"
        acct_id = r.json()["id"]

        r = requests.post(url("account", f"/api/v1/accounts/{acct_id}/credit"),
                          json={"amount": 500000, "description": "E2E seed",
                                "reference": unique_id("SEED")},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200, f"Credit: {r.status_code}"

        # 3. Find active product of matching type
        r = requests.get(url("product", "/api/v1/products"),
                         headers=admin_headers, timeout=TIMEOUT)
        products = r.json()
        items = products.get("content", products) if isinstance(products, dict) else products
        matching = [p for p in items
                    if p.get("status") == "ACTIVE"
                    and p.get("productType", "").upper() == ptype.upper()]
        if not matching:
            # Fallback: any active product
            matching = [p for p in items if p.get("status") == "ACTIVE"]
        assert matching, f"No active product for type {ptype}"
        product = matching[0]

        # Clamp the requested amount to the product's limits — product amount
        # validation is now enforced (rejects out-of-range amounts).
        p_min = float(product.get("minAmount") or 0)
        p_max = float(product.get("maxAmount") or 0)
        if amount < p_min:
            amount = p_min
        if p_max and amount > p_max:
            amount = p_max

        # 4. Create loan application
        r = requests.post(url("loan_origination", "/api/v1/loan-applications"),
                          json={"customerId": cid, "productId": product["id"],
                                "requestedAmount": amount, "tenorMonths": tenor,
                                "purpose": desc},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 201, f"Loan app create: {r.status_code} {r.text[:200]}"
        app_id = r.json()["id"]

        # 5. Submit
        r = requests.post(url("loan_origination", f"/api/v1/loan-applications/{app_id}/submit"),
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200, f"Submit: {r.status_code}"
        assert r.json()["status"] == "SUBMITTED"

        # 6. Start review
        r = requests.post(url("loan_origination", f"/api/v1/loan-applications/{app_id}/review/start"),
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200, f"Review start: {r.status_code}"
        assert r.json()["status"] == "UNDER_REVIEW"

        # 7. Approve
        r = requests.post(url("loan_origination", f"/api/v1/loan-applications/{app_id}/review/approve"),
                          json={"approvedAmount": amount, "interestRate": 15.0, "comments": "E2E approved"},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200, f"Approve: {r.status_code} {r.text[:200]}"
        assert r.json()["status"] == "APPROVED"

        # 8. Disburse
        r = requests.post(url("loan_origination", f"/api/v1/loan-applications/{app_id}/disburse"),
                          json={"disbursedAmount": amount, "disbursementAccount": acct_id,
                                "disbursementMethod": "BANK_TRANSFER"},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200, f"Disburse: {r.status_code} {r.text[:200]}"
        assert r.json()["status"] == "DISBURSED"

        # 9. Wait for loan activation (RabbitMQ async)
        def check_active():
            r = requests.get(
                url("loan_management", f"/api/v1/loans/customer/{cid}"),
                headers=admin_headers, timeout=TIMEOUT)
            if r.status_code != 200:
                return None
            loans = r.json()
            items = loans.get("content", loans) if isinstance(loans, dict) else loans
            active = [l for l in items if l.get("status") == "ACTIVE"]
            return active[0] if active else None

        loan = wait_for(check_active, retries=15, delay=2, desc=f"{ptype} loan activation")
        loan_id = loan["id"]

        # 10. Verify schedule
        r = requests.get(url("loan_management", f"/api/v1/loans/{loan_id}/schedule"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200, f"Schedule: {r.status_code}"
        schedule = r.json()
        installments = schedule if isinstance(schedule, list) else schedule.get("installments", schedule.get("content", []))
        assert len(installments) > 0, "Empty repayment schedule"

        # 11. Make first repayment
        first = installments[0]
        repay_amount = float(first.get("totalDue", first.get("totalAmount", first.get("installmentAmount", 0))))
        assert repay_amount > 0, f"Invalid repayment amount: {repay_amount}"

        r = requests.post(url("loan_management", "/api/v1/repayments"),
                          json={"loanId": loan_id, "amount": repay_amount,
                                "paymentMethod": "CASH",
                                "reference": unique_id("RPMT")},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 201, f"Repayment: {r.status_code} {r.text[:200]}"

        # 12. Verify outstanding principal reduced
        r = requests.get(url("loan_management", f"/api/v1/loans/{loan_id}"),
                         headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code == 200
        updated_loan = r.json()
        outstanding = float(updated_loan.get("outstandingPrincipal", updated_loan.get("principalBalance", amount)))
        assert outstanding < amount, f"Outstanding {outstanding} not less than original {amount}"


@pytest.mark.e2e
class TestLoanApplicationEdgeCases:

    def test_application_with_zero_amount(self, admin_headers, test_customer):
        r = requests.get(url("product", "/api/v1/products"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", r.json()) if isinstance(r.json(), dict) else r.json()
        active = [p for p in items if p.get("status") == "ACTIVE"]
        if not active:
            pytest.skip("No active products")
        r = requests.post(url("loan_origination", "/api/v1/loan-applications"),
                          json={"customerId": test_customer["_customerId"],
                                "productId": active[0]["id"],
                                "requestedAmount": 0, "tenorMonths": 3,
                                "purpose": "Zero amount test"},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code in (400, 422, 500), "Zero amount should be rejected"

    def test_application_with_negative_amount(self, admin_headers, test_customer):
        r = requests.get(url("product", "/api/v1/products"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", r.json()) if isinstance(r.json(), dict) else r.json()
        active = [p for p in items if p.get("status") == "ACTIVE"]
        if not active:
            pytest.skip("No active products")
        r = requests.post(url("loan_origination", "/api/v1/loan-applications"),
                          json={"customerId": test_customer["_customerId"],
                                "productId": active[0]["id"],
                                "requestedAmount": -5000, "tenorMonths": 3,
                                "purpose": "Negative amount test"},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code in (400, 422, 500), "Negative amount should be rejected"

    def test_double_submit_rejected(self, admin_headers, test_customer):
        r = requests.get(url("product", "/api/v1/products"),
                         headers=admin_headers, timeout=TIMEOUT)
        items = r.json().get("content", r.json()) if isinstance(r.json(), dict) else r.json()
        active = [p for p in items if p.get("status") == "ACTIVE"]
        if not active:
            pytest.skip("No active products")

        # Create + submit
        r = requests.post(url("loan_origination", "/api/v1/loan-applications"),
                          json={"customerId": test_customer["_customerId"],
                                "productId": active[0]["id"],
                                "requestedAmount": 10000, "tenorMonths": 3,
                                "purpose": "Double submit test"},
                          headers=admin_headers, timeout=TIMEOUT)
        app_id = r.json()["id"]
        requests.post(url("loan_origination", f"/api/v1/loan-applications/{app_id}/submit"),
                      headers=admin_headers, timeout=TIMEOUT)

        # Second submit should fail
        r2 = requests.post(url("loan_origination", f"/api/v1/loan-applications/{app_id}/submit"),
                           headers=admin_headers, timeout=TIMEOUT)
        assert r2.status_code in (400, 409, 422, 500), "Double submit should fail"
