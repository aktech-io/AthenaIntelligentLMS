import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type LoanStatus = "pending" | "approved" | "disbursed" | "active" | "overdue" | "closed" | "rejected";

const statusConfig: Record<LoanStatus, { label: string; className: string }> = {
  pending: { label: "Pending", className: "bg-warning/15 text-warning border-warning/30" },
  approved: { label: "Approved", className: "bg-info/15 text-info border-info/30" },
  disbursed: { label: "Disbursed", className: "bg-accent/15 text-accent-foreground border-accent/30" },
  active: { label: "Active", className: "bg-success/15 text-success border-success/30" },
  overdue: { label: "Overdue", className: "bg-destructive/15 text-destructive border-destructive/30" },
  closed: { label: "Closed", className: "bg-muted text-muted-foreground border-border" },
  rejected: { label: "Rejected", className: "bg-destructive/10 text-destructive border-destructive/20" },
};

interface LoanStatusBadgeProps {
  status: LoanStatus;
}

export function LoanStatusBadge({ status }: LoanStatusBadgeProps) {
  const config = statusConfig[status];
  return (
    <Badge variant="outline" className={cn("text-[11px] font-semibold uppercase tracking-wider", config.className)}>
      {config.label}
    </Badge>
  );
}
