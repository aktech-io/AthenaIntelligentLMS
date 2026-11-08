"""Invoice extraction tests.

Unit tests are pure logic on synthetic word lists (engine.invoice imports
nothing but engine.fields). The endpoint tests need FastAPI + PyMuPDF and the
generated sample PDFs; the text-layer path always runs when those are
installed, the scanned-PDF path runs only when an OCR engine is provisioned
(skips otherwise).
"""
from __future__ import annotations

import os

import pytest

from engine.fields import OcrWord
from engine.invoice import (
    FIELD_NAMES,
    extract_invoice,
    parse_amount,
    parse_date,
)

HERE = os.path.dirname(os.path.abspath(__file__))
SAMPLES = os.path.normpath(
    os.path.join(HERE, "..", "..", "docs", "nemo", "invoice-finance", "samples")
)


def words_from_lines(*lines: str, conf: float = 0.9) -> list[OcrWord]:
    out = []
    for i, line in enumerate(lines):
        for tok in line.split():
            out.append(OcrWord(text=tok, confidence=conf, line=i))
    return out


def run(*lines: str, conf: float = 0.9):
    words = words_from_lines(*lines, conf=conf)
    return extract_invoice(words, "\n".join(lines), engine="test")


KE_INVOICE = (
    "Mavuno Agro Supplies Ltd TAX INVOICE",
    "Enterprise Road, Industrial Area",
    "P.O. Box 41822-00100, Nairobi, Kenya",
    "KRA PIN: P051234567K",
    "Email: accounts@mavunoagro.co.ke Tel: +254 20 555 0142",
    "Bill To: Invoice No: MAS-2026-0147",
    "Nakuru Fresh Mart Ltd Invoice Date: 05 Nov 2026",
    "Kenyatta Avenue, Nakuru Due Date: 05 Dec 2026",
    "P.O. Box 1180-20100, Nakuru, Kenya Payment Terms: Net 30",
    "PIN: P051998877Q Currency: KES",
    "Email: payables@nakurufreshmart.co.ke",
    "Tel: +254 722 019 338",
    "Description Qty Unit Price (KES) Amount (KES)",
    "Hybrid maize seed (DK777) 10 kg bag 40 4,850.00 194,000.00",
    "NPK 17:17:17 fertiliser 50 kg 25 6,200.00 155,000.00",
    "Knapsack sprayer 16 L 6 3,900.00 23,400.00",
    "Delivery - Nairobi to Nakuru 1 8,500.00 8,500.00",
    "Subtotal KES 380,900.00",
    "VAT (16%) KES 60,944.00",
    "Total Due KES 441,844.00",
    "Payment due within 30 days of the invoice date. Thank you for your business.",
)


# ─── primitives ──────────────────────────────────────────────────────────────


@pytest.mark.parametrize(
    "raw,expected",
    [
        ("1,234.50", 1234.5),
        ("1.234,50", 1234.5),
        ("1 234,50", 1234.5),
        ("1 234 567,00", 1234567.0),
        ("KES 441,844.00", 441844.0),
        ("$1,776.00", 1776.0),
        ("1,234", 1234.0),
        ("1.234", 1234.0),
        ("(120.00)", -120.0),
        ("60,944,00", 60944.0),  # OCR comma-for-point
        ("12", 12.0),
        ("abc", None),
    ],
)
def test_parse_amount(raw, expected):
    assert parse_amount(raw) == expected


@pytest.mark.parametrize(
    "raw,expected",
    [
        ("05 Nov 2026", "2026-11-05"),
        ("5th November 2026", "2026-11-05"),
        ("05/11/2026", "2026-11-05"),
        ("05.11.2026", "2026-11-05"),
        ("05-11-26", "2026-11-05"),
        ("2026-11-05", "2026-11-05"),
        ("2026/11/05", "2026-11-05"),
        ("October 28, 2026", "2026-10-28"),
        ("Oct 28, 2026", "2026-10-28"),
        ("28-Oct-2026", "2026-10-28"),
        ("11/28/2026", "2026-11-28"),  # US fallback when day-first impossible
        ("not a date", ""),
    ],
)
def test_parse_date(raw, expected):
    assert parse_date(raw) == expected


# ─── output shape ────────────────────────────────────────────────────────────


