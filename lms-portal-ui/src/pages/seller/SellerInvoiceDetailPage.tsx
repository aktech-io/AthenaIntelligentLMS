import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Loader2, ShieldCheck, ShieldAlert, Anchor, ExternalLink, FileText, Download, Ban, Send, ExternalLink as LinkIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { invoiceService, isSellerProfileMissing, shortHash, verifyUrl, type InvoiceDetail, type Timeline, type LedgerEvent, type SellerApiError } from "@/services/invoiceService";
import { formatMoney, formatDate, formatDateTime } from "@/lib/format";
import { appConfig } from "@/lib/appConfig";
import { cn } from "@/lib/utils";
import { PipelineStepper } from "@/pages/invoice-finance/components/PipelineStepper";
import { CopyButton } from "@/pages/invoice-finance/components/CopyButton";
import { LoadingState, ErrorState, EmptyState } from "@/pages/invoice-finance/components/States";
import { SellerShell } from "./components/SellerShell";
import { SellerOnboardingPage } from "./SellerOnboardingPage";
import { SellerStatusPill } from "./components/SellerStatusPill";
import { ExtractedFields } from "./components/ExtractedFields";
import { QrCode } from "./components/QrCode";
import { statusHint, LIVE_STATUSES, CANCELLABLE } from "./components/sellerUtils";

/** Friendly ledger event names. */
function eventLabel(type: string): string {
  const t = type.toLowerCase();
  if (/upload/.test(t)) return "Invoice uploaded";
  if (/extract/.test(t)) return "Details read";
  if (/confirmation.*(sent|request)/.test(t)) return "Buyer asked to confirm";
  if (/dispute/.test(t)) return "Buyer disputed";
  if (/confirm/.test(t)) return "Buyer confirmed";
  if (/scor/.test(t)) return "Scored";
  if (/fraud/.test(t)) return "Fraud checks";
  if (/decid|decision/.test(t)) return "Decision made";
  if (/offer.*accept/.test(t)) return "You accepted the offer";
  if (/offer/.test(t)) return "Offer made";
  if (/fund|disburs/.test(t)) return "Advance paid to you";
  if (/payment|paid/.test(t)) return "Buyer payment received";
  if (/reserve/.test(t)) return "Reserve released";
  if (/cancel/.test(t)) return "Cancelled";
  if (/declin|reject/.test(t)) return "Declined";
  if (/anchor/.test(t)) return "Anchored to public chain";
  return type.replace(/[._]/g, " ");
}

