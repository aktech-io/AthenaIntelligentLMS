import type { InvoiceStatus } from "@/services/invoiceService";

/** Plain-language status copy for sellers (admin pills keep the raw enum). */
export const STATUS_COPY: Record<string, { label: string; hint: string }> = {
  UPLOADED: { label: "Uploaded", hint: "We're reading your invoice." },
  EXTRACTED: { label: "Read", hint: "Details extracted — asking your buyer to confirm." },
  AWAITING_BUYER: { label: "Waiting for buyer", hint: "Your buyer has been emailed a confirmation link." },
  BUYER_CONFIRMED: { label: "Buyer confirmed", hint: "Scoring the invoice now." },
  BUYER_DISPUTED: { label: "Buyer disputed", hint: "Your buyer did not confirm this invoice." },
  SCORED: { label: "Scored", hint: "Running the final checks." },
  DECIDED: { label: "Under review", hint: "A credit officer is reviewing this invoice." },
  OFFERED: { label: "Offer ready", hint: "Review the terms and accept to get paid." },
  ACCEPTED: { label: "Accepted", hint: "Funds are on the way." },
  FUNDED: { label: "Funded", hint: "Advance paid to you. Buyer pays on the due date." },
  PARTIALLY_PAID: { label: "Partly paid", hint: "Buyer has paid part of the invoice." },
  PAID: { label: "Settled", hint: "Buyer paid in full. Your reserve has been released." },
  OVERDUE: { label: "Overdue", hint: "Buyer payment is late." },
  DECLINED: { label: "Declined", hint: "We couldn't finance this invoice." },
  CANCELLED: { label: "Cancelled", hint: "You cancelled this request." },
};

export const statusLabel = (s?: string | null) => STATUS_COPY[(s ?? "").toUpperCase()]?.label ?? (s ?? "—").replace(/_/g, " ");
export const statusHint = (s?: string | null) => STATUS_COPY[(s ?? "").toUpperCase()]?.hint ?? "";

/** Seller-facing filter groups for the invoice list. */
export const SELLER_FILTERS: { key: string; label: string; statuses: InvoiceStatus[] }[] = [
  { key: "all", label: "All", statuses: [] },
  { key: "progress", label: "In progress", statuses: ["UPLOADED", "EXTRACTED", "AWAITING_BUYER", "BUYER_CONFIRMED", "SCORED", "DECIDED", "ACCEPTED"] },
  { key: "offer", label: "Offer ready", statuses: ["OFFERED"] },
  { key: "funded", label: "Funded", statuses: ["FUNDED", "PARTIALLY_PAID", "OVERDUE"] },
  { key: "settled", label: "Settled", statuses: ["PAID"] },
  { key: "closed", label: "Closed", statuses: ["DECLINED", "CANCELLED", "BUYER_DISPUTED"] },
];

/** Statuses while the pipeline is still moving (poll). */
export const LIVE_STATUSES = new Set<string>(["UPLOADED", "EXTRACTED", "BUYER_CONFIRMED", "SCORED", "ACCEPTED"]);

/** A seller may cancel until money moves. */
export const CANCELLABLE = new Set<string>(["UPLOADED", "EXTRACTED", "AWAITING_BUYER", "BUYER_CONFIRMED", "SCORED", "DECIDED", "OFFERED"]);

export const FIELD_LABELS: Record<string, string> = {
  invoiceNumber: "Invoice number",
  issueDate: "Issue date",
  dueDate: "Due date",
  currency: "Currency",
  subtotal: "Subtotal",
  vat: "VAT",
  total: "Total",
  sellerName: "Your business",
  sellerTaxId: "Your tax ID",
  buyerName: "Buyer",
  buyerTaxId: "Buyer tax ID",
  buyerEmail: "Buyer email",
  lineItems: "Line items",
};

export const daysUntil = (iso?: string | null): number | null => {
  if (!iso) return null;
  const d = new Date(iso).getTime();
  if (Number.isNaN(d)) return null;
  return Math.round((d - Date.now()) / 86_400_000);
};
