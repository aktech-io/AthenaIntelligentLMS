"""Invoice field extraction over raw OCR / PDF-text output.

Pure logic (no OCR, PDF or OpenCV imports) with the same testability contract
as engine.fields / engine.profiles: the caller hands over `OcrWord`s (text,
confidence, line index) plus line-preserving full text and gets back typed,
per-field-confidence invoice fields and line items.

Country-neutral by design — label synonyms cover Kenyan ("KRA PIN", "Ksh"),
Ethiopian ("TIN", "Br"), UK/EU ("VAT No", "£", "€") and US ("Bill To",
"Sales Tax", "$") conventions; number parsing accepts 1,234.50 / 1.234,50 /
1 234,50; dates accept numeric, month-name and ISO forms.

Confidence model: a field's base confidence is the mean OCR confidence of the
line it was read from, scaled by how it was found (label-anchored 1.0,
regex fallback 0.7, derived 0.8 × source). Cross-field consistency checks
(subtotal + VAT ≈ total, Σ line items ≈ subtotal, dueDate ≥ issueDate) then
raise or lower the confidence of the fields involved. Missing fields are
always present in the output with value null / confidence 0 so the Go side
can apply its 0.6 "needsSellerConfirmation" floor uniformly.
"""
from __future__ import annotations

import re
from dataclasses import dataclass, field
from datetime import date, datetime, timedelta

from engine.fields import OcrWord

FIELD_NAMES = (
    "invoiceNumber", "issueDate", "dueDate", "currency",
    "subtotal", "vat", "total",
    "sellerName", "sellerTaxId", "sellerEmail",
    "buyerName", "buyerTaxId", "buyerEmail", "buyerPhone",
    "paymentTerms",
)

_FALLBACK_FACTOR = 0.7  # regex / positional guess rather than a label
_DERIVED_FACTOR = 0.8  # computed from other fields
_CONSISTENT_BOOST = 0.06
_INCONSISTENT_FACTOR = 0.7
_MAX_CONF = 0.99


# ─── data types ──────────────────────────────────────────────────────────────


@dataclass
class InvoiceField:
    value: object = None  # str | float | None
    confidence: float = 0.0

    def as_dict(self) -> dict:
        return {"value": self.value, "confidence": round(self.confidence, 4)}

    @property
    def present(self) -> bool:
        return self.value is not None


@dataclass
class LineItem:
    description: str
    qty: float | None
    unitPrice: float | None
    amount: float | None
    confidence: float

    def as_dict(self) -> dict:
        return {
            "description": self.description,
            "qty": self.qty,
            "unitPrice": self.unitPrice,
            "amount": self.amount,
            "confidence": round(self.confidence, 4),
        }


@dataclass
class InvoiceExtraction:
    engine: str = ""
    fields: dict[str, InvoiceField] = field(default_factory=dict)
    line_items: list[LineItem] = field(default_factory=list)
    word_count: int = 0

    def to_dict(self) -> dict:
        return {
            "engine": self.engine,
            "fields": {
                k: self.fields.get(k, InvoiceField()).as_dict() for k in FIELD_NAMES
            },
            "lineItems": [li.as_dict() for li in self.line_items],
            "wordCount": self.word_count,
        }


@dataclass
class _Line:
    idx: int
    text: str
    conf: float


# ─── labels ──────────────────────────────────────────────────────────────────

# Each label is a regex fragment; matched case-insensitively at a word
# boundary, optionally followed by "No/Number/#" and ":".
_LBL_INVOICE_NO = (
    r"tax invoice (?:no|number|#)", r"invoice (?:no|number|#|ref|id)",
    r"inv(?:oice)?\.?\s*(?:no|#|number)", r"invoice",
)
_LBL_ISSUE_DATE = (
    r"invoice date", r"date of issue", r"issue date", r"issued on", r"issued",
    r"date",
)
_LBL_DUE_DATE = (
    r"due date", r"payment due", r"due on", r"pay by", r"due by", r"due",
)
_LBL_TERMS = (r"payment terms", r"terms of payment", r"terms")
_LBL_SUBTOTAL = (
    r"sub[\s\-]?total", r"net amount", r"net total", r"amount excl(?:uding|\.)?(?: of)? (?:vat|tax)",
    r"total (?:before|excl(?:uding|\.)?) (?:vat|tax)", r"taxable (?:amount|value)",
)
_LBL_VAT = (
    r"vat(?:\s*\(?\d{1,2}(?:\.\d+)?\s*%\)?)?(?: amount)?", r"sales tax", r"gst", r"tax(?: amount)?",
    r"output tax",
)
_LBL_TOTAL = (
    r"total (?:amount )?due", r"amount due", r"grand total", r"balance due",
    r"total payable", r"amount payable", r"total (?:amount )?incl(?:uding|\.)?(?: of)? (?:vat|tax)",
    r"invoice total", r"total amount", r"total",
)
_LBL_BUYER = (
    r"bill(?:ed)? to", r"invoice to", r"invoiced to", r"sold to", r"ship to",
    r"customer(?: details| name)?", r"buyer(?: details)?", r"client(?: details)?", r"\bto\b",
)
_LBL_SELLER = (
    r"from", r"supplier(?: details)?", r"seller(?: details)?", r"vendor(?: details)?",
    r"issued by", r"remit to", r"payable to",
)
_LBL_TAX_ID = (
    r"kra pin", r"pin (?:no|number)", r"vat (?:reg(?:istration)?\.? )?(?:no|number)", r"vat reg(?:istration)?",
    r"tax (?:id|identification)(?: (?:no|number))?", r"tin(?: no| number)?", r"tpin", r"gstin",
    r"abn", r"company (?:reg(?:istration)?|no)", r"pin",
)
_LBL_EMAIL = (r"e-?mail",)
_LBL_PHONE = (r"tel(?:ephone)?", r"phone", r"mobile", r"cell", r"contact")
_LBL_CURRENCY = (r"currency",)

