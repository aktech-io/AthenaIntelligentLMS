"""
Seed a single customer with ~1 year of realistic financial activity.
Creates: customer, 2 accounts, wallet, overdraft, transactions, loans, repayments, fraud alerts.
"""
import os
import sys
import uuid
import random
import requests
from datetime import datetime, timedelta

BASE = os.getenv("LMS_BASE", "http://localhost")
ACCT = f"{BASE}:28086"
LOAN_ORIG = f"{BASE}:28088"
LOAN_MGMT = f"{BASE}:28089"
OVERDRAFT = f"{BASE}:28097"
FRAUD = f"{BASE}:28100"
PRODUCT = f"{BASE}:28087"
T = 15

# ── Auth ─────────────────────────────────────────────────────────────
print("Logging in...")
r = requests.post(f"{ACCT}/api/auth/login",
                  json={"username": "admin",
                        "password": os.getenv("LMS_ADMIN_PASSWORD", "admin123")}, timeout=T)
assert r.status_code == 200, f"Login failed: {r.text}"
token = r.json()["token"]
H = {"Content-Type": "application/json", "Authorization": f"Bearer {token}"}

CID = "CUST-360DEMO"

# ── 1. Customer ──────────────────────────────────────────────────────
print(f"Creating customer {CID}...")
r = requests.post(f"{ACCT}/api/v1/customers", json={
    "customerId": CID,
    "firstName": "Jane",
    "lastName": "Muthoni",
    "email": "jane.muthoni@example.co.ke",
    "phone": "+254722360360",
    "customerType": "INDIVIDUAL",
    "nationalId": "30012345",
    "gender": "FEMALE",
    "address": "42 Kenyatta Ave, Nairobi",
    "dateOfBirth": "1990-06-15",
}, headers=H, timeout=T)
if r.status_code == 201:
    cust = r.json()
    print(f"  Created: {cust['id']}")
elif "already exists" in r.text.lower() or "duplicate" in r.text.lower() or r.status_code == 409:
    # Fetch existing
    rl = requests.get(f"{ACCT}/api/v1/customers/search?q={CID}", headers=H, timeout=T)
    cust = [c for c in (rl.json() or []) if c["customerId"] == CID][0]
    print(f"  Already exists: {cust['id']}")
else:
    print(f"  Customer create: {r.status_code} {r.text[:200]}")
    sys.exit(1)

CUST_UUID = cust["id"]

# ── 2. Accounts ──────────────────────────────────────────────────────
def create_account(acct_type, currency="KES"):
    r = requests.post(f"{ACCT}/api/v1/accounts", json={
        "customerId": CID,
        "accountType": acct_type,
        "currency": currency,
    }, headers=H, timeout=T)
    if r.status_code == 201:
        return r.json()
    # Check if already exists
    rl = requests.get(f"{ACCT}/api/v1/accounts/customer/{CID}", headers=H, timeout=T)
    for a in (rl.json() or []):
        if a["accountType"] == acct_type and a.get("currency", "KES") == currency:
            return a
    print(f"  Account create ({acct_type}): {r.status_code} {r.text[:200]}")
    return None

print("Creating accounts...")
savings = create_account("SAVINGS")
current = create_account("CURRENT")
print(f"  Savings: {savings['id'] if savings else 'FAILED'}")
print(f"  Current: {current['id'] if current else 'FAILED'}")

# ── 3. Simulate 1 year of transactions ──────────────────────────────
print("Seeding transactions over 12 months...")
now = datetime.now()
start = now - timedelta(days=365)

def credit(account_id, amount, desc, ref=None):
    requests.post(f"{ACCT}/api/v1/accounts/{account_id}/credit", json={
        "amount": amount,
        "description": desc,
        "reference": ref or f"REF-{uuid.uuid4().hex[:8].upper()}",
    }, headers=H, timeout=T)

def debit(account_id, amount, desc, ref=None):
    requests.post(f"{ACCT}/api/v1/accounts/{account_id}/debit", json={
        "amount": amount,
        "description": desc,
        "reference": ref or f"REF-{uuid.uuid4().hex[:8].upper()}",
    }, headers=H, timeout=T)

txn_count = 0

# Initial salary deposit
if savings:
    credit(savings["id"], 150000, "Initial salary deposit")
    txn_count += 1

if current:
    credit(current["id"], 50000, "Opening balance transfer")
    txn_count += 1

