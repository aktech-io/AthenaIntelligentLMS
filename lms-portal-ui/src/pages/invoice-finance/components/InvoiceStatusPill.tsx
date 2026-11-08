import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { InvoiceStatus, DecisionOutcome } from "@/services/invoiceService";

type PillStatus = InvoiceStatus | DecisionOutcome | "REFER" | "FLAG" | string;

const styles: Record<string, string> = {
  UPLOADED: "bg-muted text-muted-foreground border-border",
  EXTRACTED: "bg-muted text-muted-foreground border-border",
  AWAITING_BUYER: "bg-warning/15 text-warning border-warning/40",
  BUYER_CONFIRMED: "bg-accent/15 text-accent border-accent/40",
  BUYER_DISPUTED: "bg-destructive/15 text-destructive border-destructive/40",
  SCORED: "bg-accent/15 text-accent border-accent/40",
  DECIDED: "bg-warning/15 text-warning border-warning/40",
  OFFERED: "bg-transparent text-primary border-primary",
  ACCEPTED: "bg-primary/15 text-primary border-primary/40",
  FUNDED: "bg-accent/20 text-accent border-accent/50",
  PARTIALLY_PAID: "bg-accent/25 text-accent border-accent/60",
  PAID: "bg-accent text-accent-foreground border-accent",
  OVERDUE: "bg-destructive/15 text-destructive border-destructive/40",
  DECLINED: "bg-destructive/15 text-destructive border-destructive/40",
  DISPUTED: "bg-destructive/15 text-destructive border-destructive/40",
  CANCELLED: "bg-muted text-muted-foreground border-border line-through",
  REFER: "bg-warning/15 text-warning border-warning/40",
  FLAG: "bg-warning/15 text-warning border-warning/40",
  APPROVE: "bg-accent/15 text-accent border-accent/40",
  DECLINE: "bg-destructive/15 text-destructive border-destructive/40",
  ACTIVE: "bg-accent/15 text-accent border-accent/40",
  EXPIRED: "bg-muted text-muted-foreground border-border",
  WITHDRAWN: "bg-muted text-muted-foreground border-border",
  DILUTED: "bg-warning/15 text-warning border-warning/40",
  WRITTEN_OFF: "bg-destructive/15 text-destructive border-destructive/40",
};

export function InvoiceStatusPill({ status, className }: { status?: PillStatus | null; className?: string }) {
  const s = (status ?? "UNKNOWN").toUpperCase();
  const label = s.replace(/_/g, " ");
  return (
    <Badge
      variant="outline"
      className={cn(
        "text-[11px] font-semibold uppercase tracking-wider whitespace-nowrap",
        styles[s] ?? "bg-muted text-muted-foreground border-border",
        className
      )}
    >
      {label}
    </Badge>
  );
}

export function TierPill({ tier }: { tier?: string | null }) {
  const t = (tier ?? "UNRATED").toUpperCase();
  const cls: Record<string, string> = {
    A: "bg-accent/20 text-accent border-accent/50",
    B: "bg-accent/10 text-accent border-accent/30",
    C: "bg-warning/15 text-warning border-warning/40",
    D: "bg-destructive/15 text-destructive border-destructive/40",
    UNRATED: "bg-muted text-muted-foreground border-border",
  };
  return (
    <Badge variant="outline" className={cn("text-[11px] font-mono font-semibold", cls[t] ?? cls.UNRATED)}>
      {t}
    </Badge>
  );
}