_TABLE_HEADER_WORDS = (
    r"description", r"item", r"particulars", r"details", r"qty", r"quantity",
    r"unit price", r"rate", r"price", r"amount", r"line total", r"total",
)

_COMPANY_HINTS = re.compile(
    r"\b(ltd|limited|llc|inc|plc|co|company|corp(?:oration)?|enterprises?|supplies|"
    r"services|solutions|traders|trading|holdings|group|partners|agencies|"
    r"industries|distributors|logistics|technologies|consult(?:ing|ants)|"
    r"gmbh|sarl|s\.?a\.?|pty|bv|cc|sacco|stores?|hardware|pharmacy|clinic|"
    r"hotel|farm|dairy|bakery|builders|contractors|works|associates)\b\.?",
    re.I,
)
_TITLE_ANYWHERE = re.compile(
    r"\b(?:tax\s+)?(?:invoice|proforma|pro-forma|receipt|statement|credit note|quotation)\b.*$",
    re.I,
)
_TITLE_WORDS = re.compile(
    r"^\s*(tax\s+)?(invoice|proforma|pro-forma|receipt|statement|credit note|quotation|bill)\b",
    re.I,
)


_SEP = r"[\s._\-]*"  # OCR often reads "Payment Terms" as "Payment.Terms"


def _label_re(labels: tuple[str, ...]) -> re.Pattern:
    alts = "|".join(f"(?:{l.replace(' ', _SEP)})" for l in labels)
    return re.compile(rf"(?<![A-Za-z])(?:{alts})(?![A-Za-z])\s*(?:(?:no|number|#|nr)\.?\s*)?[:#.\-]?\s*", re.I)


_RE_INVOICE_NO = _label_re(_LBL_INVOICE_NO)
_RE_ISSUE_DATE = _label_re(_LBL_ISSUE_DATE)
_RE_DUE_DATE = _label_re(_LBL_DUE_DATE)
_RE_TERMS = _label_re(_LBL_TERMS)
_RE_SUBTOTAL = _label_re(_LBL_SUBTOTAL)
_RE_VAT = _label_re(_LBL_VAT)
_RE_TOTAL = _label_re(_LBL_TOTAL)
_RE_BUYER = _label_re(_LBL_BUYER)
_RE_SELLER = _label_re(_LBL_SELLER)
_RE_TAX_ID = _label_re(_LBL_TAX_ID)
_RE_EMAIL_LBL = _label_re(_LBL_EMAIL)
_RE_PHONE_LBL = _label_re(_LBL_PHONE)
_RE_CURRENCY_LBL = _label_re(_LBL_CURRENCY)

# Any label at all — used to cut a value off where the next label starts on
# the same line ("Invoice Date: 05 Nov 2026   Due Date: 05 Dec 2026").
_RE_ANY_LABEL = _label_re(
    _LBL_INVOICE_NO + _LBL_ISSUE_DATE + _LBL_DUE_DATE + _LBL_TERMS + _LBL_SUBTOTAL
    + _LBL_TOTAL + _LBL_TAX_ID + _LBL_EMAIL + _LBL_PHONE + _LBL_CURRENCY
    # Party labels only in their compound forms: bare "to"/"from"/"buyer"/
    # "customer" occur inside names and addresses ("Buyer Enterprises Ltd").
    + (r"bill(?:ed)? to", r"invoice(?:d)? to", r"sold to", r"ship to", r"remit to",
       r"vat (?:no|number)", r"p\.?o\.? (?:box|no|number)", r"po\b")
)

_RE_EMAIL = re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}")
_RE_PHONE = re.compile(r"(?<![\w.])(\+?\(?\d{1,4}\)?[\s\-.]?\(?\d{2,4}\)?(?:[\s\-.]?\d{2,4}){1,4})(?![\w.])")
_RE_TAX_TOKEN = re.compile(r"(?<![A-Za-z0-9])([A-Z]{0,3}\d[A-Z0-9\-/]{4,}[A-Z0-9]|[A-Z]\d{9}[A-Z])(?![A-Za-z0-9])")
_RE_INVNO_TOKEN = re.compile(r"(?<![A-Za-z0-9])(?=[A-Za-z0-9\-/_.#]*\d)([A-Za-z0-9][A-Za-z0-9\-/_.]*[A-Za-z0-9])(?![A-Za-z0-9])")
_RE_NET_TERMS = re.compile(r"\bnet[\s\-.:]*(\d{1,3})\b", re.I)
_RE_DAYS_TERMS = re.compile(r"\b(\d{1,3})\s*days?\b", re.I)
_RE_PERCENT = re.compile(r"\d+(?:[.,]\d+)?\s*%")

# Amount: grouped thousands with , . or space, optional decimals; or plain.
_RE_AMOUNT = re.compile(
    # Space/NBSP thousands grouping only with a comma decimal or none at all
    # ("1 234,50", "12 000") \u2014 "4 400.00" on a table row is qty 4, amount 400.00.
    r"(?<![\w.,/-])(\(?-?\s?(?:"
    r"\d{1,3}(?:[,.]\d{3})+(?:[.,]\d{1,2})?"
    r"|\d{1,3}(?:[\u00a0 ]\d{3})+(?:,\d{1,2})?"
    r"|\d+(?:[.,]\d{1,2})?"
    r")\)?)(?![\w%/-]|[.,]\d)"
)

