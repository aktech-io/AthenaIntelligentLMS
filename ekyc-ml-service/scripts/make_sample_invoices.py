#!/usr/bin/env python3
"""Generate the fictional sample invoices under docs/nemo/invoice-finance/samples/.

    pip install -r requirements-dev.txt   # reportlab + pymupdf + Pillow
    python scripts/make_sample_invoices.py [out_dir]

Produces:
  invoice-a-ke-clean.pdf   Kenyan SME, KES, KRA PIN, VAT 16 %, Net 30 (text layer)
  invoice-b-ke-second.pdf  same seller, second invoice (text layer)
  invoice-c-usd-billto.pdf USD, "Bill To" two-column layout (text layer)
  invoice-d-scanned.pdf    invoice (a)'s layout rasterised to a noisy, slightly
                           rotated 200 dpi JPEG embedded in a PDF (no text
                           layer → exercises the OCR path)

All companies, PINs, phone numbers and e-mails are fictional.
"""
from __future__ import annotations

import io
import os
import random
import sys

from reportlab.lib import colors
from reportlab.lib.pagesizes import A4
from reportlab.lib.units import mm
from reportlab.pdfgen import canvas

SELLER_KE = {
    "name": "Mavuno Agro Supplies Ltd",
    "addr": ["Enterprise Road, Industrial Area", "P.O. Box 41822-00100, Nairobi, Kenya"],
    "pin": "P051234567K",
    "email": "accounts@mavunoagro.co.ke",
    "tel": "+254 20 555 0142",
}
BUYER_KE = {
    "name": "Nakuru Fresh Mart Ltd",
    "addr": ["Kenyatta Avenue, Nakuru", "P.O. Box 1180-20100, Nakuru, Kenya"],
    "pin": "P051998877Q",
    "email": "payables@nakurufreshmart.co.ke",
    "tel": "+254 722 019 338",
}
SELLER_US = {
    "name": "Brightline Packaging LLC",
    "addr": ["1420 Harbor Blvd, Suite 300", "Tampa, FL 33602, USA"],
    "pin": "84-2219034",
    "email": "billing@brightlinepkg.example",
    "tel": "+1 (813) 555-0177",
}
BUYER_US = {
    "name": "Cedar Hollow Coffee Roasters Inc",
    "addr": ["88 Mill Street", "Asheville, NC 28801, USA"],
    "pin": "47-5583920",
    "email": "ap@cedarhollowroasters.example",
    "tel": "+1 (828) 555-0126",
}


def money(v: float) -> str:
    return f"{v:,.2f}"


