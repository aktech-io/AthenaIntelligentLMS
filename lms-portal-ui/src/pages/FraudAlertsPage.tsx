import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import {
  ShieldAlert, AlertTriangle, Search, CheckCircle2, XCircle,
  UserCheck, ArrowUpRight, Filter, Clock,
} from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fraudService, type FraudAlert, type AlertStatus, type AlertSeverity } from "@/services/fraudService";
import { formatKES } from "@/lib/format";
import { useToast } from "@/hooks/use-toast";

const severityColor: Record<AlertSeverity, string> = {
  CRITICAL: "bg-red-500/15 text-red-600 border-red-500/30",
  HIGH: "bg-destructive/15 text-destructive border-destructive/30",
  MEDIUM: "bg-warning/15 text-warning border-warning/30",
  LOW: "bg-info/15 text-info border-info/30",
};

const statusColor: Record<string, string> = {
  OPEN: "bg-destructive/15 text-destructive border-destructive/30",
  UNDER_REVIEW: "bg-warning/15 text-warning border-warning/30",
  ESCALATED: "bg-red-500/15 text-red-600 border-red-500/30",
  CONFIRMED_FRAUD: "bg-red-600/15 text-red-700 border-red-600/30",
  FALSE_POSITIVE: "bg-success/15 text-success border-success/30",
  CLOSED: "bg-muted text-muted-foreground border-muted",
};

const FraudAlertsPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedAlert, setSelectedAlert] = useState<FraudAlert | null>(null);
  const [resolveDialogOpen, setResolveDialogOpen] = useState(false);
  const [resolveNotes, setResolveNotes] = useState("");
  const [resolveAsFraud, setResolveAsFraud] = useState(false);
  const [page, setPage] = useState(0);

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ["fraud", "summary"],
    queryFn: () => fraudService.getSummary(),
    staleTime: 30_000,
    retry: false,
  });

  const queryStatus = statusFilter !== "all" ? statusFilter as AlertStatus : undefined;
  const { data: alertsPage, isLoading: alertsLoading } = useQuery({
    queryKey: ["fraud", "alerts", page, queryStatus],
    queryFn: () => fraudService.listAlerts(page, 25, queryStatus),
    staleTime: 30_000,
    retry: false,
  });

  const alerts = alertsPage?.content ?? [];
  const filtered = alerts.filter((a) => {
    if (!searchQuery) return true;
    const q = searchQuery.toLowerCase();
    return (
      a.alertType?.toLowerCase().includes(q) ||
      a.customerId?.toLowerCase().includes(q) ||
      a.description?.toLowerCase().includes(q) ||
      a.ruleCode?.toLowerCase().includes(q) ||
      a.subjectId?.toLowerCase().includes(q)
    );
  });

  const resolveMutation = useMutation({
    mutationFn: (params: { id: string; confirmedFraud: boolean; notes: string }) =>
      fraudService.resolveAlert(params.id, {
        resolvedBy: "admin",
        confirmedFraud: params.confirmedFraud,
        notes: params.notes,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fraud"] });
      setResolveDialogOpen(false);
      setSelectedAlert(null);
      setResolveNotes("");
      toast({ title: "Alert resolved successfully" });
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const assignMutation = useMutation({
    mutationFn: (params: { id: string; assignee: string }) =>
      fraudService.assignAlert(params.id, { assignee: params.assignee }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fraud"] });
      toast({ title: "Alert assigned" });
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const openResolveDialog = (alert: FraudAlert, asFraud: boolean) => {
    setSelectedAlert(alert);
    setResolveAsFraud(asFraud);
    setResolveDialogOpen(true);
  };

  return (
    <DashboardLayout
      title="Fraud Alerts"
      subtitle="Real-time fraud detection, investigation, and case management"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "Fraud Alerts" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Status indicator */}
        <div className="flex items-center gap-3">
          <Badge className="bg-success/10 text-success border-success/20 gap-1.5 px-3 py-1">
            <span className="h-1.5 w-1.5 rounded-full bg-success inline-block animate-pulse" />
            Engine Active
          </Badge>
          <span className="text-xs text-muted-foreground">
            Fraud detection engine monitoring 25 event types with 20 configurable rules
          </span>
        </div>

        {/* KPI Cards */}
        {summaryLoading ? (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-24 w-full" />)}
          </div>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Open Alerts</span>
                  <ShieldAlert className="h-4 w-4 text-destructive" />
                </div>
                <p className="text-2xl font-heading">{summary?.openAlerts ?? 0}</p>
                <p className="text-[10px] text-muted-foreground mt-1">
                  {summary?.criticalAlerts ?? 0} critical
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Under Review</span>
                  <Clock className="h-4 w-4 text-warning" />
                </div>
                <p className="text-2xl font-heading">{summary?.underReviewAlerts ?? 0}</p>
                <p className="text-[10px] text-muted-foreground mt-1">
                  {summary?.escalatedAlerts ?? 0} escalated
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Confirmed Fraud</span>
                  <AlertTriangle className="h-4 w-4 text-red-600" />
                </div>
                <p className="text-2xl font-heading">{summary?.confirmedFraud ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">High Risk Customers</span>
                  <UserCheck className="h-4 w-4 text-info" />
                </div>
                <p className="text-2xl font-heading">{summary?.highRiskCustomers ?? 0}</p>
                <p className="text-[10px] text-muted-foreground mt-1">
                  {summary?.criticalRiskCustomers ?? 0} critical
                </p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Filters */}
        <div className="flex flex-col sm:flex-row gap-3">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search by customer, rule, type, description..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 h-9 text-sm"
            />
          </div>
          <div className="flex gap-2">
            <Select value={statusFilter} onValueChange={(v) => { setStatusFilter(v); setPage(0); }}>
              <SelectTrigger className="h-9 w-[160px] text-xs">
                <Filter className="h-3.5 w-3.5 mr-1.5" />
                <SelectValue placeholder="All Statuses" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Statuses</SelectItem>
                <SelectItem value="OPEN">Open</SelectItem>
                <SelectItem value="UNDER_REVIEW">Under Review</SelectItem>
                <SelectItem value="ESCALATED">Escalated</SelectItem>
                <SelectItem value="CONFIRMED_FRAUD">Confirmed Fraud</SelectItem>
                <SelectItem value="FALSE_POSITIVE">False Positive</SelectItem>
                <SelectItem value="CLOSED">Closed</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Alerts Table */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">
              Fraud Alerts
              {alertsPage && (
                <span className="text-muted-foreground font-normal ml-2">
                  ({alertsPage.totalElements} total)
                </span>
              )}
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {alertsLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : filtered.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-40 text-muted-foreground">
                <ShieldAlert className="h-10 w-10 mb-3 opacity-30" />
                <p className="text-sm font-medium">No fraud alerts found</p>
                <p className="text-xs mt-1">Real-time fraud screening is active and monitoring events</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="text-xs w-[80px]">ID</TableHead>
                      <TableHead className="text-xs">Type</TableHead>
                      <TableHead className="text-xs">Rule</TableHead>
                      <TableHead className="text-xs">Customer</TableHead>
                      <TableHead className="text-xs">Severity</TableHead>
                      <TableHead className="text-xs">Amount</TableHead>
                      <TableHead className="text-xs max-w-[250px]">Description</TableHead>
                      <TableHead className="text-xs">Status</TableHead>
                      <TableHead className="text-xs">Date</TableHead>
                      <TableHead className="text-xs text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filtered.map((a) => (
                      <TableRow key={a.id} className="table-row-hover">
                        <TableCell className="text-xs font-mono">{a.id?.slice(0, 8)}</TableCell>
                        <TableCell>
                          <span className="text-xs">{a.alertType?.replace(/_/g, " ")}</span>
                          {a.escalatedToCompliance && (
                            <ArrowUpRight className="h-3 w-3 text-red-500 inline ml-1" title="Escalated to compliance" />
                          )}
                        </TableCell>
                        <TableCell className="text-xs font-mono text-muted-foreground">
                          {a.ruleCode ?? "—"}
                        </TableCell>
                        <TableCell className="text-xs font-mono">{a.customerId ?? "—"}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[10px] ${severityColor[a.severity]}`}>
                            {a.severity}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs">
                          {a.triggerAmount ? formatKES(a.triggerAmount) : "—"}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground max-w-[250px] truncate" title={a.description}>
                          {a.description}
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[10px] ${statusColor[a.status] ?? ""}`}>
                            {a.status?.replace(/_/g, " ")}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs whitespace-nowrap">
                          {a.createdAt?.split("T")[0] ?? "—"}
                        </TableCell>
                        <TableCell className="text-right">
                          {(a.status === "OPEN" || a.status === "UNDER_REVIEW") && (
                            <div className="flex items-center gap-1 justify-end">
                              {a.status === "OPEN" && (
                                <Button
                                  variant="ghost" size="sm"
                                  className="h-7 px-2 text-xs"
                                  onClick={() => assignMutation.mutate({ id: a.id, assignee: "admin" })}
                                  title="Assign to me"
                                >
                                  <UserCheck className="h-3.5 w-3.5" />
                                </Button>
                              )}
                              <Button
                                variant="ghost" size="sm"
                                className="h-7 px-2 text-xs text-destructive"
                                onClick={() => openResolveDialog(a, true)}
                                title="Confirm fraud"
                              >
                                <XCircle className="h-3.5 w-3.5" />
                              </Button>
                              <Button
                                variant="ghost" size="sm"
                                className="h-7 px-2 text-xs text-success"
                                onClick={() => openResolveDialog(a, false)}
                                title="Mark as false positive"
                              >
                                <CheckCircle2 className="h-3.5 w-3.5" />
                              </Button>
                            </div>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}

            {/* Pagination */}
            {alertsPage && alertsPage.totalPages > 1 && (
              <div className="flex items-center justify-between px-4 py-3 border-t">
                <span className="text-xs text-muted-foreground">
                  Page {alertsPage.number + 1} of {alertsPage.totalPages}
                </span>
                <div className="flex gap-2">
                  <Button
                    variant="outline" size="sm" className="h-7 text-xs"
                    disabled={alertsPage.first}
                    onClick={() => setPage((p) => Math.max(0, p - 1))}
                  >
                    Previous
                  </Button>
                  <Button
                    variant="outline" size="sm" className="h-7 text-xs"
                    disabled={alertsPage.last}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Resolve Dialog */}
      <Dialog open={resolveDialogOpen} onOpenChange={setResolveDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-sm">
              {resolveAsFraud ? "Confirm Fraud" : "Mark as False Positive"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            {selectedAlert && (
              <div className="rounded-md bg-muted p-3 space-y-1">
                <p className="text-xs font-medium">{selectedAlert.alertType?.replace(/_/g, " ")}</p>
                <p className="text-xs text-muted-foreground">{selectedAlert.description}</p>
                {selectedAlert.triggerAmount && (
                  <p className="text-xs">Amount: {formatKES(selectedAlert.triggerAmount)}</p>
                )}
              </div>
            )}
            <Textarea
              placeholder="Resolution notes..."
              value={resolveNotes}
              onChange={(e) => setResolveNotes(e.target.value)}
              rows={3}
              className="text-sm"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setResolveDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              variant={resolveAsFraud ? "destructive" : "default"}
              disabled={resolveMutation.isPending}
              onClick={() => {
                if (selectedAlert) {
                  resolveMutation.mutate({
                    id: selectedAlert.id,
                    confirmedFraud: resolveAsFraud,
                    notes: resolveNotes,
                  });
                }
              }}
            >
              {resolveMutation.isPending ? "Saving..." : resolveAsFraud ? "Confirm Fraud" : "False Positive"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default FraudAlertsPage;
