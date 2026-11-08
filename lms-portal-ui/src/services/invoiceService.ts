/**
 * Nemo invoice-finance service client.
 * Typed against docs/nemo/invoice-finance/SPEC.md §2 (domain) and §6 (HTTP API).
 *
 * Gateway prefix: /lms/api/v1/{invoices,buyers,invoice-policy}
 * Public (no JWT): /lms/api/public/invoices/...
 */
import { apiGet, apiPost, apiPut, apiPatch, type PageResponse } from "@/lib/api";

const BASE = "/lms/api/v1";
const PUBLIC_BASE = "/lms/api/public/invoices";
const JWT_KEY = "athena_jwt";

// ─── Domain types ──────────────────────────────────────────────────────────

export type InvoiceStatus =
  | "UPLOADED"
  | "EXTRACTED"
  | "AWAITING_BUYER"
  | "BUYER_CONFIRMED"
  | "BUYER_DISPUTED"
  | "SCORED"
  | "DECIDED"
  | "OFFERED"
  | "ACCEPTED"
  | "FUNDED"
  | "PARTIALLY_PAID"
  | "PAID"
  | "OVERDUE"
  | "DECLINED"
  | "CANCELLED";

export const INVOICE_STATUSES: InvoiceStatus[] = [
  "UPLOADED", "EXTRACTED", "AWAITING_BUYER", "BUYER_CONFIRMED", "BUYER_DISPUTED", "SCORED", "DECIDED",
  "OFFERED", "ACCEPTED", "FUNDED", "PARTIALLY_PAID", "PAID", "OVERDUE", "DECLINED", "CANCELLED",
];

export type DecisionOutcome = "APPROVE" | "REFER" | "DECLINE";
export type BuyerTier = "A" | "B" | "C" | "D" | "UNRATED";
export type OfferStatus = "OFFERED" | "ACCEPTED" | "EXPIRED" | "WITHDRAWN";
export type AdvanceStatus = "ACTIVE" | "PAID" | "OVERDUE" | "DILUTED" | "WRITTEN_OFF";

export interface Party {
  name?: string;
  taxId?: string;
  email?: string;
  phone?: string;
}

export interface LineItem {
  description: string;
  qty: number;
  unitPrice: number;
  amount: number;
}

export interface OcrInfo {
  engine?: string;
  /** confidence per extracted field, 0..1 */
  confidence?: Record<string, number>;
  rawWordCount?: number;
}