def draw_invoice_ke(c, *, number, issue, due, items, seller=SELLER_KE, buyer=BUYER_KE, vat_rate=0.16):
    W, H = A4
    y = H - 20 * mm
    c.setFont("Helvetica-Bold", 16)
    c.drawString(20 * mm, y, seller["name"])
    c.setFont("Helvetica-Bold", 20)
    c.drawRightString(W - 20 * mm, y, "TAX INVOICE")
    y -= 7 * mm
    c.setFont("Helvetica", 9.5)
    for ln in seller["addr"]:
        c.drawString(20 * mm, y, ln); y -= 4.5 * mm
    c.drawString(20 * mm, y, f"KRA PIN: {seller['pin']}"); y -= 4.5 * mm
    c.drawString(20 * mm, y, f"Email: {seller['email']}   Tel: {seller['tel']}"); y -= 10 * mm

    c.setFont("Helvetica-Bold", 10)
    c.drawString(20 * mm, y, "Bill To:")
    c.setFont("Helvetica", 9.5)
    c.drawString(120 * mm, y, "Invoice No:"); c.drawString(150 * mm, y, number); y -= 4.5 * mm
    c.setFont("Helvetica-Bold", 10); c.drawString(20 * mm, y, buyer["name"]); c.setFont("Helvetica", 9.5)
    c.drawString(120 * mm, y, "Invoice Date:"); c.drawString(150 * mm, y, issue); y -= 4.5 * mm
    c.drawString(20 * mm, y, buyer["addr"][0]); c.drawString(120 * mm, y, "Due Date:"); c.drawString(150 * mm, y, due); y -= 4.5 * mm
    c.drawString(20 * mm, y, buyer["addr"][1]); c.drawString(120 * mm, y, "Payment Terms:"); c.drawString(150 * mm, y, "Net 30"); y -= 4.5 * mm
    c.drawString(20 * mm, y, f"PIN: {buyer['pin']}"); c.drawString(120 * mm, y, "Currency:"); c.drawString(150 * mm, y, "KES"); y -= 4.5 * mm
    c.drawString(20 * mm, y, f"Email: {buyer['email']}"); y -= 4.5 * mm
    c.drawString(20 * mm, y, f"Tel: {buyer['tel']}"); y -= 10 * mm

    # table
    c.setFillColor(colors.HexColor("#e8eef5")); c.rect(20 * mm, y - 2 * mm, W - 40 * mm, 7 * mm, fill=1, stroke=0)
    c.setFillColor(colors.black); c.setFont("Helvetica-Bold", 9.5)
    c.drawString(22 * mm, y, "Description"); c.drawRightString(120 * mm, y, "Qty")
    c.drawRightString(150 * mm, y, "Unit Price (KES)"); c.drawRightString(W - 22 * mm, y, "Amount (KES)")
    y -= 7 * mm; c.setFont("Helvetica", 9.5)
    subtotal = 0.0
    for desc, qty, unit in items:
        amt = qty * unit; subtotal += amt
        c.drawString(22 * mm, y, desc); c.drawRightString(120 * mm, y, str(qty))
        c.drawRightString(150 * mm, y, money(unit)); c.drawRightString(W - 22 * mm, y, money(amt))
        y -= 5.5 * mm
    y -= 3 * mm
    c.line(110 * mm, y + 2 * mm, W - 20 * mm, y + 2 * mm)
    vat = round(subtotal * vat_rate, 2); total = round(subtotal + vat, 2)
    for label, val, bold in (("Subtotal", subtotal, False), (f"VAT ({int(vat_rate*100)}%)", vat, False), ("Total Due", total, True)):
        c.setFont("Helvetica-Bold" if bold else "Helvetica", 10.5 if bold else 9.5)
        c.drawString(112 * mm, y, label); c.drawRightString(W - 22 * mm, y, f"KES {money(val)}"); y -= 5.5 * mm
    y -= 8 * mm
    c.setFont("Helvetica", 9)
    c.drawString(20 * mm, y, "Payment: Bank transfer to Mavuno Agro Supplies Ltd, Equity Bank, Industrial Area Branch, A/C 0450123456789."); y -= 4.5 * mm
    c.drawString(20 * mm, y, "Payment due within 30 days of the invoice date. Thank you for your business."); y -= 4.5 * mm
    c.showPage()
    return subtotal, vat, total


def draw_invoice_us(c, *, number, issue, due, items, seller=SELLER_US, buyer=BUYER_US, tax_rate=0.07):
    W, H = A4
    y = H - 20 * mm
    c.setFont("Helvetica-Bold", 24); c.drawString(20 * mm, y, "INVOICE")
    c.setFont("Helvetica", 9.5)
    c.drawRightString(W - 20 * mm, y, f"Invoice # {number}"); y -= 5 * mm
    c.drawRightString(W - 20 * mm, y, f"Date: {issue}"); y -= 5 * mm
    c.drawRightString(W - 20 * mm, y, f"Due Date: {due}"); y -= 5 * mm
    c.drawRightString(W - 20 * mm, y, "Terms: Net 45"); y -= 8 * mm

    c.setFont("Helvetica-Bold", 10); c.drawString(20 * mm, y, "From:"); c.drawString(110 * mm, y, "Bill To:"); y -= 5 * mm
    c.setFont("Helvetica", 9.5)
    left = [seller["name"], *seller["addr"], f"Tax ID: {seller['pin']}", seller["email"], seller["tel"]]
    right = [buyer["name"], *buyer["addr"], f"Tax ID: {buyer['pin']}", buyer["email"], f"Phone: {buyer['tel']}"]
    for l, r in zip(left, right):
        c.drawString(20 * mm, y, l); c.drawString(110 * mm, y, r); y -= 4.5 * mm
    y -= 8 * mm
    c.setFont("Helvetica-Bold", 9.5)
    c.drawString(20 * mm, y, "Item"); c.drawString(32 * mm, y, "Description"); c.drawRightString(135 * mm, y, "Quantity")
    c.drawRightString(160 * mm, y, "Rate"); c.drawRightString(W - 20 * mm, y, "Amount")
    y -= 2 * mm; c.line(20 * mm, y, W - 20 * mm, y); y -= 5 * mm
    c.setFont("Helvetica", 9.5)
    subtotal = 0.0
    for i, (desc, qty, unit) in enumerate(items, 1):
        amt = qty * unit; subtotal += amt
        c.drawString(20 * mm, y, f"{i}."); c.drawString(32 * mm, y, desc); c.drawRightString(135 * mm, y, str(qty))
        c.drawRightString(160 * mm, y, f"${money(unit)}"); c.drawRightString(W - 20 * mm, y, f"${money(amt)}"); y -= 5.5 * mm
    y -= 4 * mm
    tax = round(subtotal * tax_rate, 2); total = round(subtotal + tax, 2)
    for label, val, bold in (("Subtotal", subtotal, False), (f"Sales Tax ({tax_rate*100:.0f}%)", tax, False), ("Balance Due", total, True)):
        c.setFont("Helvetica-Bold" if bold else "Helvetica", 10.5 if bold else 9.5)
        c.drawString(130 * mm, y, label); c.drawRightString(W - 20 * mm, y, f"${money(val)} USD"); y -= 5.5 * mm
    y -= 8 * mm
    c.setFont("Helvetica", 9)
    c.drawString(20 * mm, y, "Please remit payment by ACH or check within 45 days. Questions: billing@brightlinepkg.example")
    c.showPage()
    return subtotal, tax, total


