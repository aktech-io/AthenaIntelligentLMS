import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft, ShieldCheck, ShieldAlert, Send, Check, X, Banknote, HandCoins, Loader2, ExternalLink, FileText, Link2, Anchor,
} from "lucide-react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useToast } from "@/hooks/use-toast";
import { invoiceService, shortHash, verifyUrl, type InvoiceDetail, type Timeline, type LedgerEvent } from "@/services/invoiceService";
import { formatMoney, formatDate, formatDateTime } from "@/lib/format";
import { cn } from "@/lib/utils";
import { InvoiceStatusPill, TierPill } from "./components/InvoiceStatusPill";
import { LoadingState, ErrorState, EmptyState } from "./components/States";
import { CopyButton } from "./components/CopyButton";
import { PipelineStepper } from "./components/PipelineStepper";
import { ApproveDialog, RejectDialog } from "./components/ReviewDialogs";
import { useInvoiceRole } from "./components/useInvoiceRole";

const LIVE = new Set(["UPLOADED", "EXTRACTED", "BUYER_CONFIRMED", "SCORED", "ACCEPTED"]);

const FIELD_LABELS: Record<string, string> = {
  invoiceNumber: "Invoice number",
  issueDate: "Issue date",
  dueDate: "Due date",
  currency: "Currency",
  subtotal: "Subtotal",
  vat: "VAT",
  total: "Total",
  sellerName: "Seller",
  sellerTaxId: "Seller tax ID",
  buyerName: "Buyer",
  buyerTaxId: "Buyer tax ID",
  buyerEmail: "Buyer email",
  lineItems: "Line items",
};

function fieldValue(inv: InvoiceDetail, key: string): string {
  switch (key) {
    case "invoiceNumber":
      return inv.invoiceNumber ?? "—";
    case "issueDate":
      return formatDate(inv.issueDate);
    case "dueDate":
      return formatDate(inv.dueDate);
    case "currency":
      return inv.currency ?? "—";
    case "subtotal":
      return inv.subtotal != null ? formatMoney(inv.subtotal, inv.currency) : "—";
    case "vat":
      return inv.vat != null ? formatMoney(inv.vat, inv.currency) : "—";
    case "total":
      return formatMoney(inv.total, inv.currency);
    case "sellerName":
      return inv.seller?.name ?? "—";
    case "sellerTaxId":
      return inv.seller?.taxId ?? "—";
    case "buyerName":
      return inv.buyerSnapshot?.name ?? "—";
    case "buyerTaxId":
      return inv.buyerSnapshot?.taxId ?? "—";
    case "buyerEmail":
      return inv.buyerSnapshot?.email ?? "—";
    case "lineItems":
      return `${inv.lineItems?.length ?? 0} lines`;
    default: {
      const v = (inv as unknown as Record<string, unknown>)[key];
      return v == null ? "—" : typeof v === "object" ? JSON.stringify(v) : String(v);
    }
  }
}

function ConfidenceBar({ value }: { value: number }) {
  const pct = Math.round(Math.max(0, Math.min(1, value)) * 100);
  const tone = value >= 0.85 ? "bg-accent" : value >= 0.6 ? "bg-warning" : "bg-destructive";
  return (
    <div className="flex items-center gap-2 w-28">
      <div className="h-1.5 flex-1 rounded-full bg-muted overflow-hidden">
        <div className={cn("h-full rounded-full", tone)} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-[11px] tabular-nums font-mono w-8 text-right">{pct}%</span>
    </div>
  );
}

function KV({ k, v, mono }: { k: string; v: React.ReactNode; mono?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-3 py-1.5 text-sm">
      <span className="text-muted-foreground">{k}</span>
      <span className={cn("text-right tabular-nums", mono && "font-mono text-xs")}>{v}</span>
    </div>
  );
}

