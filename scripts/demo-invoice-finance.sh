#!/usr/bin/env bash
# End-to-end invoice-finance demo against a running Nemo stack.
#
#   BASE=https://lms.athenafinance.cloud ./scripts/demo-invoice-finance.sh
#   BASE=http://localhost:3001 ./scripts/demo-invoice-finance.sh   # vite dev + compose
#
# Walks the SPEC §8 path: upload sample invoice → OCR → buyer confirms via the
# public link (demo OTP) → scored → decided → offer → accept → fund → buyer
# pays → PAID; then verifies the ledger chain and the public verify page, and
# finally re-uploads the same PDF to show the duplicate rejection.
set -euo pipefail

BASE="${BASE:-https://lms.athenafinance.cloud}"
USER_="${DEMO_USER:-manager}"
PASS_="${DEMO_PASS:-manager123}"
SAMPLE="${SAMPLE:-$(dirname "$0")/../docs/nemo/invoice-finance/samples/invoice-b-ke-second.pdf}"
SAMPLE_DUP="${SAMPLE_DUP:-$SAMPLE}"

jqr() { python3 -c "import sys,json; d=json.load(sys.stdin); print(eval('d'+sys.argv[1]))" "$1"; }
say() { printf '\n\033[1;36m▶ %s\033[0m\n' "$*"; }
api() { curl -sS -H "Authorization: Bearer $TOKEN" "$@"; }

say "Login as $USER_"
TOKEN=$(curl -sS -X POST "$BASE/proxy/auth/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER_\",\"password\":\"$PASS_\"}" | jqr "['token']")
echo "token ok (${#TOKEN} chars)"

