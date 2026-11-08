import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { statusLabel } from "./sellerUtils";

const tone: Record<string, string> = {
  UPLOADED: "bg-muted text-muted-foreground border-border",
  EXTRACTED: "bg-muted text-muted-foreground border-border",
  AWAITING_BUYER: "bg-warning/15 text-warning border-warning/40",
  BUYER_CONFIRMED: "bg-accent/15 text-accent border-accent/40",
  BUYER_DISPUTED: "bg-destructive/15 text-destructive border-destructive/40",
  SCORED: "bg-accent/15 text-accent border-accent/40",
  DECIDED: "bg-warning/15 text-warning border-warning/40",
  OFFERED: "bg-primary/15 text-primary border-primary",
  ACCEPTED: "bg-primary/15 text-primary border-primary/40",
  FUNDED: "bg-accent/20 text-accent border-accent/50",
  PARTIALLY_PAID: "bg-accent/25 text-accent border-accent/60",
  PAID: "bg-accent text-accent-foreground border-accent",
  OVERDUE: "bg-destructive/15 text-destructive border-destructive/40",
  DECLINED: "bg-destructive/15 text-destructive border-destructive/40",
  CANCELLED: "bg-muted text-muted-foreground border-border",
};

/** Seller-facing status pill with plain-language labels. */
export function SellerStatusPill({ status, className }: { status?: string | null; className?: string }) {
  const s = (status ?? "").toUpperCase();
  return (
    <Badge variant="outline" className={cn("text-[11px] font-semibold whitespace-nowrap", tone[s] ?? "bg-muted text-muted-foreground border-border", className)}>
      {statusLabel(s)}
    </Badge>
  );
}