def test_to_dict_shape_and_missing_fields():
    out = run("Hello world").to_dict()
    assert set(out) == {"engine", "fields", "lineItems", "wordCount"}
    assert out["engine"] == "test"
    assert out["wordCount"] == 2
    assert set(out["fields"]) == set(FIELD_NAMES)
    for k, f in out["fields"].items():
        if k == "sellerName":  # header fallback: first line is the best guess
            assert f["value"] == "Hello world" and f["confidence"] < 0.8
            continue
        assert f == {"value": None, "confidence": 0.0}
    assert out["lineItems"] == []


def test_full_kenyan_invoice():
    out = run(*KE_INVOICE).to_dict()
    f = {k: v["value"] for k, v in out["fields"].items()}
    assert f["invoiceNumber"] == "MAS-2026-0147"
    assert f["issueDate"] == "2026-11-05"
    assert f["dueDate"] == "2026-12-05"
    assert f["currency"] == "KES"
    assert f["subtotal"] == 380900
    assert f["vat"] == 60944
    assert f["total"] == 441844
    assert f["sellerName"] == "Mavuno Agro Supplies Ltd"
    assert f["sellerTaxId"] == "P051234567K"
    assert f["sellerEmail"] == "accounts@mavunoagro.co.ke"
    assert f["buyerName"] == "Nakuru Fresh Mart Ltd"
    assert f["buyerTaxId"] == "P051998877Q"
    assert f["buyerEmail"] == "payables@nakurufreshmart.co.ke"
    assert f["buyerPhone"] == "+254 722 019 338"
    assert f["paymentTerms"] == "Net 30"
    assert len(out["lineItems"]) == 4
    assert out["lineItems"][0] == {
        "description": "Hybrid maize seed (DK777) 10 kg bag",
        "qty": 40, "unitPrice": 4850, "amount": 194000,
        "confidence": pytest.approx(0.99, abs=0.01),
    }
    # Consistent document: everything well above the 0.6 Go-side floor.
    assert all(v["confidence"] >= 0.8 for v in out["fields"].values())


# ─── header fields ───────────────────────────────────────────────────────────


@pytest.mark.parametrize(
    "line,expected",
    [
        ("Invoice No: INV-2026-001", "INV-2026-001"),
        ("Invoice # 10482", "10482"),
        ("Tax Invoice Number 0007/26", "0007/26"),
        ("INVOICE Invoice # BL-10482", "BL-10482"),
        ("Inv No. 55512", "55512"),
    ],
)
def test_invoice_number_label_synonyms(line, expected):
    assert run("Acme Ltd", line).fields["invoiceNumber"].value == expected


def test_invoice_number_regex_fallback_lower_confidence():
    r = run("Acme Ltd", "Ref INV-778 issued to client", conf=0.9)
    assert r.fields["invoiceNumber"].value == "INV-778"
    assert r.fields["invoiceNumber"].confidence < 0.9


def test_dates_same_line_and_due_not_swallowed_by_date():
    r = run("Date: 01/02/2026   Due Date: 03/03/2026")
    assert r.fields["issueDate"].value == "2026-02-01"
    assert r.fields["dueDate"].value == "2026-03-03"


def test_due_date_derived_from_net_terms():
    r = run("Invoice Date: 2026-01-10", "Payment Terms: Net 45")
    assert r.fields["paymentTerms"].value == "Net 45"
    assert r.fields["dueDate"].value == "2026-02-24"
    assert 0 < r.fields["dueDate"].confidence < r.fields["issueDate"].confidence


def test_due_on_receipt():
    r = run("Invoice Date: 10 Jan 2026", "Terms: Due on receipt")
    assert r.fields["paymentTerms"].value == "Due on receipt"
    assert r.fields["dueDate"].value == "2026-01-10"


def test_terms_days_phrase_and_ocr_noise():
    assert run("Payment.Terms: Net-30").fields["paymentTerms"].value == "Net 30"
    assert run("Terms: 60 days").fields["paymentTerms"].value == "Net 60"


@pytest.mark.parametrize(
    "lines,expected",
    [
        (("Total Due KES 100.00",), "KES"),
        (("Total Ksh 5,000",), "KES"),
        (("Amount Due $1,200.00",), "USD"),
        (("Grand Total € 300,00",), "EUR"),
        (("Balance Due £45.00",), "GBP"),
        (("Total Br 2,500.00",), "ETB"),
        (("Currency: ETB", "Total 1,000 Ksh"), "ETB"),  # explicit label wins
        (("Total 1,000.00",), None),
    ],
)
def test_currency_inference(lines, expected):
    assert run(*lines).fields["currency"].value == expected