export interface Invoice {
  id: string;
  tenantId?: string;
  sellerCustomerId: string;
  fingerprint?: string;
  fieldsHash?: string;
  mediaId?: string;
  invoiceNumber?: string;
  issueDate?: string;
  dueDate?: string;
  currency: string;
  subtotal?: number;
  vat?: number;
  total: number;
  seller?: Party;
  buyerId?: string;
  buyerSnapshot?: Party;
  lineItems?: LineItem[];
  ocr?: OcrInfo;
  requestedAmount?: number;
  status: InvoiceStatus;
  statusReason?: string;
  /** fields with OCR confidence < 0.6 (seller must confirm) */
  needsSellerConfirmation?: string[];
  fraudFlags?: string[];
  decisionOutcome?: DecisionOutcome;
  invoiceScore?: number;
  sellerBand?: string;
  pipelineError?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface Buyer {
  id: string;
  tenantId?: string;
  name: string;
  taxId?: string;
  emailDomain?: string;
  contacts?: Party[] | Record<string, unknown>;
  tier: BuyerTier;
  limitAmount?: number;
  exposureAmount?: number;
  onTimeRate?: number;
  confirmationsCount?: number;
  createdAt?: string;
}

export interface Offer {
  id: string;
  invoiceId: string;
  advanceRate: number;
  advanceAmount: number;
  discountFeePerDay: number;
  tenorDays: number;
  feeAmount: number;
  processingFee: number;
  netDisbursement: number;
  reserveAmount: number;
  expiresAt?: string;
  status: OfferStatus;
}

export interface Advance {
  id: string;
  invoiceId: string;
  offerId?: string;
  coreRef?: string;
  disbursedAt?: string;
  dueDate?: string;
  principal: number;
  feesAccrued: number;
  repaid: number;
  reserveReleased: number;
  status: AdvanceStatus;
}

export interface Decision {
  id?: string;
  invoiceId?: string;
  outcome: DecisionOutcome;
  reasons?: string[];
  ruleId?: string;
  policyId?: string;
  policyVersion?: number | string;
  policyHash?: string;
  inputs?: Record<string, unknown>;
  detail?: { advanceRate?: number; feePerDay?: number; maxAmount?: number } & Record<string, unknown>;
  fraudFlags?: string[];
  decidedAt?: string;
}

export interface LedgerEvent {
  seq: number;
  invoiceId: string;
  type: string;
  payload?: Record<string, unknown>;
  actor?: string;
  occurredAt: string;
  prevHash?: string;
  hash: string;
  sig?: string;
  anchorId?: string | null;
}

export interface Timeline {
  events: LedgerEvent[];
  chainOk: boolean;
}

export interface Anchor {
  id: string;
  merkleRoot: string;
  fromSeq: number;
  toSeq: number;
  method: "OTS" | "NONE" | string;
  receipt?: string;
  anchoredAt: string;
}

export interface InvoiceDetail extends Invoice {
  offer?: Offer | null;
  advance?: Advance | null;
  decision?: Decision | null;
  buyer?: Buyer | null;
  /** present in DEMO_MODE on the upload response */
  confirmLink?: string;
}

export interface UploadResponse extends InvoiceDetail {
  confirmLink?: string;
  confirmUrl?: string;
}

export interface TierStat {
  tier: BuyerTier | string;
  count: number;
  amount: number;
}

export interface Stats {
  fundedAmount: number;
  fundedCount?: number;
  outstandingAmount: number;
  overdueAmount: number;
  overdueCount?: number;
  avgTenorDays: number;
  currency?: string;
  byStatus: Record<string, number>;
  byBuyerTier: TierStat[];
}

export interface QueueItem {
  invoiceId: string;
  invoiceNumber?: string;
  sellerCustomerId?: string;
  sellerName?: string;
  buyerName?: string;
  buyerTier?: BuyerTier | string;
  total?: number;
  currency?: string;
  status?: InvoiceStatus;
  outcome?: DecisionOutcome | "FLAG" | string;
  reasons: string[];
  fraudFlags: string[];
  decidedAt?: string;
  createdAt?: string;
}

export interface Policy {
  yaml: string;
  version: number | string;
  hash: string;
  updatedAt?: string;
  updatedBy?: string;
}

export interface ConfirmSummary {
  invoiceNumber?: string;
  sellerName?: string;
  buyerName?: string;
  total: number;
  currency: string;
  issueDate?: string;
  dueDate?: string;
  lineItems?: LineItem[];
  expiresAt?: string;
  status?: string;
  /** set when the token has already been acted on */
  action?: "CONFIRMED" | "DISPUTED" | "ALREADY_PAID" | null;
  actedAt?: string;
  otpRequired?: boolean;
}

export type ConfirmAction = "CONFIRMED" | "DISPUTED" | "ALREADY_PAID";

export interface ConfirmResult {
  action: ConfirmAction;
  status?: string;
  message?: string;
}

export interface VerifyResult {
  known: boolean;
  fingerprint?: string;
  status?: string;
  buyerConfirmedAt?: string | null;
  fundedAt?: string | null;
  anchoredAt?: string | null;
  chainOk?: boolean;
}

export interface InvoiceListFilter {
  status?: InvoiceStatus | "";
  sellerId?: string;
  buyerId?: string;
  page?: number;
  size?: number;
}

// ─── helpers ──────────────────────────────────────────────────────────────

function unwrap<T>(r: { data: T | null; error: string | null }, fallbackMsg: string): T {
  if (r.error || r.data === null || r.data === undefined) throw new Error(r.error ?? fallbackMsg);
  return r.data;
}

/** Fetch without the JSON content-type (multipart), with JWT. */
async function multipart<T>(url: string, form: FormData, withAuth: boolean): Promise<T> {
  const headers: Record<string, string> = {};
  if (withAuth) {
    const token = localStorage.getItem(JWT_KEY);
    if (token) headers["Authorization"] = `Bearer ${token}`;
  }
  const res = await fetch(url, { method: "POST", headers, body: form });
  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }
  if (!res.ok) {
    const msg =
      typeof body === "object" && body !== null && "message" in body
        ? String((body as Record<string, unknown>).message)
        : res.status === 409
          ? "Duplicate invoice — this document (or seller/invoice number/buyer combination) was already submitted."
          : `Request failed with status ${res.status}`;
    const err = new Error(msg) as Error & { status?: number; body?: unknown };
    err.status = res.status;
    err.body = body;
    throw err;
  }
  return body as T;
}

