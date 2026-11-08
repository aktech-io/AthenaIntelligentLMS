import { Check, Loader2, X } from "lucide-react";
import { cn } from "@/lib/utils";
import type { InvoiceStatus } from "@/services/invoiceService";

export const PIPELINE_STEPS = [
  { key: "uploaded", label: "Uploaded" },
  { key: "extracted", label: "Extracted" },
  { key: "buyer", label: "Buyer confirmation" },
  { key: "scored", label: "Scored" },
  { key: "decided", label: "Decided" },
  { key: "offered", label: "Offered" },
  { key: "funded", label: "Funded" },
] as const;

/** Index of the furthest completed step (-1 = none), and whether the pipeline terminated. */
export function pipelinePosition(status?: InvoiceStatus | string | null): { done: number; terminal: "ok" | "fail" | null } {
  switch ((status ?? "").toUpperCase()) {
    case "UPLOADED":
      return { done: 0, terminal: null };
    case "EXTRACTED":
      return { done: 1, terminal: null };
    case "AWAITING_BUYER":
      return { done: 1, terminal: null };
    case "BUYER_CONFIRMED":
      return { done: 2, terminal: null };
    case "BUYER_DISPUTED":
      return { done: 2, terminal: "fail" };
    case "SCORED":
      return { done: 3, terminal: null };
    case "DECIDED":
      return { done: 4, terminal: null };
    case "OFFERED":
    case "ACCEPTED":
      return { done: 5, terminal: null };
    case "FUNDED":
    case "PARTIALLY_PAID":
    case "PAID":
    case "OVERDUE":
      return { done: 6, terminal: "ok" };
    case "DECLINED":
    case "CANCELLED":
      return { done: 4, terminal: "fail" };
    default:
      return { done: -1, terminal: null };
  }
}

interface Props {
  status?: InvoiceStatus | string | null;
  statusReason?: string;
  /** step-specific hints rendered under the label */
  notes?: Partial<Record<(typeof PIPELINE_STEPS)[number]["key"], React.ReactNode>>;
  polling?: boolean;
  className?: string;
}

export function PipelineStepper({ status, statusReason, notes, polling, className }: Props) {
  const { done, terminal } = pipelinePosition(status);
  const isAwaiting = (status ?? "").toUpperCase() === "AWAITING_BUYER";

  return (
    <ol className={cn("relative space-y-0", className)}>
      {PIPELINE_STEPS.map((step, i) => {
        const complete = i <= done && !(terminal === "fail" && i === done);
        const failed = terminal === "fail" && i === done;
        const current = !failed && !complete && i === done + 1 && terminal === null;
        const last = i === PIPELINE_STEPS.length - 1;
        return (
          <li key={step.key} className="relative flex gap-3 pb-4 last:pb-0">
            {!last && (
              <span
                className={cn(
                  "absolute left-[11px] top-6 bottom-0 w-px",
                  complete ? "bg-accent" : "bg-border"
                )}
                aria-hidden
              />
            )}
            <span
              className={cn(
                "relative z-10 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-[11px]",
                complete && "bg-accent border-accent text-accent-foreground",
                failed && "bg-destructive border-destructive text-destructive-foreground",
                current && "border-primary text-primary bg-card",
                !complete && !failed && !current && "border-border text-muted-foreground bg-card"
              )}
            >
              {complete ? (
                <Check className="h-3.5 w-3.5" />
              ) : failed ? (
                <X className="h-3.5 w-3.5" />
              ) : current && polling ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                i + 1
              )}
            </span>
            <div className="min-w-0 flex-1 -mt-0.5">
              <p
                className={cn(
                  "text-sm font-medium",
                  complete && "text-foreground",
                  current && "text-primary",
                  failed && "text-destructive",
                  !complete && !current && !failed && "text-muted-foreground"
                )}
              >
                {step.label}
                {step.key === "buyer" && isAwaiting && current && (
                  <span className="ml-2 text-xs font-normal text-warning">waiting for buyer…</span>
                )}
              </p>
              {failed && statusReason && <p className="text-xs text-destructive/80 mt-0.5">{statusReason.replace(/_/g, " ")}</p>}
              {notes?.[step.key] && <div className="mt-1 text-xs text-muted-foreground">{notes[step.key]}</div>}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