# ─── totals ──────────────────────────────────────────────────────────────────


def test_total_label_synonyms():
    for lbl in ("Total Due", "Amount Due", "Grand Total", "Balance Due", "Total Payable", "TOTAL"):
        assert run("Subtotal 10.00", f"{lbl} 11.60").fields["total"].value == 11.6, lbl


def test_vat_with_percentage_on_line():
    r = run("Subtotal 1,000.00", "VAT 16% 160.00", "Total 1,160.00")
    assert r.fields["vat"].value == 160
    assert r.fields["subtotal"].value == 1000
    assert r.fields["total"].value == 1160


def test_sales_tax_label_maps_to_vat():
    r = run("Subtotal $3,171.00", "Sales Tax (7%) $221.97 USD", "Balance Due $3,392.97 USD")
    assert r.fields["vat"].value == 221.97
    assert r.fields["total"].value == 3392.97


def test_amount_on_next_line_stacked_totals():
    r = run("Total Due", "KES 441,844.00")
    assert r.fields["total"].value == 441844


def test_fuzzy_ocr_label_subtotat():
    r = run("Subtotat. KES 380,900.00", "VAT (16%) KES 60,944.00", "Total Due KES 441,844.00")
    assert r.fields["subtotal"].value == 380900
    assert r.fields["subtotal"].confidence < r.fields["total"].confidence


def test_european_number_format():
    r = run("Subtotal 1.234,50", "VAT 19% 234,56", "Total 1.469,06 EUR")
    assert r.fields["subtotal"].value == 1234.5
    assert r.fields["vat"].value == 234.56
    assert r.fields["total"].value == 1469.06
    assert r.fields["currency"].value == "EUR"


# ─── consistency checks ──────────────────────────────────────────────────────


def test_consistent_totals_boost_confidence():
    r = run("Subtotal 100.00", "VAT 16.00", "Total Due 116.00", conf=0.8)
    for k in ("subtotal", "vat", "total"):
        assert r.fields[k].confidence > 0.8, k
    # Generic "Total" is a weaker anchor (0.9) but still nets a boost.
    r = run("Subtotal 100.00", "VAT 16.00", "Total 116.00", conf=0.8)
    assert r.fields["total"].confidence == pytest.approx(0.78, abs=0.01)


def test_inconsistent_totals_lower_confidence():
    r = run("Subtotal 100.00", "VAT 16.00", "Total 150.00", conf=0.8)
    for k in ("subtotal", "vat", "total"):
        assert r.fields[k].confidence < 0.8, k


def test_missing_total_derived_from_subtotal_and_vat():
    r = run("Subtotal 100.00", "VAT 16.00", conf=0.9)
    assert r.fields["total"].value == 116
    assert r.fields["total"].confidence < 0.9


def test_missing_vat_derived():
    r = run("Subtotal 100.00", "Total Due 116.00", conf=0.9)
    assert r.fields["vat"].value == 16


def test_line_items_sum_matches_subtotal_boosts():
    r = run(
        "Description Qty Unit Price Amount",
        "Widget A 2 10.00 20.00",
        "Widget B 1 30.00 30.00",
        "Subtotal 50.00",
        "VAT 8.00",
        "Total 58.00",
        conf=0.8,
    )
    assert [li.amount for li in r.line_items] == [20, 30]
    assert all(li.confidence > 0.8 for li in r.line_items)
    assert r.fields["subtotal"].confidence > 0.86


def test_line_items_sum_mismatch_lowers_item_confidence():
    r = run(
        "Description Qty Unit Price Amount",
        "Widget A 2 10.00 20.00",
        "Subtotal 999.00",
        conf=0.8,
    )
    assert r.line_items[0].confidence < 0.8


def test_due_before_issue_lowers_both():
    r = run("Invoice Date: 2026-03-01", "Due Date: 2026-02-01", conf=0.9)
    assert r.fields["issueDate"].confidence < 0.6
    assert r.fields["dueDate"].confidence < 0.6


def test_due_matches_terms_boosts():
    r = run("Invoice Date: 2026-01-01", "Due Date: 2026-01-31", "Terms: Net 30", conf=0.8)
    assert r.fields["dueDate"].confidence > 0.8
    assert r.fields["paymentTerms"].confidence > 0.8


# ─── parties ─────────────────────────────────────────────────────────────────