/** Public endpoints: plain fetch, no Authorization header. */
async function publicRequest<T>(url: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(url, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init.headers as Record<string, string> | undefined) },
  });
  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }
  if (!res.ok) {
    const msg =
      typeof body === "object" && body !== null && "message" in body
        ? String((body as Record<string, unknown>).message)
        : res.status === 404
          ? "Not found"
          : res.status === 410
            ? "This link has expired"
            : `Request failed with status ${res.status}`;
    const err = new Error(msg) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  return body as T;
}

/** Normalise `GET /invoices` which may return a Spring-style page or a bare array. */
function toPage<T>(raw: unknown, page: number, size: number): PageResponse<T> {
  if (Array.isArray(raw)) {
    return { content: raw as T[], totalElements: raw.length, totalPages: 1, size, number: page, first: true, last: true };
  }
  const r = (raw ?? {}) as Partial<PageResponse<T>> & { items?: T[]; total?: number; totalItems?: number; page?: number };
  const content = r.content ?? r.items ?? [];
  const total = r.totalElements ?? r.totalItems ?? r.total ?? content.length;
  if (r.number === undefined && r.page !== undefined) r.number = r.page;
  return {
    content,
    totalElements: total,
    totalPages: r.totalPages ?? Math.max(1, Math.ceil(total / size)),
    size: r.size ?? size,
    number: r.number ?? page,
    first: r.first ?? page === 0,
    last: r.last ?? (page + 1) * size >= total,
  };
}

/** Go `Party` on the invoice is `buyer`; the UI reads `buyerSnapshot`. Decision reasons arrive as objects. */
function normaliseInvoice(raw: Record<string, unknown>): Invoice {
  const inv = { ...(raw as unknown as Invoice) } as Invoice & { buyer?: unknown };
  const party = raw.buyer as Party | undefined;
  if (!inv.buyerSnapshot && party && !("tier" in (party as object))) inv.buyerSnapshot = party;
  if (inv.total === undefined || inv.total === null) inv.total = Number(raw.total ?? 0);
  inv.currency = inv.currency || "KES";
  return inv;
}

function normaliseDecision(raw: unknown): Decision | null {
  if (!raw || typeof raw !== "object") return null;
  const d = raw as Record<string, unknown>;
  const reasons = Array.isArray(d.reasons)
    ? (d.reasons as unknown[]).map((r) => {
        if (typeof r === "string") return r;
        const o = r as { code?: string; ruleId?: string; detail?: string };
        return [o.code, o.detail].filter(Boolean).join(" — ") || o.ruleId || "";
      }).filter(Boolean)
    : [];
  const det = (d.detail ?? {}) as Record<string, unknown>;
  const detail = {
    ...det,
    advanceRate: Number(det.advanceRate ?? det.advance_rate ?? NaN) || undefined,
    feePerDay: Number(det.feePerDay ?? det.fee_per_day ?? NaN) || undefined,
    maxAmount: Number(det.maxAmount ?? det.max_amount ?? NaN) || undefined,
  };
  const ruleId = Array.isArray(d.reasons) && d.reasons.length ? ((d.reasons as { ruleId?: string }[])[0].ruleId) : undefined;
  return { ...(d as unknown as Decision), reasons, detail, ruleId: (d.ruleId as string) ?? ruleId };
}

