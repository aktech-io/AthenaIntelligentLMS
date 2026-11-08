import { useRef, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery, useMutation } from "@tanstack/react-query";
import { FileSearch, FileUp, Loader2, ShieldCheck, ShieldAlert, Check, Minus, Anchor, SearchX } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { invoiceService, shortHash, type VerifyResult } from "@/services/invoiceService";
import { formatDateTime } from "@/lib/format";
import { InvoiceStatusPill } from "@/pages/invoice-finance/components/InvoiceStatusPill";
import { CopyButton } from "@/pages/invoice-finance/components/CopyButton";
import { PublicShell } from "./PublicShell";

function Row({ ok, label, detail }: { ok: boolean | null; label: string; detail?: string }) {
  return (
    <div className="flex items-center justify-between py-2.5 text-sm">
      <div className="flex items-center gap-2">
        <span
          className={cn(
            "h-5 w-5 rounded-full flex items-center justify-center",
            ok === true ? "bg-accent text-accent-foreground" : ok === false ? "bg-destructive/15 text-destructive" : "bg-muted text-muted-foreground"
          )}
        >
          {ok === true ? <Check className="h-3 w-3" /> : ok === false ? <ShieldAlert className="h-3 w-3" /> : <Minus className="h-3 w-3" />}
        </span>
        <span>{label}</span>
      </div>
      <span className="text-xs text-muted-foreground tabular-nums">{detail ?? ""}</span>
    </div>
  );
}

function ResultCard({ r, fingerprint }: { r: VerifyResult; fingerprint?: string }) {
  const fp = r.fingerprint ?? fingerprint;
  if (!r.known) {
    return (
      <Card>
        <CardContent className="py-10 text-center">
          <SearchX className="h-10 w-10 mx-auto text-muted-foreground mb-3" />
          <h2 className="text-lg font-heading">Not known to Nemo</h2>
          <p className="text-sm text-muted-foreground mt-1">This document has not been submitted for financing on this platform.</p>
          {fp && (
            <p className="text-[11px] font-mono text-muted-foreground mt-3 inline-flex items-center">
              {shortHash(fp, 16)} <CopyButton value={fp} />
            </p>
          )}
        </CardContent>
      </Card>
    );
  }
  return (
    <Card className={cn("border", r.chainOk === false ? "border-destructive/50" : "border-accent/40")}>
      <CardHeader className="pb-2 flex flex-row items-start justify-between gap-3">
        <div>
          <CardTitle className="font-heading text-lg flex items-center gap-2">
            {r.chainOk === false ? <ShieldAlert className="h-5 w-5 text-destructive" /> : <ShieldCheck className="h-5 w-5 text-accent" />}
            Known invoice
          </CardTitle>
          {fp && (
            <p className="text-[11px] font-mono text-muted-foreground mt-1 inline-flex items-center">
              {shortHash(fp, 16)} <CopyButton value={fp} />
            </p>
          )}
        </div>
        {r.status && <InvoiceStatusPill status={r.status} />}
      </CardHeader>
      <CardContent className="divide-y">
        <Row ok={true} label="Document known to Nemo" />
        <Row ok={!!r.buyerConfirmedAt} label="Buyer confirmed" detail={r.buyerConfirmedAt ? formatDateTime(r.buyerConfirmedAt) : "not yet"} />
        <Row ok={r.fundedAt ? true : null} label="Funded" detail={r.fundedAt ? formatDateTime(r.fundedAt) : "—"} />
        <Row ok={r.anchoredAt ? true : null} label="Anchored" detail={r.anchoredAt ? formatDateTime(r.anchoredAt) : "pending next anchor"} />
        <Row ok={r.chainOk == null ? null : r.chainOk} label="Ledger chain integrity" detail={r.chainOk == null ? "—" : r.chainOk ? "verified" : "BROKEN"} />
      </CardContent>
    </Card>
  );
}

const VerifyPage = () => {
  const { fingerprint } = useParams();
  const navigate = useNavigate();
  const [manual, setManual] = useState("");
  const [dragOver, setDragOver] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const byFp = useQuery<VerifyResult>({
    queryKey: ["public-verify", fingerprint],
    queryFn: () => invoiceService.publicVerifyFingerprint(fingerprint!),
    enabled: !!fingerprint,
    retry: false,
  });
  const byFile = useMutation({ mutationFn: (f: File) => invoiceService.publicVerifyFile(f) });

  const onFile = (f?: File | null) => {
    if (f) byFile.mutate(f);
  };

  const result = byFile.data ?? byFp.data;
  const busy = byFp.isLoading || byFile.isPending;
  const error = (byFile.error ?? byFp.error) as Error | null;

  return (
    <PublicShell caption="Verify an invoice">
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-heading">Verify an invoice</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Drop the PDF you were sent, or paste its fingerprint. Nemo computes a SHA-256 of the file and tells you whether the buyer confirmed it, whether it has been financed, and whether its ledger record is intact — without revealing any amounts or parties.
          </p>
        </div>

        <div
          onDragOver={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragOver(false);
            onFile(e.dataTransfer.files?.[0]);
          }}
          onClick={() => fileRef.current?.click()}
          className={cn(
            "rounded-xl border-2 border-dashed p-8 text-center cursor-pointer transition-colors",
            dragOver ? "border-primary bg-primary/5" : "border-border hover:border-primary/50 hover:bg-muted/40"
          )}
        >
          <input ref={fileRef} type="file" accept="application/pdf" className="hidden" onChange={(e) => onFile(e.target.files?.[0])} />
          {byFile.isPending ? <Loader2 className="h-7 w-7 mx-auto text-primary animate-spin mb-2" /> : <FileUp className="h-7 w-7 mx-auto text-muted-foreground mb-2" />}
          <p className="text-sm font-medium">{byFile.isPending ? "Computing fingerprint…" : "Drop the invoice PDF here"}</p>
          <p className="text-xs text-muted-foreground mt-1">or click to choose a file · the file is hashed, not stored</p>
        </div>

        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            const fp = manual.trim().toLowerCase();
            if (/^[0-9a-f]{64}$/.test(fp)) navigate(`/verify/${fp}`);
          }}
        >
          <Input value={manual} onChange={(e) => setManual(e.target.value)} placeholder="…or paste a 64-character SHA-256 fingerprint" className="font-mono text-xs" />
          <Button type="submit" variant="outline" disabled={!/^[0-9a-f]{64}$/i.test(manual.trim())}>
            <FileSearch className="h-4 w-4 mr-1" /> Check
          </Button>
        </form>

        {busy && !byFile.isPending && (
          <Card>
            <CardContent className="py-10 flex items-center justify-center text-sm text-muted-foreground">
              <Loader2 className="h-5 w-5 animate-spin mr-2" /> Checking ledger…
            </CardContent>
          </Card>
        )}
        {error && !busy && (
          <Card className="border-destructive/40">
            <CardContent className="py-8 text-center">
              <ShieldAlert className="h-8 w-8 mx-auto text-destructive mb-2" />
              <p className="text-sm font-medium">Verification unavailable</p>
              <p className="text-xs text-muted-foreground mt-1">{error.message}</p>
            </CardContent>
          </Card>
        )}
        {result && !busy && <ResultCard r={result} fingerprint={fingerprint} />}

        <p className="text-[11px] text-muted-foreground flex items-center gap-1.5">
          <Anchor className="h-3 w-3" /> Ledger roots are periodically anchored so records cannot be rewritten after the fact.
        </p>
      </div>
    </PublicShell>
  );
};

export default VerifyPage;
