import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { CheckCheck, XCircle, Check } from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  approvalService,
  type PendingApproval,
} from "@/services/approvalService";

const statusColors: Record<string, string> = {
  PENDING: "bg-amber-100 text-amber-700 border-amber-300",
  APPROVED: "bg-green-100 text-green-700 border-green-300",
  REJECTED: "bg-red-100 text-red-700 border-red-300",
};

function fmtCurrency(amount: number | undefined | null, currency = "KES"): string {
  if (amount == null) return "--";
  return `${currency} ${amount.toLocaleString("en", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

function fmtDateTime(d: string | undefined | null): string {
  if (!d) return "--";
  return d.replace("T", " ").split(".")[0];
}

function shortId(id: string | undefined | null): string {
  if (!id) return "--";
  return id.length > 10 ? `${id.slice(0, 8)}…` : id;
}

const ApprovalsPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const [status, setStatus] = useState<string>("PENDING");
  const [rejectTarget, setRejectTarget] = useState<PendingApproval | null>(null);
  const [rejectReason, setRejectReason] = useState("");

  const { data: approvals, isLoading } = useQuery({
    queryKey: ["pending-approvals", status],
    queryFn: () => approvalService.listPendingApprovals(status),
    retry: false,
  });

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["pending-approvals"] });

  const approveMutation = useMutation({
    mutationFn: (id: string) => approvalService.approvePending(id),
    onSuccess: () => {
      toast({ title: "Approved", description: "The operation has been approved and executed." });
      invalidate();
    },
    onError: (err: Error) => {
      toast({ title: "Approval Failed", description: err.message, variant: "destructive" });
    },
  });

  const rejectMutation = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      approvalService.rejectPending(id, reason),
    onSuccess: () => {
      toast({ title: "Rejected", description: "The operation has been rejected." });
      setRejectTarget(null);
      setRejectReason("");
      invalidate();
    },
    onError: (err: Error) => {
      toast({ title: "Reject Failed", description: err.message, variant: "destructive" });
    },
  });

  const rows: PendingApproval[] = approvals ?? [];

  return (
    <DashboardLayout
      title="Approvals"
      subtitle="Review and authorise sensitive operations awaiting a second authoriser"
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Approvals" },
      ]}
    >
      <div className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <Label className="text-xs text-muted-foreground font-sans">Status</Label>
            <Select value={status} onValueChange={setStatus}>
              <SelectTrigger className="w-[150px] h-8 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {["PENDING", "APPROVED", "REJECTED"].map((s) => (
                  <SelectItem key={s} value={s} className="text-xs">
                    {s}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        <Card>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : rows.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <CheckCheck className="h-8 w-8 mb-2 opacity-40" />
                <p className="text-sm font-medium">No approvals</p>
                <p className="text-xs mt-1">No {status.toLowerCase()} approvals found.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[10px] uppercase tracking-wider">Operation</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Entity</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider text-right">Amount</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Maker</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Status</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider">Created</TableHead>
                    <TableHead className="text-[10px] uppercase tracking-wider text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((a) => (
                    <TableRow key={a.id} className="table-row-hover">
                      <TableCell className="text-xs font-sans">
                        <Badge variant="outline" className="text-[9px]">
                          {a.operation}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs font-sans">
                        {a.entityType ? (
                          <span>
                            {a.entityType}
                            <span className="text-muted-foreground font-mono ml-1">
                              {shortId(a.entityId)}
                            </span>
                          </span>
                        ) : (
                          "--"
                        )}
                      </TableCell>
                      <TableCell className="text-xs font-mono text-right font-semibold">
                        {fmtCurrency(a.amount)}
                      </TableCell>
                      <TableCell className="text-xs font-sans">
                        {a.makerId ? (
                          <span>
                            {a.makerId}
                            {a.makerRole && (
                              <span className="text-muted-foreground ml-1">({a.makerRole})</span>
                            )}
                          </span>
                        ) : (
                          "--"
                        )}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={`text-[9px] ${
                            statusColors[a.status] ?? "bg-muted text-muted-foreground"
                          }`}
                        >
                          {a.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs font-sans text-muted-foreground">
                        {fmtDateTime(a.createdAt)}
                      </TableCell>
                      <TableCell className="text-right">
                        {a.status === "PENDING" ? (
                          <div className="flex items-center justify-end gap-1.5">
                            <Button
                              size="sm"
                              className="text-xs h-7 bg-green-600 hover:bg-green-700 text-white"
                              onClick={() => approveMutation.mutate(a.id)}
                              disabled={approveMutation.isPending}
                            >
                              <Check className="h-3.5 w-3.5 mr-1" /> Approve
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              className="text-xs h-7 text-destructive"
                              onClick={() => {
                                setRejectTarget(a);
                                setRejectReason("");
                              }}
                              disabled={rejectMutation.isPending}
                            >
                              <XCircle className="h-3.5 w-3.5 mr-1" /> Reject
                            </Button>
                          </div>
                        ) : (
                          <span className="text-[10px] text-muted-foreground font-sans">
                            {a.checkerId ? `by ${a.checkerId}` : "--"}
                          </span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Reject dialog */}
        <Dialog open={!!rejectTarget} onOpenChange={(open) => !open && setRejectTarget(null)}>
          <DialogContent className="sm:max-w-sm">
            <DialogHeader>
              <DialogTitle className="font-heading">Reject Approval</DialogTitle>
            </DialogHeader>
            <div className="space-y-3 py-1">
              <p className="text-xs text-muted-foreground font-sans">
                Provide a reason for rejecting this {rejectTarget?.operation} request.
              </p>
              <div>
                <Label className="text-xs">Reason</Label>
                <Textarea
                  className="mt-1 text-sm font-sans"
                  placeholder="Reason for rejection"
                  value={rejectReason}
                  onChange={(e) => setRejectReason(e.target.value)}
                  autoFocus
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                size="sm"
                className="text-xs"
                onClick={() => setRejectTarget(null)}
              >
                Cancel
              </Button>
              <Button
                size="sm"
                className="text-xs"
                variant="destructive"
                onClick={() =>
                  rejectTarget &&
                  rejectMutation.mutate({ id: rejectTarget.id, reason: rejectReason.trim() })
                }
                disabled={rejectMutation.isPending || !rejectReason.trim()}
              >
                {rejectMutation.isPending ? "Rejecting..." : "Confirm Reject"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  );
};

export default ApprovalsPage;