def make_pdf(path, draw, **kw):
    c = canvas.Canvas(path, pagesize=A4)
    c.setTitle("Invoice"); c.setAuthor(kw.get("seller", SELLER_KE)["name"])
    totals = draw(c, **kw)
    c.save()
    return totals


def rasterise_to_scan(src_pdf: str, dst_pdf: str, dpi: int = 200, seed: int = 7):
    """Render page 1 → slightly rotated, noisy JPEG → wrap in a PDF (no text)."""
    import fitz
    from PIL import Image, ImageFilter

    rnd = random.Random(seed)
    doc = fitz.open(src_pdf)
    pix = doc[0].get_pixmap(matrix=fitz.Matrix(dpi / 72, dpi / 72), alpha=False)
    img = Image.frombytes("RGB", (pix.width, pix.height), pix.samples).convert("L")
    img = img.rotate(rnd.uniform(-1.2, 1.2), resample=Image.BICUBIC, expand=False, fillcolor=245)
    img = img.filter(ImageFilter.GaussianBlur(0.6))
    # Speckle noise + uneven background
    px = img.load()
    w, h = img.size
    for _ in range(int(w * h * 0.004)):
        x, y = rnd.randrange(w), rnd.randrange(h)
        px[x, y] = rnd.choice((0, 40, 200, 230))
    img = img.point(lambda v: min(255, int(v * 0.96 + 6)))
    buf = io.BytesIO()
    img.convert("RGB").save(buf, "JPEG", quality=65, dpi=(dpi, dpi))
    with open(dst_pdf, "wb") as f:
        img.convert("RGB").save(f, "PDF", resolution=dpi)
    return buf.getvalue()


def main(out_dir: str):
    os.makedirs(out_dir, exist_ok=True)
    items_a = [
        ("Hybrid maize seed (DK777) 10 kg bag", 40, 4850.00),
        ("NPK 17:17:17 fertiliser 50 kg", 25, 6200.00),
        ("Knapsack sprayer 16 L", 6, 3900.00),
        ("Delivery - Nairobi to Nakuru", 1, 8500.00),
    ]
    items_b = [
        ("Drip irrigation kit 1/4 acre", 3, 42500.00),
        ("Shade net 70% 4m x 50m", 8, 11800.00),
        ("Agronomy site visit", 2, 15000.00),
    ]
    items_c = [
        ("12 oz kraft coffee bags w/ valve (case of 500)", 12, 148.00),
        ("Custom 2-color label print, roll of 1000", 6, 92.50),
        ("Corrugated shipper 12x12x8 (bundle of 25)", 20, 31.25),
        ("Freight (LTL, Tampa to Asheville)", 1, 215.00),
    ]
    a = os.path.join(out_dir, "invoice-a-ke-clean.pdf")
    print("a", make_pdf(a, draw_invoice_ke, number="MAS-2026-0147", issue="05 Nov 2026", due="05 Dec 2026", items=items_a))
    b = os.path.join(out_dir, "invoice-b-ke-second.pdf")
    print("b", make_pdf(b, draw_invoice_ke, number="MAS-2026-0152", issue="07/11/2026", due="07/12/2026", items=items_b))
    cpath = os.path.join(out_dir, "invoice-c-usd-billto.pdf")
    print("c", make_pdf(cpath, draw_invoice_us, number="BL-10482", issue="October 28, 2026", due="December 12, 2026", items=items_c))
    d = os.path.join(out_dir, "invoice-d-scanned.pdf")
    rasterise_to_scan(a, d)
    print("d scanned from a ->", d)


if __name__ == "__main__":
    here = os.path.dirname(os.path.abspath(__file__))
    default = os.path.normpath(os.path.join(here, "..", "..", "docs", "nemo", "invoice-finance", "samples"))
    main(sys.argv[1] if len(sys.argv) > 1 else default)
