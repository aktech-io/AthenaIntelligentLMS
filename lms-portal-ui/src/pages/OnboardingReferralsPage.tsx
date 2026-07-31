import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScanFace, Eye, Check, X } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  onboardingService,
  type OnboardingApplication,
  type OnboardingStatus,
} from "@/services/onboardingService";
import { useToast } from "@/hooks/use-toast";

const STATUS_CONFIG: Record<string, { label: string; color: string }> = {
  REFERRED: { label: "Referred", color: "bg-warning/15 text-warning border-warning/30" },
  APPROVED: { label: "Approved", color: "bg-success/15 text-success border-success/30" },
  AUTO_APPROVED: { label: "Auto-Approved", color: "bg-success/15 text-success border-success/30" },
  REJECTED: { label: "Rejected", color: "bg-destructive/15 text-destructive border-destructive/30" },
};

const RISK_TIER_CONFIG: Record<string, { label: string; color: string }> = {
  LOW: { label: "Low", color: "bg-success/15 text-success border-success/30" },
  MEDIUM: { label: "Medium", color: "bg-warning/15 text-warning border-warning/30" },
  HIGH: { label: "High", color: "bg-destructive/15 text-destructive border-destructive/30" },
};

const DOCUMENT_TYPE_LABELS: Record<string, string> = {
  NATIONAL_ID: "National ID",
  PASSPORT: "Passport",
};

type StatusFilter = OnboardingStatus | "ALL";

const OnboardingReferralsPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("REFERRED");
  const [selected, setSelected] = useState<OnboardingApplication | null>(null);
  const [reason, setReason] = useState("");

  const { data: applicationsPage, isLoading } = useQuery({
    queryKey: ["onboarding", "applications", statusFilter],
    queryFn: () => onboardingService.listApplications(
      statusFilter === "ALL" ? undefined : statusFilter, 0, 50,
    ),
    staleTime: 30_000,
    retry: false,
  });

  const decideMutation = useMutation({
    mutationFn: ({ app, action }: { app: OnboardingApplication; action: "approve" | "reject" }) =>
      action === "approve"
        ? onboardingService.approveApplication(app.id, app.tenantId, reason.trim())
        : onboardingService.rejectApplication(app.id, app.tenantId, reason.trim()),
    onSuccess: (_data, { action }) => {
      toast({
        title: action === "approve" ? "Application approved" : "Application rejected",
        description: `Onboarding application ${action === "approve" ? "approved" : "rejected"}`,
      });
      queryClient.invalidateQueries({ queryKey: ["onboarding", "applications"] });
      setSelected(null);
      setReason("");
    },
    onError: (e: Error) => toast({ title: "Failed", description: e.message, variant: "destructive" }),
  });

  const applications = applicationsPage?.content ?? [];
  const decisionReasons = selected?.decisionReasons
    ? selected.decisionReasons.split("; ").filter((r) => r.length > 0)
    : [];
  const isDecided = selected != null && selected.status !== "REFERRED";

  const openDetails = (app: OnboardingApplication) => {
    setSelected(app);
    setReason("");
  };

  return (
    <DashboardLayout
      title="Onboarding Referrals"
      subtitle="Review mobile eKYC applications referred for manual compliance decision"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "Onboarding Referrals" }]}
    >
      <div className="space-y-6 animate-fade-in">
        <div className="flex items-center justify-between">
          <Tabs value={statusFilter} onValueChange={(v) => setStatusFilter(v as StatusFilter)}>
            <TabsList className="h-8">
              <TabsTrigger value="REFERRED" className="text-xs">Referred</TabsTrigger>
              <TabsTrigger value="APPROVED" className="text-xs">Approved</TabsTrigger>
              <TabsTrigger value="REJECTED" className="text-xs">Rejected</TabsTrigger>
              <TabsTrigger value="ALL" className="text-xs">All</TabsTrigger>
            </TabsList>
          </Tabs>
          <span className="text-xs text-muted-foreground">
            {applicationsPage?.totalElements ?? 0} applications | All tenants
          </span>
        </div>

        {/* Applications Table */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">
              {statusFilter === "ALL" ? "All" : STATUS_CONFIG[statusFilter]?.label} Applications
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : applications.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <ScanFace className="h-8 w-8 mb-2 text-muted-foreground/50" />
                <p className="text-sm font-medium">No onboarding applications</p>
                <p className="text-xs mt-1">
                  {statusFilter === "REFERRED"
                    ? "Nothing waiting for manual review"
                    : "No applications match this filter"}
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">ID</TableHead>
                    <TableHead className="text-xs">Full Name</TableHead>
                    <TableHead className="text-xs">Phone</TableHead>
                    <TableHead className="text-xs">Document</TableHead>
                    <TableHead className="text-xs">ID / Number</TableHead>
                    <TableHead className="text-xs">Risk Tier</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Submitted</TableHead>
                    <TableHead className="text-xs"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {applications.map((a) => {
                    const statusCfg = STATUS_CONFIG[a.status] ?? { label: a.status, color: "bg-muted text-muted-foreground" };
                    const riskCfg = a.riskTier ? RISK_TIER_CONFIG[a.riskTier] : null;
                    return (
                      <TableRow key={a.id} className="table-row-hover">
                        <TableCell className="text-xs font-mono">{a.id.slice(0, 8)}</TableCell>
                        <TableCell className="text-xs font-medium">{a.fullName || "—"}</TableCell>
                        <TableCell className="text-xs">{a.phone || "—"}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className="text-[10px]">
                            {DOCUMENT_TYPE_LABELS[a.documentType] ?? a.documentType}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs font-mono">{a.nationalId || "—"}</TableCell>
                        <TableCell>
                          {riskCfg ? (
                            <Badge variant="outline" className={`text-[10px] ${riskCfg.color}`}>{riskCfg.label}</Badge>
                          ) : (
                            <span className="text-xs text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[10px] ${statusCfg.color}`}>{statusCfg.label}</Badge>
                        </TableCell>
                        <TableCell className="text-xs whitespace-nowrap">{a.createdAt?.split("T")[0] ?? "—"}</TableCell>
                        <TableCell>
                          <Button variant="ghost" size="sm" className="h-7 text-xs"
                            onClick={() => openDetails(a)}>
                            <Eye className="h-3 w-3 mr-1" /> Details
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

      {/* Details Dialog */}
      <Dialog open={selected != null} onOpenChange={(open) => { if (!open) { setSelected(null); setReason(""); } }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Onboarding Application</DialogTitle>
          </DialogHeader>
          {selected && (
            <div className="space-y-4">
              <div className="flex items-center gap-2">
                <Badge variant="outline" className={`text-[10px] ${STATUS_CONFIG[selected.status]?.color ?? "bg-muted text-muted-foreground"}`}>
                  {STATUS_CONFIG[selected.status]?.label ?? selected.status}
                </Badge>
                {selected.riskTier && RISK_TIER_CONFIG[selected.riskTier] && (
                  <Badge variant="outline" className={`text-[10px] ${RISK_TIER_CONFIG[selected.riskTier].color}`}>
                    {RISK_TIER_CONFIG[selected.riskTier].label} Risk
                  </Badge>
                )}
              </div>

              <div className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
                <div>
                  <p className="text-xs text-muted-foreground">Application ID</p>
                  <p className="text-xs font-mono break-all">{selected.id}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Tenant</p>
                  <p className="text-xs font-mono">{selected.tenantId}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Full Name</p>
                  <p className="text-xs font-medium">{selected.fullName || "—"}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Phone</p>
                  <p className="text-xs">{selected.phone || "—"}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Document</p>
                  <p className="text-xs">{DOCUMENT_TYPE_LABELS[selected.documentType] ?? selected.documentType}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">ID / Number</p>
                  <p className="text-xs font-mono">{selected.nationalId || "—"}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Date of Birth</p>
                  <p className="text-xs">{selected.dateOfBirth || "—"}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Provider</p>
                  <p className="text-xs">{selected.provider || "—"}</p>
                </div>
                {(selected.livenessMode || selected.livenessScore != null) && (
                  <div>
                    <p className="text-xs text-muted-foreground">Liveness</p>
                    <p className="text-xs">
                      {selected.livenessScore != null ? selected.livenessScore.toFixed(2) : "—"}
                      {selected.livenessMode && ` (${selected.livenessMode}${selected.livenessProvider ? `, ${selected.livenessProvider}` : ""})`}
                    </p>
                  </div>
                )}
                <div>
                  <p className="text-xs text-muted-foreground">Document Ref</p>
                  <p className="text-xs font-mono break-all">{selected.documentRef || "—"}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Selfie Ref</p>
                  <p className="text-xs font-mono break-all">{selected.selfieRef || "—"}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Customer ID</p>
                  <p className="text-xs font-mono break-all">{selected.customerId || "—"}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Submitted</p>
                  <p className="text-xs">{selected.createdAt ? new Date(selected.createdAt).toLocaleString() : "—"}</p>
                </div>
              </div>

              <div>
                <p className="text-xs text-muted-foreground mb-1">Decision Reasons</p>
                {decisionReasons.length > 0 ? (
                  <ul className="space-y-1">
                    {decisionReasons.map((r, i) => (
                      <li key={i} className="text-xs bg-muted/50 rounded px-2 py-1.5">{r}</li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-xs text-muted-foreground">—</p>
                )}
              </div>

              {isDecided ? (
                <div className="grid grid-cols-2 gap-x-4 gap-y-3 border-t pt-3">
                  <div>
                    <p className="text-xs text-muted-foreground">Decided By</p>
                    <p className="text-xs font-medium">{selected.decidedBy || "—"}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Decided At</p>
                    <p className="text-xs">{selected.decidedAt ? new Date(selected.decidedAt).toLocaleString() : "—"}</p>
                  </div>
                </div>
              ) : (
                <div className="border-t pt-3">
                  <label className="text-xs text-muted-foreground">Decision Reason</label>
                  <Textarea className="text-sm mt-1" placeholder="Reason for approving or rejecting this application (required)"
                    value={reason} onChange={(e) => setReason(e.target.value)} />
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            {isDecided ? (
              <Button variant="outline" onClick={() => setSelected(null)}>Close</Button>
            ) : (
              <>
                <Button variant="outline" onClick={() => { setSelected(null); setReason(""); }}>Cancel</Button>
                <Button variant="destructive"
                  disabled={reason.trim().length === 0 || decideMutation.isPending}
                  onClick={() => selected && decideMutation.mutate({ app: selected, action: "reject" })}>
                  <X className="h-3.5 w-3.5 mr-1" />
                  {decideMutation.isPending && decideMutation.variables?.action === "reject" ? "Rejecting..." : "Reject"}
                </Button>
                <Button
                  disabled={reason.trim().length === 0 || decideMutation.isPending}
                  onClick={() => selected && decideMutation.mutate({ app: selected, action: "approve" })}>
                  <Check className="h-3.5 w-3.5 mr-1" />
                  {decideMutation.isPending && decideMutation.variables?.action === "approve" ? "Approving..." : "Approve"}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default OnboardingReferralsPage;