# Monthly cycle for 12 months
for month in range(12):
    dt = start + timedelta(days=month * 30)
    salary = random.randint(85000, 120000)

    if savings:
        # Salary credit
        credit(savings["id"], salary, f"Salary - Month {month+1}", f"SAL-{month+1:02d}-{CID}")
        txn_count += 1

        # Rent payment
        debit(savings["id"], random.randint(15000, 25000), f"Rent payment month {month+1}")
        txn_count += 1

        # Utility bills
        debit(savings["id"], random.randint(2000, 5000), "KPLC electricity")
        debit(savings["id"], random.randint(500, 2000), "Nairobi Water")
        txn_count += 2

        # Grocery / M-Pesa
        for _ in range(random.randint(3, 8)):
            debit(savings["id"], random.randint(200, 5000), random.choice([
                "Naivas Supermarket", "Carrefour", "M-Pesa Send Money",
                "Safaricom Airtime", "Uber Ride", "Jumia Online", "School fees",
                "Pharmacy", "Fuel station", "Restaurant",
            ]))
            txn_count += 1

        # Occasional incoming transfers
        if random.random() > 0.6:
            credit(savings["id"], random.randint(5000, 30000), random.choice([
                "Freelance payment", "M-Pesa received", "Dividend income",
                "Refund", "Gift from family",
            ]))
            txn_count += 1

    if current:
        # Business transactions
        credit(current["id"], random.randint(20000, 80000), f"Business revenue month {month+1}")
        debit(current["id"], random.randint(10000, 40000), f"Supplier payment month {month+1}")
        txn_count += 2

        if random.random() > 0.5:
            debit(current["id"], random.randint(5000, 20000), "Business expense - misc")
            txn_count += 1

    if txn_count % 20 == 0:
        print(f"  ... {txn_count} transactions created")

print(f"  Total transactions: {txn_count}")

# ── 4. Wallet & Overdraft ────────────────────────────────────────────
print("Creating wallet & overdraft...")
r = requests.post(f"{OVERDRAFT}/api/v1/wallets", json={
    "customerId": CID, "currency": "KES",
}, headers=H, timeout=T)
if r.status_code in (200, 201):
    wallet = r.json()
    print(f"  Wallet: {wallet['id']}")
else:
    # Try to fetch existing
    r2 = requests.get(f"{OVERDRAFT}/api/v1/wallets/customer/{CID}", headers=H, timeout=T)
    if r2.status_code == 200:
        wallet = r2.json()
        print(f"  Wallet exists: {wallet['id']}")
    else:
        wallet = None
        print(f"  Wallet: skipped ({r.status_code})")

if wallet:
    # Fund wallet
    for i in range(8):
        amt = random.randint(5000, 25000)
        requests.post(f"{OVERDRAFT}/api/v1/wallets/{wallet['id']}/deposit", json={
            "amount": amt, "reference": f"WDEP-{i+1:02d}-{CID}",
        }, headers=H, timeout=T)

    for i in range(5):
        amt = random.randint(1000, 10000)
        requests.post(f"{OVERDRAFT}/api/v1/wallets/{wallet['id']}/withdraw", json={
            "amount": amt, "reference": f"WWTH-{i+1:02d}-{CID}",
        }, headers=H, timeout=T)

    # Apply for overdraft
    r = requests.post(f"{OVERDRAFT}/api/v1/wallets/{wallet['id']}/overdraft/apply",
                       json={}, headers=H, timeout=T)
    if r.status_code in (200, 201):
        print(f"  Overdraft approved: limit={r.json().get('approvedLimit', '?')}")
    else:
        print(f"  Overdraft: {r.status_code} (may already exist)")

# ── 5. Loan Products ────────────────────────────────────────────────
print("Checking loan products...")
r = requests.get(f"{PRODUCT}/api/v1/products?page=0&size=5", headers=H, timeout=T)
products = []
if r.status_code == 200:
    body = r.json()
    products = body.get("content", body) if isinstance(body, dict) else body
    if products:
        print(f"  Found {len(products)} products")

# ── 6. Loans & Repayments ───────────────────────────────────────────
print("Creating loans...")
loan_ids = []