/** GET /invoices/{id} (and every action) returns `{invoice, buyer, offer, advance, decision, payments, confirmation, confirmUrl}`. */
function normaliseDetail(raw: unknown): InvoiceDetail {
  const r = (raw ?? {}) as Record<string, unknown>;
  const base = (r.invoice && typeof r.invoice === "object" ? r.invoice : r) as Record<string, unknown>;
  const inv = normaliseInvoice(base) as InvoiceDetail;
  const buyer = r.buyer as Buyer | undefined;
  inv.buyer = buyer && typeof buyer === "object" && "tier" in buyer ? buyer : null;
  inv.offer = (r.offer as Offer) ?? null;
  inv.advance = (r.advance as Advance) ?? null;
  inv.decision = normaliseDecision(r.decision);
  const link = (r.confirmUrl ?? r.confirmLink) as string | undefined;
  if (link) inv.confirmLink = link;
  return inv;
}

function normaliseQueueItem(raw: Record<string, unknown>): QueueItem {
  const inv = (raw.invoice ?? {}) as Partial<Invoice>;
  const dec = (normaliseDecision(raw.decision) ?? {}) as Partial<Decision>;
  const evidence = raw.evidence as Record<string, unknown> | string[] | undefined;
  const reasons =
    (raw.reasons as string[]) ??
    dec.reasons ??
    (Array.isArray(evidence) ? evidence : ((evidence?.reasons as string[]) ?? [])) ??
    [];
  const fraudFlags =
    (raw.fraudFlags as string[]) ??
    dec.fraudFlags ??
    (!Array.isArray(evidence) ? ((evidence?.fraudFlags as string[]) ?? []) : []) ??
    [];
  return {
    invoiceId: String(raw.invoiceId ?? inv.id ?? raw.id ?? ""),
    invoiceNumber: (raw.invoiceNumber as string) ?? inv.invoiceNumber,
    sellerCustomerId: (raw.sellerCustomerId as string) ?? inv.sellerCustomerId,
    sellerName: (raw.sellerName as string) ?? inv.seller?.name,
    buyerName: (raw.buyerName as string) ?? inv.buyerSnapshot?.name ?? (inv as unknown as { buyer?: Party }).buyer?.name ?? (raw.buyer as Partial<Buyer>)?.name,
    buyerTier: (raw.buyerTier as string) ?? ((raw.buyer as Partial<Buyer>)?.tier),
    total: (raw.total as number) ?? inv.total,
    currency: (raw.currency as string) ?? inv.currency ?? "KES",
    status: (raw.status as InvoiceStatus) ?? inv.status,
    outcome: (raw.outcome as string) ?? (raw.gate === "SECOND_APPROVAL" ? "SECOND_APPROVAL" : undefined) ?? dec.outcome ?? (fraudFlags.length ? "FLAG" : "REFER"),
    reasons: Array.isArray(reasons) ? reasons : [],
    fraudFlags: Array.isArray(fraudFlags) && fraudFlags.length ? fraudFlags : (inv.fraudFlags ?? []),
    decidedAt: (raw.decidedAt as string) ?? dec.decidedAt,
    createdAt: (raw.createdAt as string) ?? inv.createdAt,
  };
}

function normaliseStats(raw: Record<string, unknown>): Stats {
  const tierRaw = raw.byBuyerTier ?? raw.byTier ?? [];
  let byBuyerTier: TierStat[] = [];
  if (Array.isArray(tierRaw)) {
    byBuyerTier = tierRaw.map((t) => {
      const o = t as Record<string, unknown>;
      return { tier: String(o.tier ?? "UNRATED"), count: Number(o.count ?? 0), amount: Number(o.amount ?? o.fundedAmount ?? 0) };
    });
  } else if (tierRaw && typeof tierRaw === "object") {
    byBuyerTier = Object.entries(tierRaw as Record<string, unknown>).map(([tier, v]) =>
      typeof v === "object" && v !== null
        ? { tier, count: Number((v as Record<string, unknown>).count ?? 0), amount: Number((v as Record<string, unknown>).amount ?? 0) }
        : { tier, count: Number(v ?? 0), amount: 0 }
    );
  }
  return {
    fundedAmount: Number(raw.fundedAmount ?? raw.fundedTotal ?? raw.funded ?? 0),
    fundedCount: Number(raw.fundedCount ?? 0),
    outstandingAmount: Number(raw.outstandingAmount ?? raw.outstanding ?? 0),
    overdueAmount: Number(raw.overdueAmount ?? raw.overdueTotal ?? raw.overdue ?? 0),
    overdueCount: Number(raw.overdueCount ?? 0),
    avgTenorDays: Number(raw.avgTenorDays ?? raw.avgTenor ?? 0),
    currency: (raw.currency as string) ?? "KES",
    byStatus: ((raw.byStatus ?? raw.countsByStatus) as Record<string, number>) ?? {},
    byBuyerTier,
  };
}

