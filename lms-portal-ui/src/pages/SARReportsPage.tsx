import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  FileWarning, ShieldCheck, ArrowUpRight, AlertTriangle, Plus,
  FileText, Send, CheckCircle, Clock, XCircle, Eye,
} from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fraudService, type SarReport, type SarStatus } from "@/services/fraudService";
import { useToast } from "@/hooks/use-toast";
import { formatKES } from "@/lib/format";

const STATUS_CONFIG: Record<SarStatus, { label: string; color: string; icon: React.ElementType }> = {
  DRAFT: { label: "Draft", color: "bg-muted text-muted-foreground", icon: FileText },
  PENDING_REVIEW: { label: "Pending Review", color: "bg-warning/15 text-warning border-warning/30", icon: Clock },
  APPROVED: { label: "Approved", color: "bg-info/15 text-info border-info/30", icon: CheckCircle },
  FILED: { label: "Filed", color: "bg-success/15 text-success border-success/30", icon: Send },
  REJECTED: { label: "Rejected", color: "bg-destructive/15 text-destructive border-destructive/30", icon: XCircle },
};

const SARReportsPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [typeFilter, setTypeFilter] = useState<string>("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [detailReport, setDetailReport] = useState<SarReport | null>(null);

  // Form state
  const [formData, setFormData] = useState({
    reportType: "SAR",
    subjectCustomerId: "",
    subjectName: "",
    subjectNationalId: "",
    narrative: "",
    suspiciousAmount: "",
    preparedBy: "",
  });

  const { data: reportsPage, isLoading } = useQuery({
    queryKey: ["fraud", "sar-reports", statusFilter, typeFilter],
    queryFn: () => fraudService.listSarReports(
      0, 50,
      statusFilter !== "all" ? statusFilter as SarStatus : undefined,
      typeFilter !== "all" ? typeFilter as "SAR" | "CTR" : undefined,
    ),
    staleTime: 30_000,
    retry: false,
  });

  const createMutation = useMutation({
    mutationFn: () => fraudService.createSarReport({
      reportType: formData.reportType,
      subjectCustomerId: formData.subjectCustomerId || undefined,
      subjectName: formData.subjectName || undefined,
      subjectNationalId: formData.subjectNationalId || undefined,
      narrative: formData.narrative || undefined,
      suspiciousAmount: formData.suspiciousAmount ? Number(formData.suspiciousAmount) : undefined,
      preparedBy: formData.preparedBy || undefined,
    }),
    onSuccess: (report) => {
      toast({ title: "Report created", description: `${report.reportNumber} created as draft` });
      queryClient.invalidateQueries({ queryKey: ["fraud", "sar-reports"] });
      setCreateOpen(false);
      setFormData({ reportType: "SAR", subjectCustomerId: "", subjectName: "", subjectNationalId: "", narrative: "", suspiciousAmount: "", preparedBy: "" });
    },
    onError: (e: Error) => toast({ title: "Failed", description: e.message, variant: "destructive" }),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Record<string, unknown> }) =>
      fraudService.updateSarReport(id, data),
    onSuccess: (report) => {
      toast({ title: "Report updated", description: `${report.reportNumber} status: ${report.status}` });
      queryClient.invalidateQueries({ queryKey: ["fraud", "sar-reports"] });
      setDetailReport(report);
    },
    onError: (e: Error) => toast({ title: "Update failed", description: e.message, variant: "destructive" }),
  });

  const reports = reportsPage?.content ?? [];

  const draftCount = reports.filter(r => r.status === "DRAFT").length;
  const pendingCount = reports.filter(r => r.status === "PENDING_REVIEW").length;
  const filedCount = reports.filter(r => r.status === "FILED").length;

  return (
    <DashboardLayout
      title="SAR / CTR Reports"
      subtitle="Suspicious Activity Reports and Currency Transaction Reports — filing & lifecycle management"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "SAR / CTR Reports" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Status Bar */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Badge className="bg-success/10 text-success border-success/20 gap-1.5 px-3 py-1">
              <span className="h-1.5 w-1.5 rounded-full bg-success inline-block animate-pulse" />
              Monitoring Active
            </Badge>
            <span className="text-xs text-muted-foreground">
              CTR threshold: KES 1,000,000 | SAR deadline: 7 business days | Regulator: FRC Kenya
            </span>
          </div>
          <Button size="sm" className="h-8 text-xs" onClick={() => setCreateOpen(true)}>
            <Plus className="h-3.5 w-3.5 mr-1" /> New Report
          </Button>
        </div>

        {/* KPIs */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground">Total Reports</span>
                <FileWarning className="h-4 w-4 text-muted-foreground" />
              </div>
              <p className="text-2xl font-heading">{reportsPage?.totalElements ?? 0}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground">Drafts</span>
                <FileText className="h-4 w-4 text-warning" />
              </div>
              <p className="text-2xl font-heading">{draftCount}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground">Pending Review</span>
                <Clock className="h-4 w-4 text-info" />
              </div>
              <p className="text-2xl font-heading">{pendingCount}</p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground">Filed</span>
                <CheckCircle className="h-4 w-4 text-success" />
              </div>
              <p className="text-2xl font-heading">{filedCount}</p>
            </CardContent>
          </Card>
        </div>

        {/* Reports Table */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium">Reports</CardTitle>
              <div className="flex gap-2">
                <Select value={typeFilter} onValueChange={setTypeFilter}>
                  <SelectTrigger className="h-8 w-[100px] text-xs"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Types</SelectItem>
                    <SelectItem value="SAR">SAR</SelectItem>
                    <SelectItem value="CTR">CTR</SelectItem>
                  </SelectContent>
                </Select>
                <Select value={statusFilter} onValueChange={setStatusFilter}>
                  <SelectTrigger className="h-8 w-[140px] text-xs"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Status</SelectItem>
                    <SelectItem value="DRAFT">Draft</SelectItem>
                    <SelectItem value="PENDING_REVIEW">Pending Review</SelectItem>
                    <SelectItem value="APPROVED">Approved</SelectItem>
                    <SelectItem value="FILED">Filed</SelectItem>
                    <SelectItem value="REJECTED">Rejected</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : reports.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <FileWarning className="h-8 w-8 mb-2 text-muted-foreground/50" />
                <p className="text-sm font-medium">No reports found</p>
                <p className="text-xs mt-1">Create a new SAR or CTR report to get started</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Report #</TableHead>
                    <TableHead className="text-xs">Type</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Subject</TableHead>
                    <TableHead className="text-xs">Amount</TableHead>
                    <TableHead className="text-xs">Prepared By</TableHead>
                    <TableHead className="text-xs">Deadline</TableHead>
                    <TableHead className="text-xs">Created</TableHead>
                    <TableHead className="text-xs"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {reports.map((r) => {
                    const sc = STATUS_CONFIG[r.status];
                    return (
                      <TableRow key={r.id} className="table-row-hover">
                        <TableCell className="text-xs font-mono font-medium">{r.reportNumber}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[11px] ${r.reportType === "CTR" ? "bg-info/15 text-info border-info/30" : "bg-warning/15 text-warning border-warning/30"}`}>
                            {r.reportType}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[11px] ${sc.color}`}>{sc.label}</Badge>
                        </TableCell>
                        <TableCell className="text-xs">
                          {r.subjectName || r.subjectCustomerId || "—"}
                        </TableCell>
                        <TableCell className="text-xs">{r.suspiciousAmount ? formatKES(r.suspiciousAmount) : "—"}</TableCell>
                        <TableCell className="text-xs">{r.preparedBy ?? "—"}</TableCell>
                        <TableCell className="text-xs whitespace-nowrap">
                          {r.filingDeadline ? r.filingDeadline.split("T")[0] : "—"}
                        </TableCell>
                        <TableCell className="text-xs whitespace-nowrap">{r.createdAt?.split("T")[0]}</TableCell>
                        <TableCell>
                          <Button variant="ghost" size="sm" className="h-7 text-xs" onClick={() => setDetailReport(r)}>
                            <Eye className="h-3 w-3" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Create Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Create New Report</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-xs text-muted-foreground">Report Type</label>
                <Select value={formData.reportType} onValueChange={(v) => setFormData(p => ({ ...p, reportType: v }))}>
                  <SelectTrigger className="h-9 text-sm"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="SAR">SAR — Suspicious Activity</SelectItem>
                    <SelectItem value="CTR">CTR — Currency Transaction</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Prepared By</label>
                <Input className="h-9 text-sm" placeholder="Analyst name" value={formData.preparedBy}
                  onChange={(e) => setFormData(p => ({ ...p, preparedBy: e.target.value }))} />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-xs text-muted-foreground">Subject Customer ID</label>
                <Input className="h-9 text-sm" placeholder="CUST-XXX" value={formData.subjectCustomerId}
                  onChange={(e) => setFormData(p => ({ ...p, subjectCustomerId: e.target.value }))} />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Subject Name</label>
                <Input className="h-9 text-sm" placeholder="Full name" value={formData.subjectName}
                  onChange={(e) => setFormData(p => ({ ...p, subjectName: e.target.value }))} />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-xs text-muted-foreground">National ID</label>
                <Input className="h-9 text-sm" placeholder="ID number" value={formData.subjectNationalId}
                  onChange={(e) => setFormData(p => ({ ...p, subjectNationalId: e.target.value }))} />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Suspicious Amount (KES)</label>
                <Input className="h-9 text-sm" type="number" placeholder="0.00" value={formData.suspiciousAmount}
                  onChange={(e) => setFormData(p => ({ ...p, suspiciousAmount: e.target.value }))} />
              </div>
            </div>
            <div>
              <label className="text-xs text-muted-foreground">Narrative</label>
              <Textarea className="text-sm min-h-[100px]" placeholder="Describe the suspicious activity..."
                value={formData.narrative}
                onChange={(e) => setFormData(p => ({ ...p, narrative: e.target.value }))} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button onClick={() => createMutation.mutate()} disabled={createMutation.isPending}>
              {createMutation.isPending ? "Creating..." : "Create Draft"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Detail Dialog */}
      <Dialog open={!!detailReport} onOpenChange={() => setDetailReport(null)}>
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
          {detailReport && (
            <>
              <DialogHeader>
                <DialogTitle className="flex items-center gap-3">
                  <span>{detailReport.reportNumber}</span>
                  <Badge variant="outline" className={`text-[11px] ${STATUS_CONFIG[detailReport.status].color}`}>
                    {STATUS_CONFIG[detailReport.status].label}
                  </Badge>
                  <Badge variant="outline" className="text-[11px]">{detailReport.reportType}</Badge>
                </DialogTitle>
              </DialogHeader>
              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div><span className="text-xs text-muted-foreground block">Subject</span>{detailReport.subjectName || detailReport.subjectCustomerId || "—"}</div>
                  <div><span className="text-xs text-muted-foreground block">National ID</span>{detailReport.subjectNationalId || "—"}</div>
                  <div><span className="text-xs text-muted-foreground block">Amount</span>{detailReport.suspiciousAmount ? formatKES(detailReport.suspiciousAmount) : "—"}</div>
                  <div><span className="text-xs text-muted-foreground block">Regulator</span>{detailReport.regulator || "—"}</div>
                  <div><span className="text-xs text-muted-foreground block">Filing Deadline</span>{detailReport.filingDeadline?.split("T")[0] || "—"}</div>
                  <div><span className="text-xs text-muted-foreground block">Prepared By</span>{detailReport.preparedBy || "—"}</div>
                  {detailReport.reviewedBy && <div><span className="text-xs text-muted-foreground block">Reviewed By</span>{detailReport.reviewedBy}</div>}
                  {detailReport.filedBy && <div><span className="text-xs text-muted-foreground block">Filed By</span>{detailReport.filedBy}</div>}
                  {detailReport.filingReference && <div><span className="text-xs text-muted-foreground block">Filing Reference</span>{detailReport.filingReference}</div>}
                </div>

                {detailReport.narrative && (
                  <div>
                    <span className="text-xs text-muted-foreground block mb-1">Narrative</span>
                    <p className="text-sm bg-muted/50 p-3 rounded-md whitespace-pre-wrap">{detailReport.narrative}</p>
                  </div>
                )}

                {/* Workflow Actions */}
                {detailReport.status !== "FILED" && detailReport.status !== "REJECTED" && (
                  <div className="flex gap-2 pt-2 border-t">
                    {detailReport.status === "DRAFT" && (
                      <Button size="sm" variant="outline" className="text-xs"
                        onClick={() => updateMutation.mutate({ id: detailReport.id, data: { status: "PENDING_REVIEW", reviewedBy: "current-user" } })}
                        disabled={updateMutation.isPending}>
                        <ArrowUpRight className="h-3 w-3 mr-1" /> Submit for Review
                      </Button>
                    )}
                    {detailReport.status === "PENDING_REVIEW" && (
                      <>
                        <Button size="sm" className="text-xs"
                          onClick={() => updateMutation.mutate({ id: detailReport.id, data: { status: "APPROVED", reviewedBy: "compliance-officer" } })}
                          disabled={updateMutation.isPending}>
                          <CheckCircle className="h-3 w-3 mr-1" /> Approve
                        </Button>
                        <Button size="sm" variant="destructive" className="text-xs"
                          onClick={() => updateMutation.mutate({ id: detailReport.id, data: { status: "REJECTED", reviewedBy: "compliance-officer" } })}
                          disabled={updateMutation.isPending}>
                          <XCircle className="h-3 w-3 mr-1" /> Reject
                        </Button>
                      </>
                    )}
                    {detailReport.status === "APPROVED" && (
                      <Button size="sm" className="text-xs bg-success hover:bg-success/90"
                        onClick={() => {
                          const ref = prompt("Enter filing reference number:");
                          if (ref) {
                            updateMutation.mutate({ id: detailReport.id, data: { status: "FILED", filedBy: "compliance-officer", filingReference: ref } });
                          }
                        }}
                        disabled={updateMutation.isPending}>
                        <Send className="h-3 w-3 mr-1" /> File with Regulator
                      </Button>
                    )}
                  </div>
                )}
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default SARReportsPage;