# Try origination flow first, fall back to direct management
for i, (amount, tenor) in enumerate([(50000, 6), (120000, 12), (30000, 3)]):
    product_id = products[i % len(products)]["id"] if products else None

    # Try via origination
    app_payload = {
        "customerId": CID,
        "requestedAmount": amount,
        "tenorMonths": tenor,
        "currency": "KES",
        "purpose": random.choice(["Business expansion", "Education", "Emergency", "Home improvement"]),
    }
    if product_id:
        app_payload["productId"] = product_id

    r = requests.post(f"{LOAN_ORIG}/api/v1/loan-applications", json=app_payload, headers=H, timeout=T)
    if r.status_code in (200, 201):
        app = r.json()
        app_id = app.get("id")
        print(f"  Application {i+1}: {app_id}")

        # Submit
        requests.post(f"{LOAN_ORIG}/api/v1/loan-applications/{app_id}/submit",
                      json={}, headers=H, timeout=T)

        # Start review
        requests.post(f"{LOAN_ORIG}/api/v1/loan-applications/{app_id}/review/start",
                      json={}, headers=H, timeout=T)

        # Approve
        requests.post(f"{LOAN_ORIG}/api/v1/loan-applications/{app_id}/review/approve",
                      json={"approvedAmount": amount, "comments": "Auto-approved for demo"},
                      headers=H, timeout=T)

        # Disburse (via origination)
        disburse_payload = {"disbursedAmount": amount}
        if savings:
            disburse_payload["disbursementAccountId"] = savings["id"]
        rd = requests.post(f"{LOAN_ORIG}/api/v1/loan-applications/{app_id}/disburse",
                          json=disburse_payload, headers=H, timeout=T)
        if rd.status_code in (200, 201):
            loan = rd.json()
            # The loan ID may be in the response or we need to find it
            lid = loan.get("loanId") or loan.get("id")
            if lid:
                loan_ids.append(lid)
            print(f"  Loan {i+1} disbursed: {lid}")
        else:
            print(f"  Disburse: {rd.status_code} {rd.text[:150]}")
            # Try to find the loan via management
            rl = requests.get(f"{LOAN_MGMT}/api/v1/loans?customerId={CID}&size=10", headers=H, timeout=T)
            if rl.status_code == 200:
                loans = rl.json().get("content", [])
                for l in loans:
                    if l["id"] not in loan_ids:
                        loan_ids.append(l["id"])
                        print(f"  Found loan: {l['id']}")
    else:
        print(f"  Application {i+1}: {r.status_code} {r.text[:150]}")

# Make repayments on loans
print("Making loan repayments...")
for lid in loan_ids:
    # Get loan details
    rl = requests.get(f"{LOAN_MGMT}/api/v1/loans/{lid}", headers=H, timeout=T)
    if rl.status_code != 200:
        continue
    loan = rl.json()

    # Make 3-6 repayments
    num_payments = random.randint(3, 6)
    for p in range(num_payments):
        pay_amt = random.randint(5000, 20000)
        rp = requests.post(f"{LOAN_MGMT}/api/v1/repayments", json={
            "loanId": lid,
            "amount": pay_amt,
            "paymentMethod": random.choice(["MPESA", "BANK_TRANSFER", "CASH"]),
            "reference": f"PAY-{lid[:8]}-{p+1:02d}",
        }, headers=H, timeout=T)
        if rp.status_code in (200, 201):
            print(f"  Repayment on {lid[:8]}...: KES {pay_amt:,}")

# ── 7. Fraud events & alerts ────────────────────────────────────────
print("Creating fraud events...")
fraud_events = [
    {"eventType": "TRANSACTION", "amount": 95000, "channel": "MOBILE",
     "description": "Large mobile transfer to new beneficiary"},
    {"eventType": "TRANSACTION", "amount": 48000, "channel": "ONLINE",
     "description": "Online purchase from flagged merchant"},
    {"eventType": "VELOCITY", "amount": 15000, "channel": "ATM",
     "description": "Multiple ATM withdrawals in short period"},
    {"eventType": "TRANSACTION", "amount": 200000, "channel": "BRANCH",
     "description": "Cash deposit above reporting threshold"},
    {"eventType": "LOGIN", "amount": 0, "channel": "MOBILE",
     "description": "Login from new device and unusual location"},
]

for evt in fraud_events:
    payload = {
        "customerId": CID,
        "eventType": evt["eventType"],
        "amount": evt["amount"],
        "currency": "KES",
        "channel": evt["channel"],
        "description": evt["description"],
        "sourceIp": f"41.89.{random.randint(1,254)}.{random.randint(1,254)}",
    }
    if savings:
        payload["accountId"] = savings["id"]

    r = requests.post(f"{FRAUD}/api/v1/fraud/evaluate", json=payload, headers=H, timeout=T)
    if r.status_code in (200, 201):
        print(f"  Fraud event: {evt['eventType']} — {evt['description'][:50]}")
    else:
        print(f"  Fraud event {evt['eventType']}: {r.status_code} {r.text[:100]}")

# Try scoring
print("Requesting fraud scoring...")
for _ in range(3):
    r = requests.post(f"{FRAUD}/api/v1/fraud/evaluate", json={
        "customerId": CID,
        "eventType": "TRANSACTION",
        "amount": random.randint(10000, 100000),
        "channel": "MOBILE",
    }, headers=H, timeout=T)
    if r.status_code in (200, 201):
        score = r.json()
        print(f"  Score: {score.get('riskLevel', '?')} ({score.get('riskScore', '?')})")

# ── Done ─────────────────────────────────────────────────────────────
print("\n" + "=" * 60)
print(f"CUSTOMER ID (for URL):  {CUST_UUID}")
print(f"CUSTOMER CODE:          {CID}")
print(f"URL: http://localhost:3001/customer/{CUST_UUID}")
print("=" * 60)