// ─── service ──────────────────────────────────────────────────────────────

export const invoiceService = {
  // Invoices
  async upload(input: { file: File; sellerCustomerId: string; buyerEmail?: string; requestedAmount?: number }): Promise<UploadResponse> {
    const form = new FormData();
    form.append("file", input.file);
    form.append("sellerCustomerId", input.sellerCustomerId);
    if (input.buyerEmail) form.append("buyerEmail", input.buyerEmail);
    if (input.requestedAmount !== undefined && Number.isFinite(input.requestedAmount)) {
      form.append("requestedAmount", String(input.requestedAmount));
    }
    const raw = await multipart<unknown>(`${BASE}/invoices`, form, true);
    return normaliseDetail(raw) as UploadResponse;
  },

  async list(filter: InvoiceListFilter = {}): Promise<PageResponse<Invoice>> {
    const page = filter.page ?? 0;
    const size = filter.size ?? 20;
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    if (filter.status) params.set("status", filter.status);
    if (filter.sellerId) params.set("sellerId", filter.sellerId);
    if (filter.buyerId) params.set("buyerId", filter.buyerId);
    const raw = unwrap(await apiGet<unknown>(`${BASE}/invoices?${params}`), "Failed to list invoices");
    const page_ = toPage<Record<string, unknown>>(raw, page, size);
    return { ...page_, content: page_.content.map(normaliseInvoice) };
  },

  async get(id: string): Promise<InvoiceDetail> {
    return normaliseDetail(unwrap(await apiGet<unknown>(`${BASE}/invoices/${encodeURIComponent(id)}`), "Failed to load invoice"));
  },

  async timeline(id: string): Promise<Timeline> {
    const raw = unwrap(await apiGet<unknown>(`${BASE}/invoices/${encodeURIComponent(id)}/timeline`), "Failed to load timeline");
    if (Array.isArray(raw)) return { events: raw as LedgerEvent[], chainOk: true };
    const r = raw as Partial<Timeline>;
    return { events: r.events ?? [], chainOk: Boolean(r.chainOk) };
  },

  async resendConfirmation(id: string): Promise<{ confirmLink?: string } | null> {
    const r = await apiPost<{ confirmLink?: string; confirmUrl?: string }>(`${BASE}/invoices/${encodeURIComponent(id)}/resend-confirmation`);
    if (r.error) throw new Error(r.error);
    if (r.data?.confirmUrl && !r.data.confirmLink) r.data.confirmLink = r.data.confirmUrl;
    return r.data;
  },

  async acceptOffer(id: string): Promise<InvoiceDetail | null> {
    const r = await apiPost<unknown>(`${BASE}/invoices/${encodeURIComponent(id)}/offer/accept`);
    if (r.error) throw new Error(r.error);
    return r.data ? normaliseDetail(r.data) : null;
  },

  async approve(id: string, note?: string): Promise<InvoiceDetail | null> {
    const r = await apiPost<unknown>(`${BASE}/invoices/${encodeURIComponent(id)}/approve`, { note: note ?? "" });
    if (r.error) throw new Error(r.error);
    return r.data ? normaliseDetail(r.data) : null;
  },

  async reject(id: string, reason: string): Promise<InvoiceDetail | null> {
    const r = await apiPost<unknown>(`${BASE}/invoices/${encodeURIComponent(id)}/reject`, { reason });
    if (r.error) throw new Error(r.error);
    return r.data ? normaliseDetail(r.data) : null;
  },

  async fund(id: string): Promise<InvoiceDetail | null> {
    const r = await apiPost<unknown>(`${BASE}/invoices/${encodeURIComponent(id)}/fund`);
    if (r.error) throw new Error(r.error);
    return r.data ? normaliseDetail(r.data) : null;
  },

  async recordPayment(id: string, payment: { amount: number; reference: string; payer?: string }): Promise<InvoiceDetail | null> {
    const r = await apiPost<unknown>(`${BASE}/invoices/${encodeURIComponent(id)}/payments`, payment);
    if (r.error) throw new Error(r.error);
    return r.data ? normaliseDetail(r.data) : null;
  },

  async queue(): Promise<QueueItem[]> {
    const raw = unwrap(await apiGet<unknown>(`${BASE}/invoices/queue`), "Failed to load review queue");
    const arr = Array.isArray(raw) ? raw : ((raw as { items?: unknown[]; content?: unknown[] }).items ?? (raw as { content?: unknown[] }).content ?? []);
    return (arr as Record<string, unknown>[]).map(normaliseQueueItem);
  },

  async stats(): Promise<Stats> {
    const raw = unwrap(await apiGet<Record<string, unknown>>(`${BASE}/invoices/stats`), "Failed to load stats");
    return normaliseStats(raw);
  },

  async anchors(): Promise<Anchor[]> {
    const raw = unwrap(await apiGet<unknown>(`${BASE}/invoices/anchors`), "Failed to load anchors");
    return Array.isArray(raw) ? (raw as Anchor[]) : ((raw as { items?: Anchor[]; content?: Anchor[] }).items ?? (raw as { content?: Anchor[] }).content ?? []);
  },

  // Buyers
  async listBuyers(): Promise<Buyer[]> {
    const raw = unwrap(await apiGet<unknown>(`${BASE}/buyers`), "Failed to list buyers");
    return Array.isArray(raw) ? (raw as Buyer[]) : ((raw as { content?: Buyer[]; items?: Buyer[] }).content ?? (raw as { items?: Buyer[] }).items ?? []);
  },

  async getBuyer(id: string): Promise<Buyer> {
    return unwrap(await apiGet<Buyer>(`${BASE}/buyers/${encodeURIComponent(id)}`), "Failed to load buyer");
  },

  async updateBuyer(id: string, patch: { tier?: BuyerTier; limitAmount?: number }): Promise<Buyer> {
    return unwrap(await apiPatch<Buyer>(`${BASE}/buyers/${encodeURIComponent(id)}`, patch), "Failed to update buyer");
  },

  // Policy
  async getPolicy(): Promise<Policy> {
    return unwrap(await apiGet<Policy>(`${BASE}/invoice-policy`), "Failed to load policy");
  },

  async putPolicy(yaml: string): Promise<Policy> {
    return unwrap(await apiPut<Policy>(`${BASE}/invoice-policy`, { yaml }), "Failed to save policy");
  },

  // Public (no JWT)
  async publicConfirmSummary(token: string): Promise<ConfirmSummary> {
    return publicRequest<ConfirmSummary>(`${PUBLIC_BASE}/confirm/${encodeURIComponent(token)}`);
  },

  async publicConfirm(token: string, body: { action: ConfirmAction; otp?: string; disputeReason?: string }): Promise<ConfirmResult> {
    return publicRequest<ConfirmResult>(`${PUBLIC_BASE}/confirm/${encodeURIComponent(token)}`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  async publicVerifyFingerprint(fingerprint: string): Promise<VerifyResult> {
    return publicRequest<VerifyResult>(`${PUBLIC_BASE}/verify/${encodeURIComponent(fingerprint)}`);
  },

  async publicVerifyFile(file: File): Promise<VerifyResult> {
    const form = new FormData();
    form.append("file", file);
    return multipart<VerifyResult>(`${PUBLIC_BASE}/verify`, form, false);
  },

  /** Media download URL for the stored invoice PDF (media-service, JWT required). */
  mediaDownloadUrl(mediaId: string): string {
    return `/proxy/media/api/v1/media/download/${encodeURIComponent(mediaId)}`;
  },
};

/** Public portal verify URL for a fingerprint (for links / QR). */
export function verifyUrl(fingerprint: string): string {
  const origin = typeof window !== "undefined" ? window.location.origin : "";
  return `${origin}/verify/${fingerprint}`;
}

export const shortHash = (h?: string, n = 10) => (h ? `${h.slice(0, n)}…${h.slice(-4)}` : "—");
