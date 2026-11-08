import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Loader2, Pencil, X, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import { invoiceService, type InvoiceDetail, type SellerApiError } from "@/services/invoiceService";
import { formatMoney, formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import { FIELD_LABELS } from "./sellerUtils";

export function ConfidenceBar({ value, className }: { value: number; className?: string }) {
  const pct = Math.round(Math.max(0, Math.min(1, value)) * 100);
  const tone = value >= 0.85 ? "bg-accent" : value >= 0.6 ? "bg-warning" : "bg-destructive";
  return (
    <div className={cn("flex items-center gap-2 w-24 sm:w-28", className)} title={`OCR confidence ${pct}%`}>
      <div className="h-1.5 flex-1 rounded-full bg-muted overflow-hidden">
        <div className={cn("h-full rounded-full transition-[width]", tone)} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-[11px] tabular-nums font-mono w-8 text-right">{pct}%</span>
    </div>
  );
}

const DATE_FIELDS = new Set(["issueDate", "dueDate"]);
const MONEY_FIELDS = new Set(["subtotal", "vat", "total"]);

function rawValue(inv: InvoiceDetail, key: string): string {
  switch (key) {
    case "sellerName":
      return inv.seller?.name ?? "";
    case "sellerTaxId":
      return inv.seller?.taxId ?? "";
    case "buyerName":
      return inv.buyerSnapshot?.name ?? "";
    case "buyerTaxId":
      return inv.buyerSnapshot?.taxId ?? "";
    case "buyerEmail":
      return inv.buyerSnapshot?.email ?? "";
    case "lineItems":
      return `${inv.lineItems?.length ?? 0}`;
    default: {
      const v = (inv as unknown as Record<string, unknown>)[key];
      if (v == null) return "";
      if (DATE_FIELDS.has(key)) return String(v).slice(0, 10);
      return typeof v === "object" ? JSON.stringify(v) : String(v);
    }
  }
}

function displayValue(inv: InvoiceDetail, key: string, override?: string): string {
  const raw = override ?? rawValue(inv, key);
  if (raw === "") return "—";
  if (DATE_FIELDS.has(key)) return formatDate(raw);
  if (MONEY_FIELDS.has(key)) return formatMoney(Number(raw), inv.currency);
  if (key === "lineItems") return `${raw} line${raw === "1" ? "" : "s"}`;
  return raw;
}

interface Props {
  invoice: InvoiceDetail;
  /** Allow inline editing of fields flagged needsSellerConfirmation. */
  editable?: boolean;
  compact?: boolean;
}

/**
 * OCR-extracted fields with confidence bars. Fields in `needsSellerConfirmation`
 * (or confidence < 0.6) get an inline editor; confirmed values are sent via
 * PATCH /invoices/{id}/fields (assumed endpoint — on 404/405 we keep the
 * values on-screen and tell the seller the bank will confirm at review).
 */
export function ExtractedFields({ invoice: inv, editable = true, compact }: Props) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const confidence = inv.ocr?.confidence;
  const conf = useMemo(() => confidence ?? {}, [confidence]);
  const needs = useMemo(
    () => new Set(inv.needsSellerConfirmation ?? Object.entries(conf).filter(([, c]) => c < 0.6).map(([k]) => k)),
    [inv.needsSellerConfirmation, conf]
  );
  const keys = useMemo(() => {
    const core = ["invoiceNumber", "issueDate", "dueDate", "total", "buyerName", "buyerEmail"];
    const all = new Set<string>([...core, ...Object.keys(FIELD_LABELS).filter((k) => k in conf), ...Object.keys(conf), ...needs]);
    return Array.from(all).filter((k) => k !== "lineItems" || (inv.lineItems?.length ?? 0) > 0);
  }, [conf, needs, inv.lineItems]);

  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [local, setLocal] = useState<Record<string, string>>({});
  const [confirmed, setConfirmed] = useState<Set<string>>(new Set());
  const [unsupported, setUnsupported] = useState(false);

  useEffect(() => {
    setLocal({});
    setConfirmed(new Set());
  }, [inv.id]);

  const save = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) => invoiceService.confirmFields(inv.id, { [key]: MONEY_FIELDS.has(key) ? Number(value) : value }),
    onSuccess: (_d, vars) => {
      setConfirmed((s) => new Set(s).add(vars.key));
      qc.invalidateQueries({ queryKey: ["seller-invoice", inv.id] });
      toast({ title: "Saved", description: `${FIELD_LABELS[vars.key] ?? vars.key} confirmed.` });
    },
    onError: (e: SellerApiError, vars) => {
      if (e.status === 404 || e.status === 405 || e.status === 501) {
        // Endpoint not available yet: keep the seller's value visible and explain.
        setUnsupported(true);
        setConfirmed((s) => new Set(s).add(vars.key));
      } else {
        toast({ title: "Couldn't save", description: e.message, variant: "destructive" });
      }
    },
  });

  const startEdit = (key: string) => {
    setEditing(key);
    setDraft(local[key] ?? rawValue(inv, key));
  };
  const commit = () => {
    if (!editing) return;
    const key = editing;
    const value = draft.trim();
    setLocal((l) => ({ ...l, [key]: value }));
    setEditing(null);
    save.mutate({ key, value });
  };
  const confirmAsIs = (key: string) => {
    setLocal((l) => ({ ...l, [key]: rawValue(inv, key) }));
    save.mutate({ key, value: rawValue(inv, key) });
  };

  const pending = Array.from(needs).filter((k) => !confirmed.has(k));

  return (
    <div className="space-y-3">
      {editable && pending.length > 0 && (
        <div className="flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/5 px-3 py-2 text-sm">
          <AlertTriangle className="h-4 w-4 text-warning shrink-0 mt-0.5" />
          <p>
            We weren't sure about <span className="font-medium">{pending.map((k) => (FIELD_LABELS[k] ?? k).toLowerCase()).join(", ")}</span>. Please check and confirm or correct {pending.length === 1 ? "it" : "them"}.
          </p>
        </div>
      )}
      {unsupported && (
        <p className="text-xs text-muted-foreground">Your corrections are noted on this device; the bank will confirm these values with you at review.</p>
      )}
      <div className="divide-y">
        {keys.map((k) => {
          const flagged = needs.has(k);
          const isEditing = editing === k;
          const done = confirmed.has(k);
          const canEdit = editable && flagged && k !== "lineItems";
          return (
            <div key={k} className={cn("py-2.5 grid gap-x-3 gap-y-1 items-center", compact ? "grid-cols-[1fr_auto]" : "grid-cols-[1fr_auto] sm:grid-cols-[150px_1fr_auto]")}>
              <span className="text-sm text-muted-foreground">{FIELD_LABELS[k] ?? k}</span>
              <div className={cn("justify-self-end sm:justify-self-auto flex items-center gap-2 min-w-0", compact ? "" : "col-span-2 sm:col-span-1 order-3 sm:order-none")}>
                {isEditing ? (
                  <form
                    className="flex items-center gap-1.5 w-full"
                    onSubmit={(e) => {
                      e.preventDefault();
                      commit();
                    }}
                  >
                    <Input
                      autoFocus
                      value={draft}
                      onChange={(e) => setDraft(e.target.value)}
                      type={DATE_FIELDS.has(k) ? "date" : MONEY_FIELDS.has(k) ? "number" : k === "buyerEmail" ? "email" : "text"}
                      inputMode={MONEY_FIELDS.has(k) ? "decimal" : undefined}
                      className="h-9 text-sm font-mono"
                    />
                    <Button type="submit" size="icon" className="h-9 w-9 shrink-0" aria-label="Save">
                      <Check className="h-4 w-4" />
                    </Button>
                    <Button type="button" size="icon" variant="ghost" className="h-9 w-9 shrink-0" onClick={() => setEditing(null)} aria-label="Cancel">
                      <X className="h-4 w-4" />
                    </Button>
                  </form>
                ) : (
                  <>
                    <span className={cn("text-sm tabular-nums truncate", flagged && !done && "text-warning font-medium")}>{displayValue(inv, k, local[k])}</span>
                    {done && (
                      <Badge variant="outline" className="text-[11px] bg-accent/10 text-accent border-accent/30 shrink-0">
                        <Check className="h-3 w-3 mr-0.5" /> confirmed
                      </Badge>
                    )}
                    {canEdit && !done && (
                      <span className="flex items-center gap-1 shrink-0">
                        <Button type="button" size="sm" variant="outline" className="h-8 px-2 text-xs" onClick={() => confirmAsIs(k)} disabled={save.isPending}>
                          {save.isPending && save.variables?.key === k ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />} Correct
                        </Button>
                        <Button type="button" size="sm" variant="ghost" className="h-8 px-2 text-xs" onClick={() => startEdit(k)} aria-label={`Edit ${FIELD_LABELS[k] ?? k}`}>
                          <Pencil className="h-3.5 w-3.5" /> Edit
                        </Button>
                      </span>
                    )}
                  </>
                )}
              </div>
              <div className={cn("justify-self-end", compact && "col-span-2 justify-self-start")}>
                {k in conf ? <ConfidenceBar value={conf[k]} /> : <span className="text-[11px] text-muted-foreground">—</span>}
              </div>
            </div>
          );
        })}
      </div>
      {inv.lineItems && inv.lineItems.length > 0 && !compact && (
        <details className="text-sm">
          <summary className="cursor-pointer text-muted-foreground">Line items ({inv.lineItems.length})</summary>
          <div className="mt-2 overflow-x-auto">
            <table className="w-full text-xs">
              <thead className="text-muted-foreground">
                <tr className="border-b">
                  <th className="text-left font-medium py-1.5">Description</th>
                  <th className="text-right font-medium py-1.5">Qty</th>
                  <th className="text-right font-medium py-1.5">Amount</th>
                </tr>
              </thead>
              <tbody className="tabular-nums font-mono">
                {inv.lineItems.map((li, i) => (
                  <tr key={i} className="border-b last:border-0">
                    <td className="py-1.5 font-sans">{li.description}</td>
                    <td className="text-right py-1.5">{li.qty}</td>
                    <td className="text-right py-1.5">{formatMoney(li.amount, inv.currency, { decimals: 2 })}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </div>
  );
}