say "Pick a seller (SELLER env, else first customer in account-service)"
SELLER="${SELLER:-}"
[[ -n "$SELLER" ]] || SELLER=$(api "$BASE/proxy/accounts/api/v1/customers?page=0&size=1" | python3 -c '
import sys,json; d=json.load(sys.stdin)
items=d.get("content") or d.get("items") or d
c=items[0]; print(c.get("customerId") or c.get("id"))')
echo "sellerCustomerId=$SELLER"

say "Upload $SAMPLE"
UP=$(api -X POST "$BASE/lms/api/v1/invoices" -F "file=@$SAMPLE" -F "sellerCustomerId=$SELLER")
echo "$UP" | python3 -m json.tool | head -40
INV=$(echo "$UP" | jqr "['invoice']['id']")
CONFIRM_URL=$(echo "$UP" | jqr "['confirmUrl']" 2>/dev/null || true)
echo "invoice=$INV"

poll() { # poll <wanted-status-regex> <seconds>
  for i in $(seq 1 "$2"); do
    ST=$(api "$BASE/lms/api/v1/invoices/$INV" | jqr "['invoice']['status']")
    printf '  %s\r' "$ST"
    [[ "$ST" =~ $1 ]] && { echo; return 0; }
    sleep 2
  done; echo; echo "timed out waiting for $1 (last: $ST)"; api "$BASE/lms/api/v1/invoices/$INV" | python3 -m json.tool | head -60; return 1
}

say "Wait for OCR + buyer notification"
poll 'AWAITING_BUYER|BUYER_CONFIRMED|SCORED|DECIDED|OFFERED' 60
api "$BASE/lms/api/v1/invoices/$INV" | python3 -c '
import sys,json; d=json.load(sys.stdin); i=d["invoice"]
print("  number", i.get("invoiceNumber"), "| total", i.get("currency"), i.get("total"), "| due", i.get("dueDate"))
print("  buyer ", i.get("buyer"), "| engine", i.get("ocr",{}).get("engine"))
print("  confirmUrl", d.get("confirmUrl"))'

if [[ -z "${CONFIRM_URL:-}" || "$CONFIRM_URL" == "None" ]]; then
  CONFIRM_URL=$(api "$BASE/lms/api/v1/invoices/$INV" | jqr "['confirmUrl']")
fi
TOKEN_=${CONFIRM_URL##*/}
say "Buyer opens the link and confirms (public, no login; demo OTP 000000)"
curl -sS "$BASE/lms/api/public/invoices/confirm/$TOKEN_" | python3 -m json.tool | head -20
curl -sS -X POST "$BASE/lms/api/public/invoices/confirm/$TOKEN_" -H 'Content-Type: application/json' \
  -d '{"action":"CONFIRMED","otp":"000000"}' | python3 -m json.tool

say "Wait for score → fraud → decision → offer"
poll 'OFFERED|DECIDED|DECLINED' 90
api "$BASE/lms/api/v1/invoices/$INV" | python3 -c '
import sys,json; d=json.load(sys.stdin); dec=d.get("decision") or {}; off=d.get("offer") or {}
print("  decision", dec.get("outcome"), [r.get("code") for r in dec.get("reasons",[])], "policy", dec.get("policyId"), "v", dec.get("policyVersion"), (dec.get("policyHash") or "")[:12])
print("  inputs  ", {k:dec.get("inputs",{}).get(k) for k in ("seller_band","buyer_tier","invoice_score","days_to_due","fraud_flags")})
print("  offer   ", "advance", off.get("advanceRate"), "x =", off.get("advanceAmount"), "| fee/day", off.get("discountFeePerDay"), "| tenor", off.get("tenorDays"), "| net", off.get("netDisbursement"), "| reserve", off.get("reserveAmount"))'

ST=$(api "$BASE/lms/api/v1/invoices/$INV" | jqr "['invoice']['status']")
if [[ "$ST" == "DECIDED" ]]; then
  say "REFER → manager approves (money gate)"
  api -X POST "$BASE/lms/api/v1/invoices/$INV/approve" -H 'Content-Type: application/json' -d '{"note":"demo approval"}' >/dev/null
  poll 'OFFERED' 30
fi

say "Seller accepts the offer"
api -X POST "$BASE/lms/api/v1/invoices/$INV/offer/accept" | jqr "['invoice']['status']"

say "Fund via core-banking adapter"
poll 'FUNDED|ACCEPTED' 60
ST=$(api "$BASE/lms/api/v1/invoices/$INV" | jqr "['invoice']['status']")
if [[ "$ST" == "ACCEPTED" ]]; then
  api -X POST "$BASE/lms/api/v1/invoices/$INV/fund" | python3 -c 'import sys,json; d=json.load(sys.stdin); print("  ", d.get("invoice",{}).get("status"), d.get("advance"))' || true
  poll 'FUNDED' 60
fi
api "$BASE/lms/api/v1/invoices/$INV" | python3 -c '
import sys,json; d=json.load(sys.stdin); a=d.get("advance") or {}
print("  advance", a.get("status"), "provider", a.get("provider"), "coreRef", a.get("coreRef"), "principal", a.get("principal"), "due", a.get("dueDate"))'

say "Buyer pays the full invoice"
TOTAL=$(api "$BASE/lms/api/v1/invoices/$INV" | jqr "['invoice']['total']")
api -X POST "$BASE/lms/api/v1/invoices/$INV/payments" -H 'Content-Type: application/json' \
  -d "{\"amount\":$TOTAL,\"reference\":\"DEMO-PAY-1\",\"payer\":\"buyer\"}" | python3 -c '
import sys,json; d=json.load(sys.stdin); a=d.get("advance") or {}
print("  status", d["invoice"]["status"], "| repaid", a.get("repaid"), "| fees", a.get("feesRepaid"), "| reserve released", a.get("reserveReleased"))'

say "Ledger timeline (hash chain)"
api "$BASE/lms/api/v1/invoices/$INV/timeline" | python3 -c '
import sys,json; d=json.load(sys.stdin)
print("  chainOk =", d.get("chainOk"))
for e in d.get("events",[]): print(f"  #{e[\"seq\"]:<4} {e[\"type\"]:<28} {e[\"occurredAt\"][:19]}  {e[\"hash\"][:16]}…")'

say "Public verify page"
FP=$(api "$BASE/lms/api/v1/invoices/$INV" | jqr "['invoice']['fingerprint']")
curl -sS "$BASE/lms/api/public/invoices/verify/$FP" | python3 -m json.tool
echo "  → $BASE/verify/$FP"

say "Re-upload the same PDF → duplicate rejected"
curl -sS -o /dev/stderr -w '  HTTP %{http_code}\n' -H "Authorization: Bearer $TOKEN" -X POST "$BASE/lms/api/v1/invoices" -F "file=@$SAMPLE_DUP" -F "sellerCustomerId=$SELLER" 2>&1 | tail -3

say "Done. Portal: $BASE/invoice-finance/invoices/$INV"
