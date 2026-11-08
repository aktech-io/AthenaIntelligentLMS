import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Inbox, ShieldAlert, Check, X, ExternalLink } from "lucide-react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useToast } from "@/hooks/use-toast";
import { invoiceService, type QueueItem } from "@/services/invoiceService";
import { formatMoney, formatDateTime } from "@/lib/format";
import { InvoiceStatusPill, TierPill } from "./components/InvoiceStatusPill";
import { LoadingState, ErrorState, EmptyState } from "./components/States";
import { ApproveDialog, RejectDialog } from "./components/ReviewDialogs";
import { useInvoiceRole } from "./components/useInvoiceRole";

const ReviewQueuePage = () => {
  const { isManager } = useInvoiceRole();
  const { toast } = useToast();
  const qc = useQueryClient();
  const q = useQuery<QueueItem[]>({ queryKey: ["if-queue"], queryFn: () => invoiceService.queue(), refetchInterval: 20_000 });
  const [approveFor, setApproveFor] = useState<QueueItem | null>(null);
  const [rejectFor, setRejectFor] = useState<QueueItem | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["if-queue"] });
    qc.invalidateQueries({ queryKey: ["if-invoices"] });
    qc.invalidateQueries({ queryKey: ["if-stats"] });
  };

  const approve = useMutation({
    mutationFn: ({ id, note }: { id: string; note: string }) => invoiceService.approve(id, note),
    onSuccess: () => {
      toast({ title: "Approved", description: "Invoice moved forward; an offer will be created." });
      setApproveFor(null);
      invalidate();
    },
    onError: (e: Error) => toast({ title: "Approve failed", description: e.message, variant: "destructive" }),
  });
  const reject = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => invoiceService.reject(id, reason),
    onSuccess: () => {
      toast({ title: "Rejected", description: "Invoice declined and the seller notified." });
      setRejectFor(null);
      invalidate();
    },
    onError: (e: Error) => toast({ title: "Reject failed", description: e.message, variant: "destructive" }),
  });

  const items = q.data ?? [];

  return (
    <DashboardLayout
      title="Review Queue"
      subtitle="Referred decisions and fraud flags awaiting a manager"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance", href: "/invoice-finance" }, { label: "Review Queue" }]}
    >
      <div className="space-y-4 animate-fade-in">
        {!isManager && items.length > 0 && (
          <p className="text-xs text-muted-foreground flex items-center gap-1.5">
            <ShieldAlert className="h-3.5 w-3.5" /> Approve / reject requires the MANAGER role. You can review evidence and open each invoice.
          </p>
        )}
        <Card>
          <CardContent className="p-0">
            {q.isLoading ? (
              <LoadingState label="Loading queue…" />
            ) : q.isError ? (
              <ErrorState error={q.error} onRetry={() => q.refetch()} />
            ) : items.length === 0 ? (
              <EmptyState icon={Inbox} title="Queue is clear" description="REFER decisions and fraud flags will appear here for a manager to action." />
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Invoice</TableHead>
                      <TableHead>Seller</TableHead>
                      <TableHead>Buyer</TableHead>
                      <TableHead className="text-right">Amount</TableHead>
                      <TableHead>Outcome</TableHead>
                      <TableHead className="min-w-[260px]">Reasons / evidence</TableHead>
                      <TableHead>Decided</TableHead>
                      <TableHead className="text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {items.map((it) => (
                      <TableRow key={it.invoiceId} className="table-row-hover">
                        <TableCell>
                          <Link to={`/invoice-finance/invoices/${it.invoiceId}`} className="font-medium hover:underline inline-flex items-center gap-1">
                            {it.invoiceNumber ?? it.invoiceId.slice(0, 8)}
                            <ExternalLink className="h-3 w-3 text-muted-foreground" />
                          </Link>
                          <div className="mt-1">
                            <InvoiceStatusPill status={it.status} />
                          </div>
                        </TableCell>
                        <TableCell className="text-sm">
                          <div>{it.sellerName ?? "—"}</div>
                          {it.sellerCustomerId && <div className="text-xs text-muted-foreground font-mono">{it.sellerCustomerId}</div>}
                        </TableCell>
                        <TableCell className="text-sm">
                          <div className="flex items-center gap-2">
                            <span>{it.buyerName ?? "—"}</span>
                            {it.buyerTier && <TierPill tier={it.buyerTier} />}
                          </div>
                        </TableCell>
                        <TableCell className="text-right tabular-nums font-mono text-sm">{formatMoney(it.total ?? 0, it.currency)}</TableCell>
                        <TableCell>
                          <InvoiceStatusPill status={it.outcome} />
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-wrap gap-1">
                            {it.reasons.map((r) => (
                              <Badge key={r} variant="outline" className="text-[11px] font-mono bg-warning/10 text-warning border-warning/30">
                                {r}
                              </Badge>
                            ))}
                            {it.fraudFlags.map((f) => (
                              <Badge key={f} variant="outline" className="text-[11px] font-mono bg-destructive/10 text-destructive border-destructive/30">
                                <ShieldAlert className="h-3 w-3 mr-1" /> {f}
                              </Badge>
                            ))}
                            {it.reasons.length === 0 && it.fraudFlags.length === 0 && <span className="text-xs text-muted-foreground">No reasons recorded</span>}
                          </div>
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{formatDateTime(it.decidedAt ?? it.createdAt)}</TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-1">
                            <Button size="sm" variant="outline" className="h-8 text-destructive border-destructive/30 hover:bg-destructive/10" disabled={!isManager} onClick={() => setRejectFor(it)}>
                              <X className="h-3.5 w-3.5 mr-1" /> Reject
                            </Button>
                            <Button size="sm" className="h-8" disabled={!isManager} onClick={() => setApproveFor(it)}>
                              <Check className="h-3.5 w-3.5 mr-1" /> Approve
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <ApproveDialog
        open={!!approveFor}
        onOpenChange={(o) => !o && setApproveFor(null)}
        title={approveFor ? `Approve ${approveFor.invoiceNumber ?? approveFor.invoiceId.slice(0, 8)}` : ""}
        pending={approve.isPending}
        onConfirm={(note) => approveFor && approve.mutate({ id: approveFor.invoiceId, note })}
      />
      <RejectDialog
        open={!!rejectFor}
        onOpenChange={(o) => !o && setRejectFor(null)}
        title={rejectFor ? `Reject ${rejectFor.invoiceNumber ?? rejectFor.invoiceId.slice(0, 8)}` : ""}
        pending={reject.isPending}
        onConfirm={(reason) => rejectFor && reject.mutate({ id: rejectFor.invoiceId, reason })}
      />
    </DashboardLayout>
  );
};

export default ReviewQueuePage;
