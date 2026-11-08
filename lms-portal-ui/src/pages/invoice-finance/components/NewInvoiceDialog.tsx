import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useMutation } from "@tanstack/react-query";
import { FileUp, Loader2, Search, X, ExternalLink, Check } from "lucide-react";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { cn } from "@/lib/utils";
import { customerService, type Customer } from "@/services/customerService";
import { invoiceService, type InvoiceDetail, type UploadResponse } from "@/services/invoiceService";
import { formatMoney } from "@/lib/format";
import { PipelineStepper, pipelinePosition } from "./PipelineStepper";
import { InvoiceStatusPill } from "./InvoiceStatusPill";
import { CopyButton } from "./CopyButton";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated?: (invoice: InvoiceDetail) => void;
}

const TERMINAL = new Set(["OFFERED", "ACCEPTED", "FUNDED", "PAID", "PARTIALLY_PAID", "DECLINED", "CANCELLED", "BUYER_DISPUTED", "OVERDUE"]);

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

export function NewInvoiceDialog({ open, onOpenChange, onCreated }: Props) {
  const [file, setFile] = useState<File | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [sellerQuery, setSellerQuery] = useState("");
  const [seller, setSeller] = useState<Customer | null>(null);
  const [buyerEmail, setBuyerEmail] = useState("");
  const [requested, setRequested] = useState("");
  const [created, setCreated] = useState<UploadResponse | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const debounced = useDebounced(sellerQuery, 300);
  const sellers = useQuery<Customer[]>({
    queryKey: ["if-seller-search", debounced],
    queryFn: () => customerService.searchCustomers(debounced),
    enabled: open && !seller && debounced.trim().length >= 2,
    staleTime: 30_000,
  });

  const reset = () => {
    setFile(null);
    setSeller(null);
    setSellerQuery("");
    setBuyerEmail("");
    setRequested("");
    setCreated(null);
    upload.reset();
  };
  useEffect(() => {
    if (!open) reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const upload = useMutation({
    mutationFn: () =>
      invoiceService.upload({
        file: file!,
        sellerCustomerId: seller!.customerId || seller!.id,
        buyerEmail: buyerEmail.trim() || undefined,
        requestedAmount: requested ? Number(requested) : undefined,
      }),
    onSuccess: (inv) => {
      setCreated(inv);
      onCreated?.(inv);
    },
  });

  // Poll the created invoice every 2s until the pipeline pauses or terminates
  const live = useQuery<InvoiceDetail>({
    queryKey: ["if-invoice", created?.id],
    queryFn: () => invoiceService.get(created!.id),
    enabled: !!created?.id,
    initialData: created ?? undefined,
    refetchInterval: (q) => {
      const s = q.state.data?.status;
      return s && TERMINAL.has(s) ? false : 2000;
    },
  });
  const current = live.data ?? created;
  const confirmLink = created?.confirmLink ?? (current as UploadResponse | null)?.confirmLink;
  const polling = !!current && !TERMINAL.has(current.status) && current.status !== "AWAITING_BUYER";

  const uploadError = upload.error as (Error & { status?: number }) | null;
  const canSubmit = !!file && !!seller && !upload.isPending;

  const ocrNote = useMemo(() => {
    if (!current?.ocr?.confidence) return null;
    const vals = Object.values(current.ocr.confidence);
    if (!vals.length) return null;
    const avg = vals.reduce((a, b) => a + b, 0) / vals.length;
    const low = current.needsSellerConfirmation ?? Object.entries(current.ocr.confidence).filter(([, c]) => c < 0.6).map(([k]) => k);
    return (
      <span>
        {current.invoiceNumber && (
          <>
            #{current.invoiceNumber} · {formatMoney(current.total, current.currency)} ·{" "}
          </>
        )}
        avg confidence {Math.round(avg * 100)}%
        {low.length > 0 && <span className="text-warning"> · seller to confirm: {low.join(", ")}</span>}
      </span>
    );
  }, [current]);

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const f = e.dataTransfer.files?.[0];
    if (f) setFile(f);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="font-heading">New invoice</DialogTitle>
          <DialogDescription>Upload on behalf of a seller. The pipeline extracts fields, asks the buyer to confirm, then scores and decides.</DialogDescription>
        </DialogHeader>

        {!created ? (
          <div className="space-y-4">
            {/* File */}
            <div
              onDragOver={(e) => {
                e.preventDefault();
                setDragOver(true);
              }}
              onDragLeave={() => setDragOver(false)}
              onDrop={onDrop}
              onClick={() => fileRef.current?.click()}
              className={cn(
                "rounded-lg border-2 border-dashed p-6 text-center cursor-pointer transition-colors",
                dragOver ? "border-primary bg-primary/5" : "border-border hover:border-primary/50 hover:bg-muted/40"
              )}
            >
              <input ref={fileRef} type="file" accept="application/pdf,image/*" className="hidden" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
              {file ? (
                <div className="flex items-center justify-center gap-2 text-sm">
                  <FileUp className="h-4 w-4 text-accent" />
                  <span className="font-medium truncate max-w-[260px]">{file.name}</span>
                  <span className="text-xs text-muted-foreground">{(file.size / 1024).toFixed(0)} KB</span>
                  <button
                    type="button"
                    className="p-1 rounded hover:bg-muted"
                    onClick={(e) => {
                      e.stopPropagation();
                      setFile(null);
                    }}
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
              ) : (
                <>
                  <FileUp className="h-6 w-6 mx-auto text-muted-foreground mb-2" />
                  <p className="text-sm">Drop the invoice PDF here, or click to choose</p>
                  <p className="text-xs text-muted-foreground mt-1">PDF or image · max 15 MB</p>
                </>
              )}
            </div>

            {/* Seller */}
            <div className="space-y-2">
              <Label>Seller (customer)</Label>
              {seller ? (
                <div className="flex items-center justify-between rounded-md border px-3 py-2 text-sm">
                  <div>
                    <span className="font-medium">
                      {seller.firstName} {seller.lastName}
                    </span>
                    <span className="ml-2 text-xs text-muted-foreground font-mono">{seller.customerId}</span>
                    {seller.email && <span className="ml-2 text-xs text-muted-foreground">{seller.email}</span>}
                  </div>
                  <button type="button" className="p-1 rounded hover:bg-muted" onClick={() => setSeller(null)}>
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
              ) : (
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                  <Input value={sellerQuery} onChange={(e) => setSellerQuery(e.target.value)} placeholder="Search by name, ID or phone…" className="pl-9" autoComplete="off" />
                  {debounced.trim().length >= 2 && (
                    <div className="absolute z-20 mt-1 w-full rounded-md border bg-popover shadow-md max-h-56 overflow-y-auto">
                      {sellers.isLoading ? (
                        <div className="p-3 text-xs text-muted-foreground flex items-center gap-2">
                          <Loader2 className="h-3 w-3 animate-spin" /> Searching…
                        </div>
                      ) : sellers.isError ? (
                        <div className="p-3 text-xs text-destructive">Customer search unavailable</div>
                      ) : (sellers.data ?? []).length === 0 ? (
                        <div className="p-3 text-xs text-muted-foreground">No customers match</div>
                      ) : (
                        (sellers.data ?? []).slice(0, 12).map((c) => (
                          <button
                            key={c.id}
                            type="button"
                            onClick={() => setSeller(c)}
                            className="w-full text-left px-3 py-2 text-sm hover:bg-muted flex items-center justify-between"
                          >
                            <span>
                              {c.firstName} {c.lastName}
                              {c.customerType && <span className="ml-2 text-[11px] uppercase text-muted-foreground">{c.customerType}</span>}
                            </span>
                            <span className="text-xs font-mono text-muted-foreground">{c.customerId}</span>
                          </button>
                        ))
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div className="space-y-2">
                <Label htmlFor="buyer-email">Buyer email</Label>
                <Input id="buyer-email" type="email" value={buyerEmail} onChange={(e) => setBuyerEmail(e.target.value)} placeholder="ap@buyer.co.ke" />
                <p className="text-[11px] text-muted-foreground">Optional — used if the PDF has no buyer contact.</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="requested">Requested amount</Label>
                <Input id="requested" type="number" min={0} value={requested} onChange={(e) => setRequested(e.target.value)} placeholder="Defaults to max advance" className="font-mono" />
              </div>
            </div>

            {uploadError && (
              <Alert variant="destructive">
                <AlertTitle>{uploadError.status === 409 ? "Duplicate invoice" : "Upload failed"}</AlertTitle>
                <AlertDescription className="text-xs">{uploadError.message}</AlertDescription>
              </Alert>
            )}
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">{current?.invoiceNumber ? `Invoice #${current.invoiceNumber}` : "Invoice created"}</p>
                <p className="text-xs text-muted-foreground font-mono">{current?.id}</p>
              </div>
              <InvoiceStatusPill status={current?.status} />
            </div>

            <PipelineStepper
              status={current?.status}
              statusReason={current?.statusReason}
              polling={polling}
              notes={{
                extracted: ocrNote,
                buyer:
                  current?.status === "AWAITING_BUYER" && confirmLink ? (
                    <div className="mt-1 rounded-md border border-warning/30 bg-warning/5 p-2 space-y-1">
                      <p className="text-[11px] uppercase tracking-wide text-warning font-semibold">Demo mode — buyer confirm link</p>
                      <div className="flex items-center gap-1">
                        <code className="text-[11px] font-mono truncate flex-1">{confirmLink}</code>
                        <CopyButton value={confirmLink} />
                        <a href={confirmLink} target="_blank" rel="noreferrer" className="p-1.5 rounded hover:bg-muted" title="Open">
                          <ExternalLink className="h-3.5 w-3.5" />
                        </a>
                      </div>
                      <p className="text-[11px] text-muted-foreground">OTP in demo is 000000.</p>
                    </div>
                  ) : current?.status === "AWAITING_BUYER" ? (
                    <span>Confirmation email sent to {current.buyerSnapshot?.email ?? "the buyer"}. Pipeline resumes when they respond.</span>
                  ) : pipelinePosition(current?.status).done >= 2 ? (
                    <span className="inline-flex items-center gap-1">
                      <Check className="h-3 w-3 text-accent" /> Confirmed by {current?.buyerSnapshot?.name ?? "buyer"}
                    </span>
                  ) : undefined,
                decided: current?.decision ? (
                  <span>
                    <InvoiceStatusPill status={current.decision.outcome} className="mr-1" />
                    {(current.decision.reasons ?? []).join(", ")}
                  </span>
                ) : undefined,
                offered: current?.offer ? (
                  <span>
                    {Math.round(current.offer.advanceRate * 100)}% advance · {formatMoney(current.offer.netDisbursement, current.currency)} net
                  </span>
                ) : undefined,
              }}
            />
            {live.isError && <p className="text-xs text-destructive">Lost contact with the invoice service — {String((live.error as Error)?.message)}</p>}
          </div>
        )}

        <DialogFooter>
          {!created ? (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)} disabled={upload.isPending}>
                Cancel
              </Button>
              <Button onClick={() => upload.mutate()} disabled={!canSubmit}>
                {upload.isPending && <Loader2 className="h-4 w-4 mr-1 animate-spin" />} Upload & start pipeline
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Close
              </Button>
              <Button asChild>
                <Link to={`/invoice-finance/invoices/${created.id}`}>Open invoice</Link>
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