const SellerInvoiceDetailPage = () => {
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const { toast } = useToast();
  const [pdfUrl, setPdfUrl] = useState<string | null>(null);
  const [pdfErr, setPdfErr] = useState<string | null>(null);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [cancelReason, setCancelReason] = useState("");
  const [acceptOpen, setAcceptOpen] = useState(false);
  const [confirmLink, setConfirmLink] = useState<string | null>(null);

  const q = useQuery<InvoiceDetail>({
    queryKey: ["seller-invoice", id],
    queryFn: () => invoiceService.getMine(id),
    enabled: !!id,
    refetchInterval: (query) => (query.state.data && LIVE_STATUSES.has(query.state.data.status) ? 2000 : false),
    retry: (n, e) => !isSellerProfileMissing(e) && (e as SellerApiError)?.status !== 404 && n < 2,
  });
  const tl = useQuery<Timeline>({ queryKey: ["seller-timeline", id], queryFn: () => invoiceService.timeline(id), enabled: !!id && !!q.data });

  useEffect(() => {
    qc.invalidateQueries({ queryKey: ["seller-timeline", id] });
  }, [q.data?.status, id, qc]);

  const inv = q.data;

  // PDF preview: GET /invoices/{id}/document with JWT → blob URL (iframe can't carry the header).
  useEffect(() => {
    let url: string | null = null;
    setPdfUrl(null);
    setPdfErr(null);
    if (!inv?.id) return;
    invoiceService
      .documentBlob(inv.id)
      .then((blob) => {
        url = URL.createObjectURL(blob);
        setPdfUrl(url);
      })
      .catch((e: Error) => setPdfErr(e.message));
    return () => {
      if (url) URL.revokeObjectURL(url);
    };
  }, [inv?.id]);

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["seller-invoice", id] });
    qc.invalidateQueries({ queryKey: ["seller-timeline", id] });
    qc.invalidateQueries({ queryKey: ["seller-invoices"] });
    qc.invalidateQueries({ queryKey: ["seller-summary"] });
  };
  const onErr = (title: string) => (e: Error) => toast({ title, description: e.message, variant: "destructive" });

  const accept = useMutation({
    mutationFn: () => invoiceService.acceptOfferMine(id),
    onSuccess: () => {
      setAcceptOpen(false);
      toast({ title: "Offer accepted", description: "Your advance is being paid out now." });
      refresh();
    },
    onError: onErr("Couldn't accept the offer"),
  });
  const cancel = useMutation({
    mutationFn: () => invoiceService.cancel(id, cancelReason),
    onSuccess: () => {
      setCancelOpen(false);
      toast({ title: "Request cancelled" });
      refresh();
    },
    onError: onErr("Couldn't cancel"),
  });
  const resend = useMutation({
    mutationFn: () => invoiceService.resendConfirmation(id),
    onSuccess: (r) => {
      toast({ title: "Reminder sent to your buyer" });
      if (r?.confirmLink) setConfirmLink(r.confirmLink);
    },
    onError: onErr("Couldn't resend"),
  });

  if (isSellerProfileMissing(q.error)) return <SellerOnboardingPage onRetry={() => q.refetch()} retrying={q.isFetching} />;

  if (q.isLoading) {
    return (
      <SellerShell title="Invoice" back="/invoices" wide>
        <LoadingState label="Loading invoice…" />
      </SellerShell>
    );
  }
  if (q.isError || !inv) {
    return (
      <SellerShell title="Invoice" back="/invoices" wide>
        <ErrorState error={q.error ?? new Error("Invoice not found")} onRetry={() => q.refetch()} />
      </SellerShell>
    );
  }

  const status = inv.status;
  const offer = inv.offer;
  const adv = inv.advance;
  const canAccept = status === "OFFERED" && !!offer && offer.status === "OFFERED";
  const canCancel = CANCELLABLE.has(status);
  const canResend = status === "AWAITING_BUYER";
  const events: LedgerEvent[] = tl.data?.events ?? [];
  const anchored = events.some((e) => e.anchorId);
  const verify = inv.fingerprint ? verifyUrl(inv.fingerprint) : null;
  const dueDate = adv?.dueDate ?? inv.dueDate;
  const outstanding = adv ? Math.max(0, adv.principal + adv.feesAccrued - adv.repaid) : 0;

  return (
    <SellerShell title={inv.invoiceNumber ? `Invoice #${inv.invoiceNumber}` : "Invoice"} subtitle={statusHint(status)} back="/invoices" wide>
      <div className="space-y-4 animate-fade-in">
        {/* Summary strip */}
        <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="eyebrow">{inv.buyerSnapshot?.name ?? inv.buyer?.name ?? "Buyer to confirm"}</p>
              <p className="num-display mt-1">{formatMoney(inv.total, inv.currency)}</p>
              <p className="text-sm text-muted-foreground mt-1">
                {inv.issueDate && <>Issued {formatDate(inv.issueDate)} · </>}
                {inv.dueDate && <>Buyer pays by {formatDate(inv.dueDate)}</>}
              </p>
            </div>
            <div className="flex flex-col items-end gap-2">
              <SellerStatusPill status={status} />
              {inv.statusReason && <span className="text-[11px] text-muted-foreground font-mono">{inv.statusReason.replace(/_/g, " ")}</span>}
            </div>
          </div>
          <div className="flex flex-wrap gap-2 mt-4">
            {canAccept && (
              <Button className="h-11 flex-1 sm:flex-none" onClick={() => setAcceptOpen(true)}>
                <Check className="h-4 w-4" /> Accept offer
              </Button>
            )}
            {canResend && (
              <Button variant="outline" className="h-11" onClick={() => resend.mutate()} disabled={resend.isPending}>
                {resend.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />} Remind buyer
              </Button>
            )}
            {canCancel && (
              <Button variant="ghost" className="h-11 text-muted-foreground hover:text-destructive" onClick={() => setCancelOpen(true)}>
                <Ban className="h-4 w-4" /> Cancel request
              </Button>
            )}
          </div>
          {confirmLink && appConfig.demo && (
            <div className="mt-3 rounded-md border border-warning/30 bg-warning/5 p-2 text-xs flex items-center gap-2">
              <span className="uppercase tracking-wide text-warning font-semibold shrink-0">Demo link</span>
              <code className="font-mono truncate flex-1">{confirmLink}</code>
              <CopyButton value={confirmLink} />
            </div>
          )}
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-5 gap-4">
          <div className="lg:col-span-3 space-y-4">
            {/* Offer card */}
            {offer && (
              <div data-nemo-card="" className={cn("rounded-xl border bg-card p-4 sm:p-5", canAccept && "border-primary/50 shadow-glow")}>
                <div className="flex items-center justify-between">
                  <h2 className="font-heading text-base font-semibold">{canAccept ? "Your offer" : "Offer terms"}</h2>
                  <Badge variant="outline" className="text-[11px] font-semibold">
                    {offer.status}
                  </Badge>
                </div>
                <div className="mt-3 grid grid-cols-1 sm:grid-cols-3 gap-3">
                  <div className="rounded-lg bg-primary/10 p-3">
                    <p className="text-[11px] uppercase tracking-wider text-primary font-semibold">You receive now</p>
                    <p className="font-heading text-xl font-semibold tabular-nums mt-1">{formatMoney(offer.netDisbursement, inv.currency)}</p>
                  </div>
                  <div className="rounded-lg bg-muted p-3">
                    <p className="text-[11px] uppercase tracking-wider text-muted-foreground font-semibold">Buyer pays</p>
                    <p className="font-heading text-xl font-semibold tabular-nums mt-1">{formatMoney(inv.total, inv.currency)}</p>
                    <p className="text-xs text-muted-foreground">on {formatDate(dueDate)}</p>
                  </div>
                  <div className="rounded-lg bg-accent/10 p-3">
                    <p className="text-[11px] uppercase tracking-wider text-accent font-semibold">You get back after</p>
                    <p className="font-heading text-xl font-semibold tabular-nums mt-1">{formatMoney(offer.reserveAmount, inv.currency)}</p>
                    <p className="text-xs text-muted-foreground">once the buyer pays</p>
                  </div>
                </div>
                <p className="text-sm mt-4 leading-relaxed">
                  You receive <strong>{formatMoney(offer.netDisbursement, inv.currency)}</strong> now. Your buyer pays <strong>{formatMoney(inv.total, inv.currency)}</strong> on{" "}
                  <strong>{formatDate(dueDate)}</strong>, and you get <strong>{formatMoney(offer.reserveAmount, inv.currency)}</strong> back after fees.
                </p>
                <details className="mt-3 text-sm">
                  <summary className="cursor-pointer text-muted-foreground">How the fees break down</summary>
                  <dl className="mt-2 divide-y text-sm">
                    {[
                      ["Advance", `${formatMoney(offer.advanceAmount, inv.currency)} (${Math.round(offer.advanceRate * 100)}% of the invoice)`],
                      ["Discount fee", `${formatMoney(offer.feeAmount, inv.currency)} · ${(offer.discountFeePerDay * 100).toFixed(3)}% per day × ${offer.tenorDays} days`],
                      ["Processing fee", formatMoney(offer.processingFee, inv.currency)],
                      ["Paid to you now", formatMoney(offer.netDisbursement, inv.currency)],
                      ["Held in reserve", formatMoney(offer.reserveAmount, inv.currency)],
                      ...(offer.expiresAt ? [["Offer valid until", formatDateTime(offer.expiresAt)]] : []),
                    ].map(([k, v]) => (
                      <div key={k} className="flex justify-between gap-3 py-1.5">
                        <dt className="text-muted-foreground">{k}</dt>
                        <dd className="tabular-nums text-right">{v}</dd>
                      </div>
                    ))}
                  </dl>
                </details>
                {canAccept && (
                  <Button className="w-full h-12 mt-4 text-base" onClick={() => setAcceptOpen(true)} disabled={accept.isPending}>
                    {accept.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />} Accept offer — get {formatMoney(offer.netDisbursement, inv.currency)}
                  </Button>
                )}
              </div>
            )}

            {/* Advance / repayment */}
            {adv && (
              <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
                <h2 className="font-heading text-base font-semibold">Advance</h2>
                <dl className="mt-2 divide-y text-sm">
                  {[
                    ["Paid to you", `${formatMoney(adv.principal, inv.currency)} on ${formatDate(adv.disbursedAt)}`],
                    ["Buyer has paid", formatMoney(adv.repaid, inv.currency)],
                    ["Still due from buyer", formatMoney(outstanding, inv.currency)],
                    ["Reserve released to you", formatMoney(adv.reserveReleased, inv.currency)],
                    ["Due date", formatDate(adv.dueDate)],
                  ].map(([k, v]) => (
                    <div key={k} className="flex justify-between gap-3 py-1.5">
                      <dt className="text-muted-foreground">{k}</dt>
                      <dd className={cn("tabular-nums text-right", k === "Still due from buyer" && status === "OVERDUE" && "text-destructive font-semibold")}>{v}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            )}

            {/* Extracted fields */}
            <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
              <div className="flex items-center justify-between mb-2">
                <h2 className="font-heading text-base font-semibold">Invoice details</h2>
                {inv.ocr?.engine && <span className="text-[11px] text-muted-foreground font-mono">{inv.ocr.engine}</span>}
              </div>
              {status === "UPLOADED" ? <EmptyState icon={FileText} title="Reading your invoice…" /> : <ExtractedFields invoice={inv} editable={CANCELLABLE.has(status)} />}
            </div>

            {/* Document */}
            <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
              <div className="flex items-center justify-between mb-2">
                <h2 className="font-heading text-base font-semibold">Document</h2>
                {pdfUrl && (
                  <a href={pdfUrl} download={`${inv.invoiceNumber ?? inv.id}.pdf`} className="text-sm text-primary hover:underline inline-flex items-center gap-1">
                    <Download className="h-4 w-4" /> Download
                  </a>
                )}
              </div>
              {pdfErr ? (
                <EmptyState icon={FileText} title="Preview unavailable" description={pdfErr} />
              ) : !pdfUrl ? (
                <LoadingState label="Fetching document…" />
              ) : (
                <iframe title="Invoice document" src={pdfUrl} className="w-full h-[360px] sm:h-[520px] rounded-md border bg-white" />
              )}
            </div>
          </div>

          <div className="lg:col-span-2 space-y-4">
            {/* Pipeline */}
            <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
              <h2 className="font-heading text-base font-semibold mb-3">Progress</h2>
              <PipelineStepper status={status} statusReason={inv.statusReason} polling={LIVE_STATUSES.has(status)} />
            </div>

            {/* Verify + QR */}
            {verify && (
              <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
                <h2 className="font-heading text-base font-semibold">Proof of record</h2>
                <p className="text-sm text-muted-foreground mt-1">Anyone can check this invoice's status without seeing the amounts.</p>
                <div className="mt-3 flex flex-col sm:flex-row lg:flex-col items-center gap-4">
                  <QrCode value={verify} size={150} />
                  <div className="min-w-0 w-full space-y-2 text-sm">
                    <a href={verify} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-primary hover:underline break-all">
                      <LinkIcon className="h-4 w-4 shrink-0" /> Public verify page <ExternalLink className="h-3 w-3 shrink-0" />
                    </a>
                    <div className="flex items-center gap-1 text-xs text-muted-foreground">
                      <span>fingerprint</span>
                      <code className="font-mono text-foreground">{shortHash(inv.fingerprint, 10)}</code>
                      <CopyButton value={inv.fingerprint!} />
                    </div>
                    <CopyButton value={verify} label="Copy link" className="border rounded-md px-2 py-1.5" />
                  </div>
                </div>
              </div>
            )}

            {/* Timeline */}
            <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
              <div className="flex items-center justify-between mb-3">
                <h2 className="font-heading text-base font-semibold">History</h2>
                <div className="flex items-center gap-1.5">
                  {anchored && (
                    <Badge variant="outline" className="text-[11px] bg-accent/10 text-accent border-accent/30">
                      <Anchor className="h-3 w-3 mr-1" /> anchored
                    </Badge>
                  )}
                  {tl.data && (
                    <Badge variant="outline" className={cn("text-[11px]", tl.data.chainOk ? "bg-accent/15 text-accent border-accent/40" : "bg-destructive/15 text-destructive border-destructive/40")}>
                      {tl.data.chainOk ? <ShieldCheck className="h-3 w-3 mr-1" /> : <ShieldAlert className="h-3 w-3 mr-1" />}
                      {tl.data.chainOk ? "chain OK" : "chain broken"}
                    </Badge>
                  )}
                </div>
              </div>
              {tl.isLoading ? (
                <LoadingState label="Verifying…" />
              ) : tl.isError ? (
                <ErrorState error={tl.error} onRetry={() => tl.refetch()} />
              ) : events.length === 0 ? (
                <EmptyState title="No events yet" />
              ) : (
                <ol className="relative">
                  {[...events].reverse().map((e, i, arr) => (
                    <li key={e.seq ?? i} className="relative flex gap-3 pb-4 last:pb-0">
                      {i < arr.length - 1 && <span className="absolute left-[5px] top-3 bottom-0 w-px bg-border" aria-hidden />}
                      <span className={cn("relative z-10 mt-1.5 h-3 w-3 rounded-full border-2 bg-card shrink-0", /declin|dispute|reject|overdue|cancel/.test(e.type) ? "border-destructive" : /fund|paid|confirm|approv|accept/.test(e.type) ? "border-accent" : "border-primary")} />
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium">{eventLabel(e.type)}</p>
                        <p className="text-xs text-muted-foreground">
                          {formatDateTime(e.occurredAt)} <span className="font-mono">#{e.seq}</span>
                        </p>
                      </div>
                    </li>
                  ))}
                </ol>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Accept dialog */}
      <Dialog open={acceptOpen} onOpenChange={setAcceptOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="font-heading">Accept this offer?</DialogTitle>
            <DialogDescription>Please read the terms once more.</DialogDescription>
          </DialogHeader>
          {offer && (
            <p className="text-base leading-relaxed">
              You receive <strong>{formatMoney(offer.netDisbursement, inv.currency)}</strong> now. Your buyer pays <strong>{formatMoney(inv.total, inv.currency)}</strong> on <strong>{formatDate(dueDate)}</strong>. You get{" "}
              <strong>{formatMoney(offer.reserveAmount, inv.currency)}</strong> back after fees of {formatMoney(offer.feeAmount + offer.processingFee, inv.currency)}.
            </p>
          )}
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setAcceptOpen(false)} disabled={accept.isPending}>
              Not now
            </Button>
            <Button onClick={() => accept.mutate()} disabled={accept.isPending}>
              {accept.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />} Accept and get paid
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Cancel dialog */}
      <Dialog open={cancelOpen} onOpenChange={setCancelOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="font-heading">Cancel this request?</DialogTitle>
            <DialogDescription>You can cancel any time before funds are paid out. Your buyer won't be charged anything.</DialogDescription>
          </DialogHeader>
          <Textarea value={cancelReason} onChange={(e) => setCancelReason(e.target.value)} placeholder="Reason (optional)" rows={3} className="text-sm" />
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setCancelOpen(false)} disabled={cancel.isPending}>
              Keep it
            </Button>
            <Button variant="destructive" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
              {cancel.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Ban className="h-4 w-4" />} Cancel request
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SellerShell>
  );
};

export default SellerInvoiceDetailPage;