_RE_DATE = re.compile(
    r"(?<![\d/.-])("
    r"\d{4}[-/.]\d{1,2}[-/.]\d{1,2}"  # ISO-ish
    r"|\d{1,2}[-/.]\d{1,2}[-/.]\d{2,4}"  # numeric d/m/y or m/d/y
    r"|\d{1,2}(?:st|nd|rd|th)?[\s\-]+[A-Za-z]{3,9}\.?,?[\s\-]+\d{4}"  # 05 Nov 2026, 5-Nov-2026
    r"|[A-Za-z]{3,9}\.?\s+\d{1,2}(?:st|nd|rd|th)?,?\s+\d{4}"  # Nov 5, 2026
    r")(?![\d/.-])"
)
_DATE_FORMATS = (
    "%Y-%m-%d", "%Y/%m/%d", "%Y.%m.%d",
    "%d/%m/%Y", "%d-%m-%Y", "%d.%m.%Y", "%d/%m/%y", "%d-%m-%y", "%d.%m.%y",
    "%d %b %Y", "%d %B %Y", "%d-%b-%Y", "%d-%B-%Y", "%d %b, %Y", "%d %B, %Y",
    "%b %d, %Y", "%B %d, %Y", "%b %d %Y", "%B %d %Y",
)

_CURRENCY_WORDS: tuple[tuple[re.Pattern, str, float], ...] = (
    (re.compile(r"\b(KES|USD|EUR|ETB|GBP|UGX|TZS|RWF|ZAR|NGN|GHS|INR|AED|CHF|CAD|AUD)\b"), "", 0.95),
    (re.compile(r"\b(?:ksh?s?|kshs|k\.?sh\.?|kenya(?:n)? shillings?)\b\.?", re.I), "KES", 0.9),
    (re.compile(r"\b(?:birr|br)\b\.?", re.I), "ETB", 0.85),
    (re.compile(r"\b(?:us\s?dollars?|dollars?)\b", re.I), "USD", 0.85),
    (re.compile(r"\b(?:euros?)\b", re.I), "EUR", 0.85),
    (re.compile(r"\b(?:pounds?( sterling)?)\b", re.I), "GBP", 0.85),
    (re.compile(r"\bUS?\$"), "USD", 0.85),
    (re.compile(r"\$"), "USD", 0.8),
    (re.compile(r"€"), "EUR", 0.85),
    (re.compile(r"£"), "GBP", 0.85),
)


# ─── parsing primitives ──────────────────────────────────────────────────────


def parse_amount(s: str) -> float | None:
    """Tolerant money parser: '1,234.50', '1.234,50', '1 234,50', '(120.00)',
    'KES 1,200' -> float. Returns None when no number is present."""
    if s is None:
        return None
    t = s.strip()
    neg = t.startswith("(") and t.endswith(")") or t.startswith("-")
    t = re.sub(r"[^\d.,\u00a0 ]", "", t).replace("\u00a0", " ").strip()
    if not t or not re.search(r"\d", t):
        return None
    # Which separator is decimal? The last one, if it is followed by 1-2
    # digits; a final group of exactly 3 digits is a thousands group.
    m = re.search(r"[.,](\d{1,2})$", t)
    if m:
        dec = m.group(1)
        intpart = re.sub(r"\D", "", t[: m.start()])
        val = float(f"{intpart or '0'}.{dec}")
    else:
        val = float(re.sub(r"\D", "", t))
    return -val if neg else val


def parse_date(s: str) -> str:
    """Printed date in common invoice formats -> ISO yyyy-mm-dd ('' if not a
    date). Ambiguous numeric d/m vs m/d: day-first unless impossible."""
    t = s.strip().rstrip(".,").replace("‐", "-").replace("\u00a0", " ")
    t = re.sub(r"\s+", " ", t)
    t = re.sub(r"(\d)(st|nd|rd|th)\b", r"\1", t, flags=re.I)
    t = re.sub(r"(\d)\s*-\s*([A-Za-z])", r"\1-\2", t)
    t = re.sub(r"([A-Za-z])\s*-\s*(\d)", r"\1-\2", t)
    t = re.sub(r"\b([A-Za-z]{3})\.", r"\1", t)
    t = re.sub(r"\b(Sept)\b", "Sep", t, flags=re.I)
    for fmt in _DATE_FORMATS:
        try:
            d = datetime.strptime(t, fmt).date()
        except ValueError:
            continue
        if "%y" in fmt and d.year < 2000:
            d = d.replace(year=d.year + 100)
        return d.isoformat()
    # m/d/y fallback (US) when day-first failed
    m = re.fullmatch(r"(\d{1,2})[/.-](\d{1,2})[/.-](\d{2,4})", t)
    if m:
        mo, da, yr = int(m.group(1)), int(m.group(2)), int(m.group(3))
        if yr < 100:
            yr += 2000
        try:
            return date(yr, mo, da).isoformat()
        except ValueError:
            return ""
    return ""


def _first_date(s: str) -> str:
    for m in _RE_DATE.finditer(s):
        iso = parse_date(m.group(1))
        if iso:
            return iso
    return ""


def _norm_numeric_noise(s: str) -> str:
    """OCR noise: ';' for ',' between digit groups, stray '.' before a number."""
    s = re.sub(r"(?<=\d)[;](?=\d{3})", ",", s)
    s = re.sub(r"(?<![\w])\.(?=\d)", "", s)
    return s


