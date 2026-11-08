import { useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Camera, FileUp, Loader2, X, ExternalLink, Check, ArrowRight, Mail } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { cn } from "@/lib/utils";
import { appConfig } from "@/lib/appConfig";
import { formatMoney } from "@/lib/format";
import { invoiceService, isSellerProfileMissing, type InvoiceDetail, type UploadResponse, type SellerApiError } from "@/services/invoiceService";
import { PipelineStepper, pipelinePosition } from "@/pages/invoice-finance/components/PipelineStepper";
import { CopyButton } from "@/pages/invoice-finance/components/CopyButton";
import { SellerShell } from "./components/SellerShell";
import { SellerOnboardingPage } from "./SellerOnboardingPage";
import { SellerStatusPill } from "./components/SellerStatusPill";
import { ExtractedFields } from "./components/ExtractedFields";
import { statusHint } from "./components/sellerUtils";

const TERMINAL = new Set(["OFFERED", "ACCEPTED", "FUNDED", "PAID", "PARTIALLY_PAID", "DECLINED", "CANCELLED", "BUYER_DISPUTED", "OVERDUE"]);
const MAX_MB = 15;

const SellerUploadPage = () => {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [buyerEmail, setBuyerEmail] = useState("");
  const [created, setCreated] = useState<UploadResponse | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const cameraRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!file || !file.type.startsWith("image/")) {
      setPreview(null);
      return;
    }
    const url = URL.createObjectURL(file);
    setPreview(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  const upload = useMutation({
    mutationFn: () => invoiceService.uploadMine({ file: file!, buyerEmail: buyerEmail.trim() || undefined }),
    onSuccess: (inv) => {
      setCreated(inv);
      qc.invalidateQueries({ queryKey: ["seller-invoices"] });
      qc.invalidateQueries({ queryKey: ["seller-summary"] });
    },
  });

  const live = useQuery<InvoiceDetail>({
    queryKey: ["seller-invoice", created?.id],
    queryFn: () => invoiceService.getMine(created!.id),
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
  const err = upload.error as SellerApiError | null;

  if (isSellerProfileMissing(err)) {
    return <SellerOnboardingPage onRetry={() => upload.reset()} />;
  }

  const pickFile = (f: File | null | undefined) => {
    if (!f) return;
    if (f.size > MAX_MB * 1024 * 1024) {
      upload.reset();
      setFile(null);
      alert(`That file is ${(f.size / 1024 / 1024).toFixed(1)} MB — the limit is ${MAX_MB} MB.`);
      return;
    }
    setFile(f);
  };

  const buyerEmailOk = !buyerEmail.trim() || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(buyerEmail.trim());
  const canSubmit = !!file && buyerEmailOk && !upload.isPending;

  return (
    <SellerShell
      title={created ? "Invoice submitted" : "Finance an invoice"}
      subtitle={created ? statusHint(current?.status) : "Upload the invoice PDF or a clear photo. We read it, ask your buyer to confirm, then make you an offer."}
      back="/"
    >
      {!created ? (
        <div className="space-y-5 animate-fade-in">
          {/* File picker */}
          <div
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDragOver(false);
              pickFile(e.dataTransfer.files?.[0]);
            }}
            className={cn(
              "rounded-xl border-2 border-dashed p-5 sm:p-8 text-center transition-colors",
              dragOver ? "border-primary bg-primary/5" : "border-border bg-card/40 hover:border-primary/50"
            )}
          >
            <input ref={fileRef} type="file" accept="application/pdf,image/*" className="hidden" onChange={(e) => pickFile(e.target.files?.[0])} />
            <input ref={cameraRef} type="file" accept="image/*" capture="environment" className="hidden" onChange={(e) => pickFile(e.target.files?.[0])} />
            {file ? (
              <div className="space-y-3">
                {preview ? (
                  <img src={preview} alt="Invoice preview" className="mx-auto max-h-56 rounded-lg border object-contain bg-white" />
                ) : (
                  <div className="mx-auto h-16 w-16 rounded-xl bg-accent/10 flex items-center justify-center">
                    <FileUp className="h-7 w-7 text-accent" />
                  </div>
                )}
                <div className="flex items-center justify-center gap-2 text-sm">
                  <span className="font-medium truncate max-w-[220px] sm:max-w-xs">{file.name}</span>
                  <span className="text-xs text-muted-foreground">{(file.size / 1024).toFixed(0)} KB</span>
                  <button type="button" className="p-1 rounded hover:bg-muted" onClick={() => setFile(null)} aria-label="Remove file">
                    <X className="h-4 w-4" />
                  </button>
                </div>
              </div>
            ) : (
              <div className="space-y-4">
                <div className="mx-auto h-14 w-14 rounded-xl bg-primary/10 flex items-center justify-center">
                  <FileUp className="h-7 w-7 text-primary" />
                </div>
                <div>
                  <p className="text-base font-medium">Drop your invoice here</p>
                  <p className="text-sm text-muted-foreground mt-1">PDF or photo · up to {MAX_MB} MB</p>
                </div>
                <div className="flex flex-col sm:flex-row gap-2 justify-center">
                  <Button type="button" variant="outline" className="h-11" onClick={() => fileRef.current?.click()}>
                    <FileUp className="h-4 w-4" /> Choose file
                  </Button>
                  <Button type="button" variant="outline" className="h-11 sm:hidden" onClick={() => cameraRef.current?.click()}>
                    <Camera className="h-4 w-4" /> Take a photo
                  </Button>
                </div>
              </div>
            )}
          </div>

          {/* Buyer email */}
          <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5 space-y-2">
            <Label htmlFor="buyer-email" className="text-sm">
              Buyer's email <span className="text-muted-foreground font-normal">(who pays this invoice)</span>
            </Label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-accent/80" />
              <Input
                id="buyer-email"
                type="email"
                inputMode="email"
                autoComplete="off"
                placeholder="accounts@buyer.co.ke"
                value={buyerEmail}
                onChange={(e) => setBuyerEmail(e.target.value)}
                className={cn("h-11 pl-10 text-base", !buyerEmailOk && "border-destructive")}
              />
            </div>
            <p className="text-xs text-muted-foreground">We email your buyer a one-tap confirmation. Leave blank if the address is printed on the invoice.</p>
          </div>

          {err && (
            <Alert variant="destructive">
              <AlertTitle>{err.status === 409 ? "Already submitted" : err.status === 401 ? "Signed out" : "Upload failed"}</AlertTitle>
              <AlertDescription className="text-sm">
                {err.status === 409 && !/SELLER_PROFILE/.test(err.message) ? "This invoice (same document or invoice number for this buyer) has already been submitted." : err.message}
                {err.status === 401 && (
                  <>
                    {" "}
                    <Link to="/login" className="underline">
                      Sign in again
                    </Link>
                    .
                  </>
                )}
              </AlertDescription>
            </Alert>
          )}

          <Button className="w-full h-12 text-base" onClick={() => upload.mutate()} disabled={!canSubmit}>
            {upload.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <ArrowRight className="h-4 w-4" />}
            {upload.isPending ? "Uploading…" : "Submit for financing"}
          </Button>
          <p className="text-xs text-muted-foreground text-center">By submitting you confirm this invoice is genuine and unpaid. Everything is written to a tamper-evident ledger.</p>
        </div>
      ) : (
        <div className="space-y-5 animate-fade-in">
          <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="eyebrow">Progress</p>
                <p className="font-heading text-lg font-semibold mt-1 truncate">
                  {current?.invoiceNumber ? `Invoice #${current.invoiceNumber}` : "Reading your invoice…"}
                  {current?.total ? <span className="text-muted-foreground font-normal text-base"> · {formatMoney(current.total, current.currency)}</span> : null}
                </p>
              </div>
              <SellerStatusPill status={current?.status} />
            </div>
            <PipelineStepper
              className="mt-4"
              status={current?.status}
              statusReason={current?.statusReason}
              polling={polling}
              notes={{
                buyer:
                  current?.status === "AWAITING_BUYER" && confirmLink && appConfig.demo ? (
                    <div className="mt-1 rounded-md border border-warning/30 bg-warning/5 p-2 space-y-1">
                      <p className="text-[11px] uppercase tracking-wide text-warning font-semibold">Demo — buyer confirm link</p>
                      <div className="flex items-center gap-1">
                        <code className="text-[11px] font-mono truncate flex-1">{confirmLink}</code>
                        <CopyButton value={confirmLink} />
                        <a href={confirmLink} target="_blank" rel="noreferrer" className="p-1.5 rounded hover:bg-muted" title="Open">
                          <ExternalLink className="h-3.5 w-3.5" />
                        </a>
                      </div>
                      <p className="text-[11px] text-muted-foreground">Buyer OTP in demo is 000000.</p>
                    </div>
                  ) : current?.status === "AWAITING_BUYER" ? (
                    <span>We've emailed {current.buyerSnapshot?.email ?? "your buyer"}. You'll move on as soon as they confirm.</span>
                  ) : pipelinePosition(current?.status).done >= 2 ? (
                    <span className="inline-flex items-center gap-1">
                      <Check className="h-3 w-3 text-accent" /> Confirmed by {current?.buyerSnapshot?.name ?? "buyer"}
                    </span>
                  ) : undefined,
                offered: current?.offer ? (
                  <span>
                    You'd receive <span className="font-semibold text-foreground">{formatMoney(current.offer.netDisbursement, current.currency)}</span> now.
                  </span>
                ) : undefined,
              }}
            />
            {live.isError && <p className="text-xs text-destructive mt-2">Lost contact with the service — {(live.error as Error)?.message}</p>}
          </div>

          {current && current.status !== "UPLOADED" && (
            <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
              <div className="flex items-center justify-between mb-2">
                <h2 className="font-heading text-base font-semibold">What we read</h2>
                {current.ocr?.engine && <span className="text-[11px] text-muted-foreground font-mono">{current.ocr.engine}</span>}
              </div>
              <ExtractedFields invoice={current} />
            </div>
          )}

          <div className="flex flex-col sm:flex-row gap-2">
            <Button className="h-11 flex-1" onClick={() => navigate(`/invoices/${created.id}`)}>
              Open invoice <ArrowRight className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              className="h-11"
              onClick={() => {
                setCreated(null);
                setFile(null);
                setBuyerEmail("");
                upload.reset();
              }}
            >
              Finance another
            </Button>
          </div>
        </div>
      )}
    </SellerShell>
  );
};

export default SellerUploadPage;