const InvoiceDetailPage = () => {
  const { id = "" } = useParams();
  const { isManager, isOfficer } = useInvoiceRole();
  const { toast } = useToast();
  const qc = useQueryClient();

  const q = useQuery<InvoiceDetail>({
    queryKey: ["if-invoice", id],
    queryFn: () => invoiceService.get(id),
    enabled: !!id,
    refetchInterval: (query) => (query.state.data && LIVE.has(query.state.data.status) ? 2000 : false),
  });
  const tl = useQuery<Timeline>({ queryKey: ["if-timeline", id], queryFn: () => invoiceService.timeline(id), enabled: !!id });

  useEffect(() => {
    // refresh timeline whenever status changes
    qc.invalidateQueries({ queryKey: ["if-timeline", id] });
  }, [q.data?.status, id, qc]);

  const [approveOpen, setApproveOpen] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);
  const [payOpen, setPayOpen] = useState(false);
  const [pay, setPay] = useState({ amount: "", reference: "", payer: "" });
  const [pdfUrl, setPdfUrl] = useState<string | null>(null);
  const [pdfErr, setPdfErr] = useState<string | null>(null);
  const [confirmLink, setConfirmLink] = useState<string | null>(null);

  const inv = q.data;

  // Fetch the PDF with the JWT and show it via an object URL (plain <iframe src> can't carry the header).
  useEffect(() => {
    let revoke: string | null = null;
    setPdfUrl(null);
    setPdfErr(null);
    if (!inv?.mediaId) return;
    const token = localStorage.getItem("athena_jwt");
    fetch(invoiceService.mediaDownloadUrl(inv.mediaId), { headers: token ? { Authorization: `Bearer ${token}` } : {} })
      .then(async (r) => {
        if (!r.ok) throw new Error(`media ${r.status}`);
        const blob = await r.blob();
        revoke = URL.createObjectURL(blob);
        setPdfUrl(revoke);
      })
      .catch((e: Error) => setPdfErr(e.message));
    return () => {
      if (revoke) URL.revokeObjectURL(revoke);
    };
  }, [inv?.mediaId]);

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["if-invoice", id] });
    qc.invalidateQueries({ queryKey: ["if-timeline", id] });
    qc.invalidateQueries({ queryKey: ["if-invoices"] });
    qc.invalidateQueries({ queryKey: ["if-queue"] });
    qc.invalidateQueries({ queryKey: ["if-stats"] });
  };
  const onErr = (title: string) => (e: Error) => toast({ title, description: e.message, variant: "destructive" });

  const resend = useMutation({
    mutationFn: () => invoiceService.resendConfirmation(id),
    onSuccess: (r) => {
      toast({ title: "Confirmation re-sent" });
      if (r?.confirmLink) setConfirmLink(r.confirmLink);
      refresh();
    },
    onError: onErr("Resend failed"),
  });
  const accept = useMutation({
    mutationFn: () => invoiceService.acceptOffer(id),
    onSuccess: () => {
      toast({ title: "Offer accepted", description: "Funding will follow automatically unless a second approval is required." });
      refresh();
    },
    onError: onErr("Accept failed"),
  });
  const approve = useMutation({
    mutationFn: (note: string) => invoiceService.approve(id, note),
    onSuccess: () => {
      toast({ title: "Approved" });
      setApproveOpen(false);
      refresh();
    },
    onError: onErr("Approve failed"),
  });
  const reject = useMutation({
    mutationFn: (reason: string) => invoiceService.reject(id, reason),
    onSuccess: () => {
      toast({ title: "Rejected" });
      setRejectOpen(false);
      refresh();
    },
    onError: onErr("Reject failed"),
  });
  const fund = useMutation({
    mutationFn: () => invoiceService.fund(id),
    onSuccess: () => {
      toast({ title: "Funding triggered", description: "The core-banking adapter is disbursing the advance." });
      refresh();
    },
    onError: onErr("Funding failed"),
  });
  const payment = useMutation({
    mutationFn: () => invoiceService.recordPayment(id, { amount: Number(pay.amount), reference: pay.reference.trim(), payer: pay.payer.trim() || undefined }),
    onSuccess: () => {
      toast({ title: "Payment recorded" });
      setPayOpen(false);
      setPay({ amount: "", reference: "", payer: "" });
      refresh();
    },
    onError: onErr("Payment failed"),
  });

  const title = inv?.invoiceNumber ? `Invoice #${inv.invoiceNumber}` : "Invoice";

  if (q.isLoading) {
    return (
      <DashboardLayout title="Invoice" breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance", href: "/invoice-finance" }, { label: "Invoices", href: "/invoice-finance/invoices" }, { label: "…" }]}>
        <LoadingState label="Loading invoice…" />
      </DashboardLayout>
    );
  }
  if (q.isError || !inv) {
    return (
      <DashboardLayout title="Invoice" breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance", href: "/invoice-finance" }, { label: "Invoices", href: "/invoice-finance/invoices" }]}>
        <ErrorState error={q.error ?? new Error("Invoice not found")} onRetry={() => q.refetch()} />
      </DashboardLayout>
    );
  }

  const status = inv.status;
  const conf = inv.ocr?.confidence ?? {};
  const fieldKeys = Array.from(new Set([...Object.keys(FIELD_LABELS).filter((k) => k in conf || ["invoiceNumber", "issueDate", "dueDate", "total", "buyerName"].includes(k)), ...Object.keys(conf)]));
  const needsSeller = new Set(inv.needsSellerConfirmation ?? Object.entries(conf).filter(([, c]) => c < 0.6).map(([k]) => k));
  const outstanding = inv.advance ? Math.max(0, inv.advance.principal + inv.advance.feesAccrued - inv.advance.repaid) : 0;
  const events: LedgerEvent[] = tl.data?.events ?? [];
  const anchored = events.some((e) => e.anchorId);

  const canResend = status === "AWAITING_BUYER";
  const canAccept = status === "OFFERED" && !!inv.offer && inv.offer.status === "OFFERED";
  const canReview = isManager && (status === "DECIDED" || (status === "ACCEPTED" && (inv.decision?.outcome === "REFER" || (inv.offer?.advanceAmount ?? 0) >= 500_000)));
  const canFund = isOfficer && status === "ACCEPTED";
  const canPay = isOfficer && ["FUNDED", "PARTIALLY_PAID", "OVERDUE"].includes(status);

  return (
    <DashboardLayout
      title={title}
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance", href: "/invoice-finance" }, { label: "Invoices", href: "/invoice-finance/invoices" }, { label: inv.invoiceNumber ?? inv.id.slice(0, 8) }]}
    >
      <div className="space-y-5 animate-fade-in">
        {/* Header */}
        <div className="flex flex-col lg:flex-row lg:items-start justify-between gap-4">
          <div className="space-y-2 min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Link to="/invoice-finance/invoices" className="text-muted-foreground hover:text-foreground">
                <ArrowLeft className="h-4 w-4" />
              </Link>
              <InvoiceStatusPill status={status} />
              {inv.statusReason && <span className="text-xs text-muted-foreground font-mono">{inv.statusReason}</span>}
              <span className="text-sm">
                {inv.seller?.name ?? inv.sellerCustomerId} <span className="text-muted-foreground">→</span> {inv.buyerSnapshot?.name ?? "buyer tbc"}
              </span>
              <span className="font-mono tabular-nums text-sm font-semibold">{formatMoney(inv.total, inv.currency)}</span>
            </div>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-1">
                <FileText className="h-3.5 w-3.5" /> fingerprint
                <code className="font-mono text-foreground">{shortHash(inv.fingerprint, 12)}</code>
                {inv.fingerprint && <CopyButton value={inv.fingerprint} />}
              </span>
              {inv.fingerprint && (
                <a href={verifyUrl(inv.fingerprint)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-primary hover:underline">
                  <Link2 className="h-3.5 w-3.5" /> public verify <ExternalLink className="h-3 w-3" />
                </a>
              )}
              <span className="font-mono">id {inv.id}</span>
              <span>created {formatDateTime(inv.createdAt)}</span>
            </div>
          </div>

          {/* Actions */}
          <div className="flex flex-wrap gap-2 shrink-0">
            {canResend && (
              <Button size="sm" variant="outline" onClick={() => resend.mutate()} disabled={resend.isPending}>
                {resend.isPending ? <Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" /> : <Send className="h-3.5 w-3.5 mr-1" />} Resend confirmation
              </Button>
            )}
            {canAccept && (
              <Button size="sm" onClick={() => accept.mutate()} disabled={accept.isPending}>
                {accept.isPending ? <Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" /> : <Check className="h-3.5 w-3.5 mr-1" />} Accept offer
              </Button>
            )}
            {canReview && (
              <>
                <Button size="sm" variant="outline" className="text-destructive border-destructive/30 hover:bg-destructive/10" onClick={() => setRejectOpen(true)}>
                  <X className="h-3.5 w-3.5 mr-1" /> Reject
                </Button>
                <Button size="sm" onClick={() => setApproveOpen(true)}>
                  <Check className="h-3.5 w-3.5 mr-1" /> Approve
                </Button>
              </>
            )}
            {canFund && (
              <Button size="sm" onClick={() => fund.mutate()} disabled={fund.isPending}>
                {fund.isPending ? <Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" /> : <Banknote className="h-3.5 w-3.5 mr-1" />} Fund
              </Button>
            )}
            {canPay && (
              <Button size="sm" variant="outline" onClick={() => setPayOpen(true)}>
                <HandCoins className="h-3.5 w-3.5 mr-1" /> Record payment
              </Button>
            )}
          </div>
        </div>

        {confirmLink && (
          <div className="rounded-md border border-warning/30 bg-warning/5 p-3 text-xs flex items-center gap-2">
            <span className="uppercase tracking-wide text-warning font-semibold shrink-0">Demo confirm link</span>
            <code className="font-mono truncate flex-1">{confirmLink}</code>
            <CopyButton value={confirmLink} />
          </div>
        )}

        <div className="grid grid-cols-1 xl:grid-cols-5 gap-4">
          {/* Left: extracted fields + PDF */}
          <div className="xl:col-span-3 space-y-4">
            <Card>
              <CardHeader className="pb-2 flex flex-row items-center justify-between">
                <CardTitle className="text-sm font-heading">Extracted fields</CardTitle>
                {inv.ocr?.engine && (
                  <span className="text-[11px] text-muted-foreground font-mono">
                    {inv.ocr.engine}
                    {inv.ocr.rawWordCount ? ` · ${inv.ocr.rawWordCount} words` : ""}
                  </span>
                )}
              </CardHeader>
              <CardContent>
                {status === "UPLOADED" ? (
                  <EmptyState icon={FileText} title="Extracting…" description="OCR is running on the uploaded document." />
                ) : (
                  <div className="divide-y">
                    {fieldKeys.map((k) => (
                      <div key={k} className="grid grid-cols-[1fr_auto] sm:grid-cols-[160px_1fr_auto] items-center gap-3 py-2 text-sm">
                        <span className="text-muted-foreground">{FIELD_LABELS[k] ?? k}</span>
                        <span className={cn("tabular-nums truncate col-span-2 sm:col-span-1", needsSeller.has(k) && "text-warning")}>
                          {fieldValue(inv, k)}
                          {needsSeller.has(k) && (
                            <Badge variant="outline" className="ml-2 text-[11px] bg-warning/10 text-warning border-warning/30">
                              seller to confirm
                            </Badge>
                          )}
                        </span>
                        <div className="justify-self-end">{k in conf ? <ConfidenceBar value={conf[k]} /> : <span className="text-[11px] text-muted-foreground">—</span>}</div>
                      </div>
                    ))}
                  </div>
                )}
                {inv.lineItems && inv.lineItems.length > 0 && (
                  <div className="mt-4 overflow-x-auto">
                    <table className="w-full text-xs">
                      <thead className="text-muted-foreground">
                        <tr className="border-b">
                          <th className="text-left font-medium py-1.5">Description</th>
                          <th className="text-right font-medium py-1.5">Qty</th>
                          <th className="text-right font-medium py-1.5">Unit</th>
                          <th className="text-right font-medium py-1.5">Amount</th>
                        </tr>
                      </thead>
                      <tbody className="tabular-nums font-mono">
                        {inv.lineItems.map((li, i) => (
                          <tr key={i} className="border-b last:border-0">
                            <td className="py-1.5 font-sans">{li.description}</td>
                            <td className="text-right py-1.5">{li.qty}</td>
                            <td className="text-right py-1.5">{formatMoney(li.unitPrice, inv.currency, { decimals: 2 })}</td>
                            <td className="text-right py-1.5">{formatMoney(li.amount, inv.currency, { decimals: 2 })}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2 flex flex-row items-center justify-between">
                <CardTitle className="text-sm font-heading">Document</CardTitle>
                {pdfUrl && (
                  <a href={pdfUrl} download={`${inv.invoiceNumber ?? inv.id}.pdf`} className="text-xs text-primary hover:underline">
                    Download
                  </a>
                )}
              </CardHeader>
              <CardContent>
                {!inv.mediaId ? (
                  <EmptyState icon={FileText} title="No document stored" />
                ) : pdfErr ? (
                  <EmptyState icon={FileText} title="Preview unavailable" description={`media-service: ${pdfErr}`} />
                ) : !pdfUrl ? (
                  <LoadingState label="Fetching PDF…" />
                ) : (
                  <iframe title="Invoice PDF" src={pdfUrl} className="w-full h-[520px] rounded-md border bg-white" />
                )}
              </CardContent>
            </Card>
          </div>

          {/* Right: decision / offer / advance / buyer */}
          <div className="xl:col-span-2 space-y-4">
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-heading">Pipeline</CardTitle>
              </CardHeader>
              <CardContent>
                <PipelineStepper status={status} statusReason={inv.statusReason} polling={LIVE.has(status)} />
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2 flex flex-row items-center justify-between">
                <CardTitle className="text-sm font-heading">Decision</CardTitle>
                {inv.decision && <InvoiceStatusPill status={inv.decision.outcome} />}
              </CardHeader>
              <CardContent>
                {!inv.decision ? (
                  <p className="text-xs text-muted-foreground">Not yet decided — runs after buyer confirmation, scoring and fraud checks.</p>
                ) : (
                  <div className="space-y-2">
                    <div className="flex flex-wrap gap-1">
                      {(inv.decision.reasons ?? []).map((r) => (
                        <Badge key={r} variant="outline" className="font-mono text-[11px]">
                          {r}
                        </Badge>
                      ))}
                      {inv.decision.ruleId && (
                        <Badge variant="outline" className="font-mono text-[11px] text-muted-foreground">
                          {inv.decision.ruleId}
                        </Badge>
                      )}
                      {(inv.decision.fraudFlags ?? []).map((f) => (
                        <Badge key={f} variant="outline" className="font-mono text-[11px] bg-destructive/10 text-destructive border-destructive/30">
                          <ShieldAlert className="h-3 w-3 mr-1" /> {f}
                        </Badge>
                      ))}
                    </div>
                    {inv.decision.detail && (
                      <div className="divide-y">
                        {inv.decision.detail.advanceRate != null && <KV k="Advance rate" v={`${Math.round(Number(inv.decision.detail.advanceRate) * 100)}%`} />}
                        {inv.decision.detail.feePerDay != null && <KV k="Fee / day" v={`${(Number(inv.decision.detail.feePerDay) * 100).toFixed(3)}%`} />}
                        {inv.decision.detail.maxAmount != null && <KV k="Max amount" v={formatMoney(Number(inv.decision.detail.maxAmount), inv.currency)} />}
                      </div>
                    )}
                    {inv.decision.inputs && (
                      <details className="text-xs">
                        <summary className="cursor-pointer text-muted-foreground">Inputs</summary>
                        <pre className="mt-1 rounded bg-muted p-2 font-mono text-[11px] overflow-x-auto">{JSON.stringify(inv.decision.inputs, null, 2)}</pre>
                      </details>
                    )}
                    <div className="text-[11px] text-muted-foreground flex flex-wrap items-center gap-x-2 pt-1">
                      <span>
                        policy {inv.decision.policyId ?? "invoice.advance"}
                        {inv.decision.policyVersion != null && ` v${inv.decision.policyVersion}`}
                      </span>
                      {inv.decision.policyHash && (
                        <span className="inline-flex items-center font-mono">
                          {shortHash(inv.decision.policyHash)}
                          <CopyButton value={inv.decision.policyHash} />
                        </span>
                      )}
                      {inv.decision.decidedAt && <span>{formatDateTime(inv.decision.decidedAt)}</span>}
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2 flex flex-row items-center justify-between">
                <CardTitle className="text-sm font-heading">Offer</CardTitle>
                {inv.offer && <InvoiceStatusPill status={inv.offer.status} />}
              </CardHeader>
              <CardContent>
                {!inv.offer ? (
                  <p className="text-xs text-muted-foreground">No offer yet.</p>
                ) : (
                  <div className="divide-y">
                    <KV k="Advance" v={`${formatMoney(inv.offer.advanceAmount, inv.currency)} (${Math.round(inv.offer.advanceRate * 100)}%)`} />
                    <KV k="Tenor" v={`${inv.offer.tenorDays} days`} />
                    <KV k="Discount fee" v={`${formatMoney(inv.offer.feeAmount, inv.currency)} · ${(inv.offer.discountFeePerDay * 100).toFixed(3)}%/day`} />
                    <KV k="Processing fee" v={formatMoney(inv.offer.processingFee, inv.currency)} />
                    <KV k="Net disbursement" v={<span className="font-semibold">{formatMoney(inv.offer.netDisbursement, inv.currency)}</span>} />
                    <KV k="Reserve" v={formatMoney(inv.offer.reserveAmount, inv.currency)} />
                    {inv.offer.expiresAt && <KV k="Expires" v={formatDateTime(inv.offer.expiresAt)} />}
                    {canAccept && (
                      <div className="pt-3">
                        <Button className="w-full" size="sm" onClick={() => accept.mutate()} disabled={accept.isPending}>
                          {accept.isPending ? <Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" /> : <Check className="h-3.5 w-3.5 mr-1" />} Accept on behalf of seller
                        </Button>
                      </div>
                    )}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2 flex flex-row items-center justify-between">
                <CardTitle className="text-sm font-heading">Advance</CardTitle>
                {inv.advance && <InvoiceStatusPill status={inv.advance.status} />}
              </CardHeader>
              <CardContent>
                {!inv.advance ? (
                  <p className="text-xs text-muted-foreground">Not funded.</p>
                ) : (
                  <div className="divide-y">
                    <KV k="Principal" v={formatMoney(inv.advance.principal, inv.currency)} />
                    <KV k="Fees accrued" v={formatMoney(inv.advance.feesAccrued, inv.currency)} />
                    <KV k="Repaid" v={formatMoney(inv.advance.repaid, inv.currency)} />
                    <KV k="Outstanding" v={<span className={cn("font-semibold", outstanding > 0 && status === "OVERDUE" && "text-destructive")}>{formatMoney(outstanding, inv.currency)}</span>} />
                    <KV k="Reserve released" v={formatMoney(inv.advance.reserveReleased, inv.currency)} />
                    <KV k="Disbursed" v={formatDateTime(inv.advance.disbursedAt)} />
                    <KV k="Due" v={formatDate(inv.advance.dueDate)} />
                    {inv.advance.coreRef && (
                      <KV
                        k="Core loan"
                        v={
                          <Link to={`/loan/${inv.advance.coreRef}`} className="text-primary hover:underline inline-flex items-center gap-1">
                            {inv.advance.coreRef.slice(0, 12)} <ExternalLink className="h-3 w-3" />
                          </Link>
                        }
                        mono
                      />
                    )}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2 flex flex-row items-center justify-between">
                <CardTitle className="text-sm font-heading">Buyer</CardTitle>
                {inv.buyer && <TierPill tier={inv.buyer.tier} />}
              </CardHeader>
              <CardContent>
                <div className="divide-y">
                  <KV k="Name" v={inv.buyer?.name ?? inv.buyerSnapshot?.name ?? "—"} />
                  <KV k="Tax ID" v={inv.buyer?.taxId ?? inv.buyerSnapshot?.taxId ?? "—"} mono />
                  <KV k="Email" v={inv.buyerSnapshot?.email ?? "—"} />
                  {inv.buyerSnapshot?.phone && <KV k="Phone" v={inv.buyerSnapshot.phone} mono />}
                  {inv.buyer && (
                    <>
                      <KV k="Exposure / limit" v={`${formatMoney(inv.buyer.exposureAmount ?? 0, inv.currency)}${inv.buyer.limitAmount ? ` / ${formatMoney(inv.buyer.limitAmount, inv.currency)}` : ""}`} />
                      <KV k="Confirmations" v={inv.buyer.confirmationsCount ?? 0} />
                      {inv.buyer.onTimeRate != null && <KV k="On-time rate" v={`${Math.round(inv.buyer.onTimeRate * (inv.buyer.onTimeRate <= 1 ? 100 : 1))}%`} />}
                    </>
                  )}
                </div>
                {inv.buyer && (
                  <Link to="/invoice-finance/buyers" className="mt-2 inline-block text-xs text-primary hover:underline">
                    Manage buyer
                  </Link>
                )}
              </CardContent>
            </Card>
          </div>
        </div>

        {/* Timeline */}
        <Card>
          <CardHeader className="pb-2 flex flex-row items-center justify-between">
            <CardTitle className="text-sm font-heading">Ledger timeline</CardTitle>
            <div className="flex items-center gap-2">
              {anchored && (
                <Badge variant="outline" className="text-[11px] bg-accent/10 text-accent border-accent/30">
                  <Anchor className="h-3 w-3 mr-1" /> anchored
                </Badge>
              )}
              {tl.data && (
                <Badge variant="outline" className={cn("text-[11px]", tl.data.chainOk ? "bg-accent/15 text-accent border-accent/40" : "bg-destructive/15 text-destructive border-destructive/40")}>
                  {tl.data.chainOk ? <ShieldCheck className="h-3 w-3 mr-1" /> : <ShieldAlert className="h-3 w-3 mr-1" />}
                  {tl.data.chainOk ? "chain OK" : "chain BROKEN"}
                </Badge>
              )}
            </div>
          </CardHeader>
          <CardContent>
            {tl.isLoading ? (
              <LoadingState label="Verifying chain…" />
            ) : tl.isError ? (
              <ErrorState error={tl.error} onRetry={() => tl.refetch()} />
            ) : events.length === 0 ? (
              <EmptyState title="No events yet" />
            ) : (
              <Tabs defaultValue="list">
                <TabsList className="h-8">
                  <TabsTrigger value="list" className="text-xs">
                    Events
                  </TabsTrigger>
                  <TabsTrigger value="raw" className="text-xs">
                    Raw
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="list">
                  <ol className="relative mt-2">
                    {events.map((e, i) => (
                      <li key={e.seq ?? i} className="relative flex gap-3 pb-4 last:pb-0">
                        {i < events.length - 1 && <span className="absolute left-[5px] top-3 bottom-0 w-px bg-border" aria-hidden />}
                        <span className={cn("relative z-10 mt-1.5 h-3 w-3 rounded-full border-2 bg-card shrink-0", /declin|dispute|reject|overdue|written/.test(e.type) ? "border-destructive" : /funded|paid|confirmed|approved|accepted/.test(e.type) ? "border-accent" : "border-primary")} />
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5">
                            <span className="text-sm font-medium font-mono">{e.type}</span>
                            <span className="text-xs text-muted-foreground">{formatDateTime(e.occurredAt)}</span>
                            {e.actor && <span className="text-xs text-muted-foreground">by {e.actor}</span>}
                            <span className="text-[11px] text-muted-foreground font-mono">#{e.seq}</span>
                          </div>
                          {e.payload && Object.keys(e.payload).length > 0 && (
                            <p className="text-xs text-muted-foreground mt-0.5 truncate">
                              {Object.entries(e.payload)
                                .slice(0, 5)
                                .map(([k, v]) => `${k}=${typeof v === "object" ? JSON.stringify(v) : String(v)}`)
                                .join("  ")}
                            </p>
                          )}
                          <div className="text-[11px] text-muted-foreground font-mono mt-0.5 flex items-center gap-2">
                            <span>hash {shortHash(e.hash, 8)}</span>
                            <span>prev {shortHash(e.prevHash, 6)}</span>
                            {e.anchorId && (
                              <span className="inline-flex items-center gap-0.5 text-accent">
                                <Anchor className="h-2.5 w-2.5" /> {String(e.anchorId).slice(0, 8)}
                              </span>
                            )}
                          </div>
                        </div>
                      </li>
                    ))}
                  </ol>
                </TabsContent>
                <TabsContent value="raw">
                  <pre className="mt-2 rounded bg-muted p-3 font-mono text-[11px] overflow-x-auto max-h-96">{JSON.stringify(tl.data, null, 2)}</pre>
                </TabsContent>
              </Tabs>
            )}
          </CardContent>
        </Card>
      </div>

      <ApproveDialog open={approveOpen} onOpenChange={setApproveOpen} title={`Approve ${title}`} pending={approve.isPending} onConfirm={(n) => approve.mutate(n)} />
      <RejectDialog open={rejectOpen} onOpenChange={setRejectOpen} title={`Reject ${title}`} pending={reject.isPending} onConfirm={(r) => reject.mutate(r)} />

      <Dialog open={payOpen} onOpenChange={setPayOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="font-heading">Record buyer payment</DialogTitle>
            <DialogDescription>
              Applied to fees first, then principal; any surplus releases the reserve to the seller.
              {inv.advance && <> Outstanding: <span className="font-mono tabular-nums">{formatMoney(outstanding, inv.currency)}</span>.</>}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="pay-amount">Amount ({inv.currency})</Label>
              <Input id="pay-amount" type="number" min={0} step="0.01" value={pay.amount} onChange={(e) => setPay({ ...pay, amount: e.target.value })} className="font-mono" placeholder={outstanding ? String(outstanding) : "0.00"} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="pay-ref">Reference</Label>
              <Input id="pay-ref" value={pay.reference} onChange={(e) => setPay({ ...pay, reference: e.target.value })} className="font-mono" placeholder="Bank / M-Pesa reference" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="pay-payer">Payer (optional)</Label>
              <Input id="pay-payer" value={pay.payer} onChange={(e) => setPay({ ...pay, payer: e.target.value })} placeholder={inv.buyerSnapshot?.name ?? "Buyer"} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPayOpen(false)} disabled={payment.isPending}>
              Cancel
            </Button>
            <Button onClick={() => payment.mutate()} disabled={payment.isPending || !(Number(pay.amount) > 0) || pay.reference.trim().length < 2}>
              {payment.isPending && <Loader2 className="h-4 w-4 mr-1 animate-spin" />} Record
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default InvoiceDetailPage;