def _amounts(s: str) -> list[float]:
    """All money-looking numbers in s, percentages and dates excluded."""
    s = _norm_numeric_noise(s)
    s = _RE_PERCENT.sub(" ", s)
    s = _RE_DATE.sub(" ", s)
    out = []
    for m in _RE_AMOUNT.finditer(s):
        v = parse_amount(m.group(1))
        if v is not None:
            out.append(v)
    return out


def _cut_at_next_label(s: str) -> str:
    """Value text ends where another field label begins on the same line."""
    masked = _RE_EMAIL.sub(lambda m: "x" * len(m.group(0)), s)  # "buyer@…" is not a label
    m = _RE_ANY_LABEL.search(masked)
    return s[: m.start()] if m else s


def _after_label(line: str, rx: re.Pattern) -> str | None:
    m = rx.search(line)
    if not m:
        return None
    return _cut_at_next_label(line[m.end():]).strip()


def _clean_name(s: str) -> str:
    s = _RE_EMAIL.sub("", s)
    s = _TITLE_ANYWHERE.sub("", s)
    s = re.sub(r"\s+\b(?:tax|proforma)\s*$", "", s, flags=re.I)  # "… Ltd TAX" (INVOICE cut off)
    s = re.sub(r"[|_]+", " ", s)
    s = re.sub(r"^[\s:,\-#.]+|[\s:,\-#.]+$", "", s)
    return re.sub(r"\s{2,}", " ", s).strip()


def _clip(c: float) -> float:
    return max(0.0, min(_MAX_CONF, c))


# ─── lines ───────────────────────────────────────────────────────────────────


def _build_lines(words: list[OcrWord], text: str) -> list[_Line]:
    groups: dict[int, list[OcrWord]] = {}
    for w in words:
        if w.text.strip():
            groups.setdefault(w.line, []).append(w)
    if groups:
        return [
            _Line(i, " ".join(w.text for w in groups[k]),
                  sum(w.confidence for w in groups[k]) / len(groups[k]))
            for i, k in enumerate(sorted(groups))
        ]
    # No word boxes (plain text only): neutral mid confidence.
    return [
        _Line(i, ln.strip(), 0.5)
        for i, ln in enumerate(l for l in text.splitlines() if l.strip())
    ]


def _is_table_header(line: str) -> bool:
    hits = sum(1 for w in _TABLE_HEADER_WORDS if re.search(rf"\b{w}\b", line, re.I))
    return hits >= 2 and not _amounts(line)


_FUZZY_MONEY = {
    "subtotal": "subtotal", "sub total": "subtotal", "net amount": "subtotal",
    "total due": "total", "amount due": "total", "grand total": "total",
    "balance due": "total", "total amount": "total", "total": "total",
    "vat": "vat", "sales tax": "vat",
}


def _fuzzy_money_label(line: str) -> str | None:
    """OCR-tolerant label kind for a totals line ('Subtotat.' -> subtotal):
    the leading 1-2 alphabetic words must be >=0.8 similar to a known label."""
    import difflib

    words = re.findall(r"[A-Za-z]+", line[:40])
    if not words:
        return None
    cands = [words[0].lower()] + ([" ".join(words[:2]).lower()] if len(words) > 1 else [])
    best, kind = 0.0, None
    for c in cands:
        if len(c) < 4:
            continue
        for lbl, k in _FUZZY_MONEY.items():
            r = difflib.SequenceMatcher(None, c, lbl).ratio()
            if r > best:
                best, kind = r, k
    return kind if best >= 0.8 else None


def _is_money_label_line(line: str) -> bool:
    if _RE_TAX_ID.search(line):  # "Tax ID: 84-2219034" is a party detail, not a VAT line
        return False
    return bool(_RE_SUBTOTAL.search(line) or _RE_TOTAL.search(line) or _RE_VAT.match(line)
                or _fuzzy_money_label(line))


# ─── extractor ───────────────────────────────────────────────────────────────