def test_bill_to_block_and_header_seller():
    r = run(
        "Acme Traders Ltd",
        "PIN: A001234567B",
        "info@acme.example",
        "Bill To:",
        "Zed Supplies Ltd",
        "TIN: 0012345678",
        "buyer@zed.example",
        "Tel: 0712 345 678",
        "Description Qty Amount",
        "Thing 1 5.00",
        "Total 5.00",
    )
    f = r.fields
    assert f["sellerName"].value == "Acme Traders Ltd"
    assert f["sellerTaxId"].value == "A001234567B"
    assert f["sellerEmail"].value == "info@acme.example"
    assert f["buyerName"].value == "Zed Supplies Ltd"
    assert f["buyerTaxId"].value == "0012345678"
    assert f["buyerEmail"].value == "buyer@zed.example"
    assert f["buyerPhone"].value == "0712 345 678"


@pytest.mark.parametrize("label", ["Bill To", "Sold To", "Customer", "Invoice To", "To"])
def test_buyer_label_synonyms(label):
    r = run("Seller Co Ltd", f"{label}: Buyer Enterprises Ltd", "Total 1.00")
    assert r.fields["buyerName"].value == "Buyer Enterprises Ltd", label


def test_from_block_seller():
    r = run("INVOICE", "From:", "Brightline Packaging LLC", "Tax ID: 84-2219034",
            "Bill To:", "Cedar Hollow Inc", "Total 1.00")
    assert r.fields["sellerName"].value == "Brightline Packaging LLC"
    assert r.fields["sellerTaxId"].value == "84-2219034"
    assert r.fields["buyerName"].value == "Cedar Hollow Inc"


def test_two_column_from_and_bill_to_on_one_line():
    r = run(
        "From: Bill To:",
        "Brightline Packaging LLC Cedar Hollow Coffee Roasters Inc",
        "1420 Harbor Blvd 88 Mill Street",
        "Tax ID: 84-2219034 Tax ID: 47-5583920",
        "billing@brightline.example ap@cedarhollow.example",
        "+1 (813) 555-0177 Phone: +1 (828) 555-0126",
        "Total $1.00",
    )
    f = r.fields
    assert f["sellerName"].value == "Brightline Packaging LLC"
    assert f["buyerName"].value == "Cedar Hollow Coffee Roasters Inc"
    assert f["sellerTaxId"].value == "84-2219034"
    assert f["buyerTaxId"].value == "47-5583920"
    assert f["sellerEmail"].value == "billing@brightline.example"
    assert f["buyerEmail"].value == "ap@cedarhollow.example"
    assert f["buyerPhone"].value == "+1 (828) 555-0126"


def test_tax_id_label_synonyms():
    for lbl in ("KRA PIN", "PIN", "TIN", "VAT No", "VAT Reg No", "Tax ID", "TPIN"):
        r = run("Acme Ltd", f"{lbl}: P051234567K")
        assert r.fields["sellerTaxId"].value == "P051234567K", lbl


def test_seller_name_strips_title_words():
    assert run("Mavuno Agro Supplies Ltd TAX INVOICE").fields["sellerName"].value == "Mavuno Agro Supplies Ltd"
    assert run("INVOICE", "Acme Ltd").fields["sellerName"].value == "Acme Ltd"


# ─── line items ──────────────────────────────────────────────────────────────


def test_line_items_qty_unit_amount():
    r = run(
        "Item Description Quantity Rate Amount",
        "1. Kraft bags (case of 500) 12 $148.00 $1,776.00",
        "2. Label print, roll of 1000 6 $92.50 $555.00",
        "3. Freight 1 $215.00 $215.00",
        "Subtotal $2,546.00",
    )
    items = [li.as_dict() for li in r.line_items]
    assert [i["description"] for i in items] == [
        "Kraft bags (case of 500)", "Label print, roll of 1000", "Freight",
    ]
    assert [(i["qty"], i["unitPrice"], i["amount"]) for i in items] == [
        (12, 148, 1776), (6, 92.5, 555), (1, 215, 215),
    ]


def test_line_item_two_numbers_derives_unit_price():
    r = run("Description Qty Amount", "Consulting hours 4 400.00", "Total 400.00")
    li = r.line_items[0]
    assert (li.qty, li.unitPrice, li.amount) == (4, 100, 400)


def test_line_item_single_amount():
    r = run("Description Amount", "Delivery fee 8,500.00", "Total 8,500.00")
    li = r.line_items[0]
    assert (li.qty, li.unitPrice, li.amount) == (1, 8500, 8500)


