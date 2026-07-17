"""
Nemo E1 — Decision spine v1 (shadow).

An overdraft application must leave a decision.recorded trail: the overdraft
service shadow-evaluates the overdraft.facility policy, writes the record
through its transactional outbox in the same tx as facility creation, and
decision-service projects it into the queryable decision_log.
"""
import random
import uuid

import pytest
import requests
from conftest import url, unique_id, wait_for, TIMEOUT

VALID_OUTCOMES = {"APPROVE", "DECLINE", "REFER", "FLAG", "NO_ACTION"}


def _decision_service_up() -> bool:
    try:
        r = requests.get(url("decision", "/actuator/health"), timeout=5)
        return r.status_code == 200 and r.json().get("status") == "UP"
    except requests.RequestException:
        return False


@pytest.mark.e2e
class TestDecisionLogE2E:

    def test_overdraft_application_records_decision(self, admin_headers):
        if not _decision_service_up():
            pytest.skip("decision-service not running (stack predates E1)")

        # 1. Customer with a numeric id (the scoring service keys on int64).
        cid = str(random.randint(10_000_000, 99_999_999))
        r = requests.post(url("account", "/api/v1/customers"), headers=admin_headers,
                          json={"customerId": cid, "firstName": "Decision", "lastName": "Spine",
                                "email": f"e1-{cid}@e2e.test", "phone": "+254700000034",
                                "customerType": "INDIVIDUAL", "status": "ACTIVE"},
                          timeout=TIMEOUT)
        assert r.status_code == 201, f"Customer create: {r.status_code} {r.text}"

        # 2. Wallet.
        r = requests.post(url("overdraft", "/api/v1/wallets"),
                          json={"customerId": cid, "currency": "KES"},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code in (200, 201), f"Wallet create: {r.status_code} {r.text}"
        wallet_id = r.json()["id"]

        # 3. Score the customer (falls back to the deterministic mock when the
        # external scoring API is down — which the shadow decision records as
        # MODEL_UNAVAILABLE rather than trusting).
        r = requests.post(url("scoring", "/api/v1/scoring/requests"),
                          json={"loanApplicationId": str(uuid.uuid4()),
                                "customerId": int(cid), "triggerEvent": "MANUAL"},
                          headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code in (200, 201), f"Scoring trigger: {r.status_code} {r.text}"

        def score_ready():
            resp = requests.get(url("scoring", f"/api/v1/scoring/customers/{cid}/latest"),
                                headers=admin_headers, timeout=TIMEOUT)
            return resp.status_code == 200
        wait_for(score_ready, retries=10, delay=2, desc="scoring result")

        # 4. Apply for the overdraft (legacy money path decides).
        r = requests.post(url("overdraft", f"/api/v1/wallets/{wallet_id}/overdraft/apply"),
                          json={}, headers=admin_headers, timeout=TIMEOUT)
        assert r.status_code in (200, 201), f"Overdraft apply: {r.status_code} {r.text}"

        # 5. The shadow decision must land in the decision_log projection
        # (outbox relay -> RabbitMQ -> decision-service, so allow for lag).
        def find_decision():
            resp = requests.get(url("decision", "/api/v1/decisions"),
                                params={"subjectId": wallet_id,
                                        "decisionType": "overdraft.facility"},
                                headers=admin_headers, timeout=TIMEOUT)
            if resp.status_code != 200:
                return None
            content = resp.json().get("content") or []
            return content[0] if content else None

        decision = wait_for(find_decision, retries=15, delay=2,
                            desc="decision.recorded projection for overdraft application")

        # 6. The record answers who/what/inputs/policy/outcome in one query.
        assert decision["decisionType"] == "overdraft.facility"
        assert decision["subjectType"] == "wallet"
        assert decision["subjectId"] == wallet_id
        assert decision["actorType"] == "SYSTEM"
        assert decision["policyId"] == "overdraft.facility"
        assert decision["policyVersion"] >= 1
        assert decision["policyHash"].startswith("sha256:")
        assert decision["outcome"] in VALID_OUTCOMES
        assert decision["variant"] == "champion"
        assert decision["inputs"].get("band"), "inputs snapshot missing the score band"
        models = decision["models"]
        assert models and models[0]["name"] == "credit_score"
        # v1 is SHADOW: whatever the shadow outcome, the money path already
        # decided above (step 4 succeeded) — the log observes, never enforces.