class _Extractor:
    def __init__(self, lines: list[_Line]):
        self.lines = lines
        self.f: dict[str, InvoiceField] = {k: InvoiceField() for k in FIELD_NAMES}
        self.items: list[LineItem] = []
        self.buyer_range: tuple[int, int] = (-1, -1)
        self.seller_range: tuple[int, int] = (-1, -1)
        self.table_range: tuple[int, int] = (-1, -1)

    # -- helpers --
    def _set(self, name: str, value, conf: float, replace: bool = False):
        if value is None or value == "":
            return
        if self.f[name].present and not replace and self.f[name].confidence >= conf:
            return
        self.f[name] = InvoiceField(value, _clip(conf))

    def _in_buyer(self, idx: int) -> bool:
        a, b = self.buyer_range
        return a <= idx <= b

    # -- header / meta fields --
    def extract_invoice_number(self):
        for ln in self.lines:
            # Every label occurrence on the line: "INVOICE   Invoice # BL-1"
            # has a title word first and the real label second.
            for lm in _RE_INVOICE_NO.finditer(ln.text):
                val = _cut_at_next_label(ln.text[lm.end():])
                # "Invoice" alone is only a label when a number-ish token follows.
                val = _RE_DATE.sub(" ", val)
                m = _RE_INVNO_TOKEN.search(val)
                if not m:
                    continue
                tok = m.group(1).strip(".")
                if tok.lower() in ("no", "number", "date", "to") or not re.search(r"\d", tok):
                    continue
                self._set("invoiceNumber", tok, ln.conf)
                return
        # Fallback: a token like INV-2026-001 anywhere.
        for ln in self.lines:
            m = re.search(r"(?<![A-Za-z0-9])(INV[A-Z]*[-/#]?\d[\w\-/]*)", ln.text, re.I)
            if m:
                self._set("invoiceNumber", m.group(1), ln.conf * _FALLBACK_FACTOR)
                return

    def extract_dates(self):
        for ln in self.lines:
            # Due date first so "Due Date" isn't swallowed by the generic "Date".
            due = _after_label(ln.text, _RE_DUE_DATE)
            if due is not None:
                iso = _first_date(due)
                if iso:
                    self._set("dueDate", iso, ln.conf)
                elif re.search(r"\b(on|upon) receipt\b", due, re.I):
                    self._set("paymentTerms", "Due on receipt", ln.conf)
            # Issue date: text after an issue-date label that is not the
            # due-date label itself.
            for m in _RE_ISSUE_DATE.finditer(ln.text):
                pre = ln.text[max(0, m.start() - 12): m.start()]
                if re.search(r"\b(due|pay(?:ment)?)\s*$", pre, re.I):
                    continue
                iso = _first_date(_cut_at_next_label(ln.text[m.end():]))
                if iso:
                    self._set("issueDate", iso, ln.conf)
                    break
        if not self.f["issueDate"].present:
            for ln in self.lines:
                iso = _first_date(ln.text)
                if iso and iso != self.f["dueDate"].value:
                    self._set("issueDate", iso, ln.conf * _FALLBACK_FACTOR)
                    break

    def extract_terms(self):
        for ln in self.lines:
            lm = _RE_TERMS.search(ln.text)
            if not lm:
                continue
            raw = ln.text[lm.end():]
            val = _cut_at_next_label(raw)  # "Due on receipt" starts with a label word
            if re.match(r"\s*(?:due\s+)?(?:on|upon)\s+receipt\b", raw, re.I):
                self._set("paymentTerms", "Due on receipt", ln.conf, replace=True)
                return
            if val:
                m = _RE_NET_TERMS.search(val)
                if m:
                    self._set("paymentTerms", f"Net {int(m.group(1))}", ln.conf, replace=True)
                    return
                m = _RE_DAYS_TERMS.search(val)
                if m:
                    self._set("paymentTerms", f"Net {int(m.group(1))}", ln.conf, replace=True)
                    return
                if re.search(r"\b(on|upon) receipt\b", val, re.I):
                    self._set("paymentTerms", "Due on receipt", ln.conf, replace=True)
                    return
                if re.search(r"\b(cod|cash on delivery)\b", val, re.I):
                    self._set("paymentTerms", "Cash on delivery", ln.conf, replace=True)
                    return
        for ln in self.lines:
            m = _RE_NET_TERMS.search(ln.text)
            if m:
                self._set("paymentTerms", f"Net {int(m.group(1))}", ln.conf * _FALLBACK_FACTOR)
                return

    def extract_currency(self):
        for ln in self.lines:
            val = _after_label(ln.text, _RE_CURRENCY_LBL)
            if val:
                m = re.search(r"\b([A-Z]{3})\b", val)
                if m:
                    self._set("currency", m.group(1), ln.conf)
                    return
        votes: dict[str, list[float]] = {}
        for ln in self.lines:
            for rx, code, base in _CURRENCY_WORDS:
                for m in rx.finditer(ln.text):
                    c = code or m.group(1).upper()
                    votes.setdefault(c, []).append(base * ln.conf)
        if votes:
            best = max(votes, key=lambda k: (len(votes[k]), max(votes[k])))
            conf = max(votes[best]) + 0.01 * (len(votes[best]) - 1)
            self._set("currency", best, conf)

    # -- totals --
    def _labelled_amount(self, rx: re.Pattern, name: str, skip_if=None):
        best: tuple[float, float] | None = None  # (conf, value)
        for ln in self.lines:
            if skip_if and skip_if(ln.text):
                continue
            m = rx.search(ln.text)
            fuzzy = False
            if not m:
                if _fuzzy_money_label(ln.text) != name:
                    continue
                fuzzy = True
            tail = ln.text[m.end():] if m else ln.text
            amts = _amounts(_cut_at_next_label(tail)) or _amounts(tail)
            if not amts:
                # Value may sit on the next line (stacked total boxes).
                nxt = next((l for l in self.lines if l.idx == ln.idx + 1), None)
                if nxt and not _RE_ANY_LABEL.search(nxt.text):
                    amts = _amounts(nxt.text)
                if not amts:
                    continue
            # Specific labels (earlier alternatives) beat the generic one.
            conf = ln.conf * (0.85 if fuzzy else 1.0 if m.group(0).strip().lower() not in ("total", "tax", "date") else 0.9)
            if best is None or conf > best[0]:
                best = (conf, amts[-1])
        if best:
            self._set(name, best[1], best[0])

    def extract_totals(self):
        self._labelled_amount(_RE_SUBTOTAL, "subtotal")
        self._labelled_amount(
            _RE_VAT, "vat",
            skip_if=lambda t: bool(_RE_TAX_ID.search(t)) or bool(_RE_SUBTOTAL.search(t)),
        )
        self._labelled_amount(
            _RE_TOTAL, "total",
            skip_if=lambda t: bool(_RE_SUBTOTAL.search(t)) or _is_table_header(t)
            or _fuzzy_money_label(t) == "subtotal"
            or bool(re.search(r"\btotal (?:vat|tax)\b", t, re.I)),
        )

    # -- parties --
    def _find_block(self, rx: re.Pattern, start: int = 0) -> tuple[int, int, str] | None:
        """Locate a party label; return (first_line, last_line, inline_tail)."""
        for ln in self.lines[start:]:
            m = rx.search(ln.text)
            if not m:
                continue
            lbl = m.group(0).strip().lower().rstrip(":")
            # Bare "to"/"from"/"customer" must start the line to count.
            if lbl in ("to", "from", "customer", "client", "buyer") and m.start() > 0:
                continue
            tail = _cut_at_next_label(ln.text[m.end():]).strip()
            end = ln.idx
            for nxt in self.lines[ln.idx + 1: ln.idx + 7]:
                t = nxt.text
                tm = _RE_EMAIL.sub("x", t)  # "buyer@…" is a detail, not a new block
                if (_is_table_header(t) or _is_money_label_line(t)
                        or _RE_BUYER.match(tm) or _RE_SELLER.match(tm)
                        or re.match(r"^\s*(description|item|qty)\b", t, re.I)):
                    break
                end = nxt.idx
            return ln.idx, end, tail
        return None

    def _party_from_block(self, prefix: str, first: int, last: int, inline: str, conf_scale: float):
        lines = [l for l in self.lines if first <= l.idx <= last]
        name_set = False
        if inline and not _RE_EMAIL.fullmatch(inline):
            nm = _clean_name(_cut_at_next_label(inline))
            if nm and not _RE_TAX_ID.match(nm):
                self._set(f"{prefix}Name", nm, lines[0].conf * conf_scale)
                name_set = True
        for ln in lines:
            t = ln.text if ln.idx != first else ln.text[ln.text.lower().find(inline.lower()) + len(inline):] if inline else ""
            if ln.idx == first and not inline:
                continue
            if ln.idx != first:
                t = _cut_at_next_label(ln.text) if _RE_ANY_LABEL.search(ln.text) and not _RE_TAX_ID.search(ln.text) \
                    and not _RE_EMAIL_LBL.search(ln.text) and not _RE_PHONE_LBL.search(ln.text) else ln.text
            self._party_details(prefix, t, ln.conf * conf_scale)
            if not name_set and ln.idx != first:
                cand = _clean_name(_RE_PHONE.sub("", _cut_at_next_label(ln.text)))
                if cand and not _RE_TAX_ID.search(ln.text) and not _RE_EMAIL.search(ln.text) \
                        and not _RE_PHONE_LBL.match(ln.text) and re.search(r"[A-Za-z]{2,}", cand):
                    self._set(f"{prefix}Name", cand, ln.conf * conf_scale)
                    name_set = True

    def _party_details(self, prefix: str, text: str, conf: float):
        m = _RE_EMAIL.search(text)
        if m:
            self._set(f"{prefix}Email", m.group(0).lower(), conf)
        tid = _after_label(text, _RE_TAX_ID)
        if tid is not None:
            mt = _RE_TAX_TOKEN.search(tid.upper())
            if mt:
                self._set(f"{prefix}TaxId", mt.group(1), conf)
        if prefix == "buyer":
            ph_text = _after_label(text, _RE_PHONE_LBL)
            src = ph_text if ph_text is not None else text
            src = _RE_EMAIL.sub(" ", src)
            if ph_text is not None or re.search(r"\+\d", src):
                for mp in _RE_PHONE.finditer(src):
                    digits = re.sub(r"\D", "", mp.group(1))
                    if 9 <= len(digits) <= 15:
                        self._set("buyerPhone", re.sub(r"\s{2,}", " ", mp.group(1).strip()),
                                  conf if ph_text is not None else conf * _FALLBACK_FACTOR)
                        break

    def _two_column_parties(self, first: int, last: int, seller_left: bool):
        """'From:' and 'Bill To:' share a line: the lines below carry both
        parties side by side. Assign each e-mail / tax id / phone by its
        order (two found) or position (one found); split the first line's
        company names at the first legal-form suffix (Ltd, LLC, Inc, ...)."""
        self.buyer_range = self.seller_range = (first, last)
        lines = [l for l in self.lines if first < l.idx <= last]
        left, right = ("seller", "buyer") if seller_left else ("buyer", "seller")

        def side(start: int, text: str) -> str:
            return left if start < len(text) / 2 else right

        names_done = False
        for ln in lines:
            t = ln.text
            if not names_done and not _RE_EMAIL.search(t) and not _RE_TAX_ID.search(t) and not _RE_PHONE_LBL.search(t):
                m = _COMPANY_HINTS.search(t)
                if m and m.end() < len(t) - 2:
                    a, b = t[: m.end()], t[m.end():]
                else:
                    toks = t.split()
                    a, b = " ".join(toks[: len(toks) // 2]), " ".join(toks[len(toks) // 2:])
                for prefix, chunk in ((left, a), (right, b)):
                    nm = _clean_name(_cut_at_next_label(chunk))
                    if nm and re.search(r"[A-Za-z]{2,}", nm):
                        self._set(f"{prefix}Name", nm, ln.conf * (0.95 if m else 0.75))
                names_done = True
                continue
            emails = list(_RE_EMAIL.finditer(t))
            for i, m in enumerate(emails):
                prefix = (left, right)[i] if len(emails) == 2 else side(m.start(), t)
                self._set(f"{prefix}Email", m.group(0).lower(), ln.conf)
            tids = []
            for m in _RE_TAX_ID.finditer(t):
                mt = _RE_TAX_TOKEN.search((_cut_at_next_label(t[m.end():]) or t[m.end():]).upper())
                if mt:
                    tids.append((m.start(), mt.group(1)))
            for i, (pos, tok) in enumerate(tids):
                prefix = (left, right)[i] if len(tids) == 2 else side(pos, t)
                self._set(f"{prefix}TaxId", tok, ln.conf)
            src = _RE_EMAIL.sub(lambda m: " " * len(m.group(0)), t)
            phones = [(m.start(), m.group(1)) for m in _RE_PHONE.finditer(src)
                      if 9 <= len(re.sub(r"\D", "", m.group(1))) <= 15]
            if phones and (_RE_PHONE_LBL.search(t) or re.search(r"\+\d", t)):
                for i, (pos, ph) in enumerate(phones):
                    prefix = (left, right)[i] if len(phones) == 2 else side(pos, t)
                    if prefix == "buyer":
                        self._set("buyerPhone", re.sub(r"\s{2,}", " ", ph.strip()), ln.conf)

    def extract_parties(self):
        buyer = self._find_block(_RE_BUYER)
        seller = self._find_block(_RE_SELLER)
        if buyer and seller and buyer[0] == seller[0]:
            line = self.lines[buyer[0]].text
            seller_left = _RE_SELLER.search(line).start() < _RE_BUYER.search(line).start()
            self._two_column_parties(buyer[0], max(buyer[1], seller[1]), seller_left)
            return
        if buyer:
            self.buyer_range = (buyer[0], buyer[1])
            self._party_from_block("buyer", buyer[0], buyer[1], buyer[2], 1.0)

        if seller and not self._in_buyer(seller[0]):
            self.seller_range = (seller[0], seller[1])
            self._party_from_block("seller", seller[0], seller[1], seller[2], 1.0)
        else:
            # Header block: everything above the buyer block / table.
            stop = self.buyer_range[0] if self.buyer_range[0] >= 0 else len(self.lines)
            header = [l for l in self.lines[:stop] if not _is_table_header(l.text)]
            if not header:
                header = self.lines[:6]
            for ln in header[:8]:
                self._party_details("seller", ln.text, ln.conf * 0.95)
            for ln in header[:6]:
                t = _cut_at_next_label(ln.text)
                cand = _clean_name(t)
                if not cand or _TITLE_WORDS.match(cand) or _RE_EMAIL.search(ln.text) \
                        or _RE_TAX_ID.search(ln.text) or _RE_PHONE_LBL.match(ln.text) \
                        or re.search(r"\bp\.?o\.? box\b", cand, re.I) or not re.search(r"[A-Za-z]{3,}", cand):
                    continue
                boost = 1.0 if _COMPANY_HINTS.search(cand) else 0.85
                self._set("sellerName", cand, ln.conf * 0.95 * boost)
                break
        # Seller details anywhere outside the buyer block (footer PIN, email).
        for ln in self.lines:
            if self._in_buyer(ln.idx):
                continue
            if not self.f["sellerTaxId"].present or not self.f["sellerEmail"].present:
                self._party_details("seller", ln.text, ln.conf * 0.9)

    # -- line items --
    def extract_line_items(self):
        header_idx = next((l.idx for l in self.lines if _is_table_header(l.text)), -1)
        if header_idx < 0:
            return
        end = len(self.lines)
        for ln in self.lines[header_idx + 1:]:
            if _is_money_label_line(ln.text) or _RE_TERMS.match(ln.text) or re.match(r"^\s*(bank|notes?|thank)", ln.text, re.I):
                end = ln.idx
                break
        self.table_range = (header_idx, end)
        for ln in self.lines[header_idx + 1: end]:
            item = self._parse_item(ln)
            if item:
                self.items.append(item)

    @staticmethod
    def _parse_item(ln: _Line) -> LineItem | None:
        text = re.sub(r"^\s*\d{1,2}[.)]\s+", "", ln.text)  # leading "1." index
        text = _norm_numeric_noise(_RE_PERCENT.sub(" ", text))
        text_nodate = _RE_DATE.sub(lambda m: " " * len(m.group(0)), text)
        matches = list(_RE_AMOUNT.finditer(text_nodate))
        if not matches:
            return None
        # Description = text before the first number that is followed only by
        # numbers / currency codes to the end of the line.
        nums = [parse_amount(m.group(1)) for m in matches]
        desc_end = len(text)
        tail_start = None
        for m in matches:
            rest = text_nodate[m.start():]
            rest = re.sub(r"\b(?:KES|USD|EUR|ETB|GBP|UGX|TZS|Kshs?|KSh|Br)\b|[$€£]", "", rest, flags=re.I)
            if re.fullmatch(r"[\s\d.,;\-\u00a0]*", rest):
                tail_start = m.start()
                break
        if tail_start is None:
            tail_start = matches[0].start()
        desc_end = tail_start
        tail_ms = [m for m in matches if m.start() >= tail_start and parse_amount(m.group(1)) is not None]
        if not tail_ms:
            tail_ms = [m for m in matches if parse_amount(m.group(1)) is not None]
        tail_nums = [parse_amount(m.group(1)) for m in tail_ms]
        if len(tail_nums) > 3:
            # "roll of 1000  6  92.50  555.00": the numeric tail swallowed a
            # number that belongs to the description. Pick the qty whose
            # product matches the amount and give the rest back.
            unit_, amount_ = tail_nums[-2], tail_nums[-1]
            for i in range(len(tail_nums) - 2):
                if abs(tail_nums[i] * unit_ - amount_) <= max(0.02, 0.005 * abs(amount_)):
                    desc_end = tail_ms[i].start()
                    tail_nums = tail_nums[i:]
                    break
        desc = _clean_name(text[:desc_end])
        if not tail_nums or not re.search(r"[A-Za-z]{2,}", desc):
            return None
        qty = unit = amount = None
        if len(tail_nums) >= 3:
            qty, unit, amount = tail_nums[0], tail_nums[-2], tail_nums[-1]
        elif len(tail_nums) == 2:
            qty, amount = tail_nums
            unit = round(amount / qty, 4) if qty else None
        else:
            amount = tail_nums[0]
            qty, unit = 1.0, amount
        conf = ln.conf
        if qty is not None and unit is not None and amount is not None:
            if abs(qty * unit - amount) <= max(0.02, 0.005 * abs(amount)):
                conf = _clip(conf + _CONSISTENT_BOOST)
            else:
                conf *= _INCONSISTENT_FACTOR
        if len(tail_nums) == 1:
            conf *= 0.9
        return LineItem(desc, _num(qty), _num(unit), _num(amount), conf)

    # -- consistency --
    def reconcile(self):
        f = self.f
        sub, vat, tot = f["subtotal"], f["vat"], f["total"]
        tol = lambda v: max(0.05, 0.005 * abs(v))  # noqa: E731

        # Derive missing members of subtotal + vat = total.
        if sub.present and vat.present and not tot.present:
            self._set("total", _num(sub.value + vat.value), min(sub.confidence, vat.confidence) * _DERIVED_FACTOR)
        elif sub.present and tot.present and not vat.present and tot.value >= sub.value:
            self._set("vat", _num(tot.value - sub.value), min(sub.confidence, tot.confidence) * _DERIVED_FACTOR)
        elif vat.present and tot.present and not sub.present and tot.value >= vat.value:
            self._set("subtotal", _num(tot.value - vat.value), min(vat.confidence, tot.confidence) * _DERIVED_FACTOR)
        elif tot.present and not sub.present and not vat.present and self.items:
            s = sum(i.amount for i in self.items if i.amount is not None)
            if abs(s - tot.value) <= tol(tot.value):
                self._set("subtotal", _num(s), tot.confidence * _DERIVED_FACTOR)
                self._set("vat", 0.0, tot.confidence * _DERIVED_FACTOR)

        sub, vat, tot = f["subtotal"], f["vat"], f["total"]
        if sub.present and vat.present and tot.present:
            if abs(sub.value + vat.value - tot.value) <= tol(tot.value):
                for k in ("subtotal", "vat", "total"):
                    f[k].confidence = _clip(f[k].confidence + _CONSISTENT_BOOST)
            else:
                for k in ("subtotal", "vat", "total"):
                    f[k].confidence *= _INCONSISTENT_FACTOR

        # Line items vs subtotal (or total when there is no subtotal).
        if self.items:
            s = sum(i.amount for i in self.items if i.amount is not None)
            ref = f["subtotal"] if f["subtotal"].present else f["total"]
            if ref.present:
                if abs(s - ref.value) <= tol(ref.value):
                    ref.confidence = _clip(ref.confidence + _CONSISTENT_BOOST)
                    for i in self.items:
                        i.confidence = _clip(i.confidence + _CONSISTENT_BOOST)
                else:
                    for i in self.items:
                        i.confidence *= 0.8

        # Payment terms -> due date when absent.
        terms, issue, due = f["paymentTerms"], f["issueDate"], f["dueDate"]
        if terms.present and issue.present and not due.present:
            m = re.fullmatch(r"Net (\d+)", str(terms.value))
            if m:
                d = date.fromisoformat(issue.value) + timedelta(days=int(m.group(1)))
                self._set("dueDate", d.isoformat(), min(terms.confidence, issue.confidence) * _DERIVED_FACTOR)
            elif terms.value == "Due on receipt":
                self._set("dueDate", issue.value, min(terms.confidence, issue.confidence) * _DERIVED_FACTOR)
        issue, due = f["issueDate"], f["dueDate"]
        if issue.present and due.present:
            if due.value < issue.value:
                issue.confidence *= 0.6
                due.confidence *= 0.6
            else:
                m = re.fullmatch(r"Net (\d+)", str(terms.value or ""))
                if m and (date.fromisoformat(due.value) - date.fromisoformat(issue.value)).days == int(m.group(1)):
                    due.confidence = _clip(due.confidence + _CONSISTENT_BOOST)
                    terms.confidence = _clip(terms.confidence + _CONSISTENT_BOOST)

    def run(self) -> tuple[dict[str, InvoiceField], list[LineItem]]:
        self.extract_invoice_number()
        self.extract_dates()
        self.extract_terms()
        self.extract_currency()
        self.extract_totals()
        self.extract_parties()
        self.extract_line_items()
        self.reconcile()
        for k in ("subtotal", "vat", "total"):
            if self.f[k].present:
                self.f[k].value = _num(self.f[k].value)
        return self.f, self.items


def _num(v):
    """Round money to cents; keep ints as ints for qty."""
    if v is None:
        return None
    v = round(float(v), 4)
    return int(v) if v == int(v) else v


def extract_invoice(words: list[OcrWord], text: str, engine: str = "") -> InvoiceExtraction:
    """Extract invoice fields from OCR / PDF-text words and full text."""
    lines = _build_lines(words, text)
    fields, items = _Extractor(lines).run()
    return InvoiceExtraction(engine=engine, fields=fields, line_items=items,
                             word_count=len([w for w in words if w.text.strip()]))