def test_line_item_inconsistent_math_lowers_confidence():
    r = run("Description Qty Unit Price Amount", "Widget 2 10.00 99.00", conf=0.9)
    assert r.line_items[0].confidence < 0.9


def test_ocr_noise_in_items_and_table_end():
    r = run(
        "Description. .Qty. Unit Price (KES) Amount (KES)",
        "Delivery -Nairobi toNakuru 1. 8,500.00 8;500.00",
        ".Subtotat. KES 8,500.00",
        "Total Due KES 9,860.00",
    )
    assert len(r.line_items) == 1
    assert r.line_items[0].amount == 8500


# ─── endpoint ────────────────────────────────────────────────────────────────


@pytest.fixture(scope="module")
def client():
    pytest.importorskip("fastapi")
    pytest.importorskip("httpx")
    pytest.importorskip("fitz")
    from fastapi.testclient import TestClient

    from main import app

    return TestClient(app)


def _post(client, path):
    with open(path, "rb") as f:
        return client.post(
            "/v1/invoice/extract",
            files={"file": (os.path.basename(path), f, "application/pdf")},
        )


def _sample(name):
    p = os.path.join(SAMPLES, name)
    if not os.path.exists(p):
        pytest.skip(f"sample {name} not generated (scripts/make_sample_invoices.py)")
    return p


def test_endpoint_sample_a_pdf_text(client):
    r = _post(client, _sample("invoice-a-ke-clean.pdf"))
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["engine"] == "pdf-text"
    f = {k: v["value"] for k, v in body["fields"].items()}
    assert f["invoiceNumber"] == "MAS-2026-0147"
    assert f["issueDate"] == "2026-11-05" and f["dueDate"] == "2026-12-05"
    assert f["currency"] == "KES"
    assert (f["subtotal"], f["vat"], f["total"]) == (380900, 60944, 441844)
    assert f["sellerName"] == "Mavuno Agro Supplies Ltd"
    assert f["sellerTaxId"] == "P051234567K"
    assert f["buyerName"] == "Nakuru Fresh Mart Ltd"
    assert f["buyerTaxId"] == "P051998877Q"
    assert f["buyerEmail"] == "payables@nakurufreshmart.co.ke"
    assert f["paymentTerms"] == "Net 30"
    assert len(body["lineItems"]) == 4
    assert sum(li["amount"] for li in body["lineItems"]) == 380900
    assert all(v["confidence"] >= 0.6 for v in body["fields"].values())


def test_endpoint_sample_c_bill_to(client):
    r = _post(client, _sample("invoice-c-usd-billto.pdf"))
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["engine"] == "pdf-text"
    f = {k: v["value"] for k, v in body["fields"].items()}
    assert f["invoiceNumber"] == "BL-10482"
    assert f["currency"] == "USD"
    assert (f["subtotal"], f["vat"], f["total"]) == (3171, 221.97, 3392.97)
    assert f["sellerName"] == "Brightline Packaging LLC"
    assert f["buyerName"] == "Cedar Hollow Coffee Roasters Inc"
    assert f["buyerTaxId"] == "47-5583920"
    assert f["buyerEmail"] == "ap@cedarhollowroasters.example"
    assert f["buyerPhone"] == "+1 (828) 555-0126"
    assert f["paymentTerms"] == "Net 45"
    assert f["dueDate"] == "2026-12-12"
    assert [li["qty"] for li in body["lineItems"]] == [12, 6, 20, 1]


def test_endpoint_rejects_empty_and_garbage(client):
    r = client.post("/v1/invoice/extract", files={"file": ("x.pdf", b"", "application/pdf")})
    assert r.status_code == 400
    r = client.post("/v1/invoice/extract", files={"file": ("x.pdf", b"%PDF-1.4 garbage", "application/pdf")})
    assert r.status_code == 422


def test_endpoint_scanned_pdf_needs_ocr(client):
    """Scanned sample: 200 via OCR when an engine is provisioned, else 503."""
    from engine import ocr, ppocr

    r = _post(client, _sample("invoice-d-scanned.pdf"))
    if ppocr.models_available() or ocr.tesseract_available():
        assert r.status_code == 200, r.text
        body = r.json()
        assert body["engine"] in ("ppocr", "tesseract")
        f = {k: v["value"] for k, v in body["fields"].items()}
        assert f["invoiceNumber"] == "MAS-2026-0147"
        assert f["total"] == 441844
    else:
        assert r.status_code == 503
