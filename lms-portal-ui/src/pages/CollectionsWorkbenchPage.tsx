import { useState, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  AlertTriangle,
  Users,
  Clock,
  ShieldAlert,
  Phone,
  Eye,
} from "lucide-react";
import {
  collectionsService,
  type CollectionCase,
  type AddActionRequest,
} from "@/services/collectionsService";
import { formatKES } from "@/lib/format";
import { useAuth } from "@/contexts/AuthContext";

const ACTION_TYPES = [
  "PHONE_CALL",
  "SMS",
  "EMAIL",
  "FIELD_VISIT",
  "LEGAL_NOTICE",
  "RESTRUCTURE_OFFER",
  "WRITE_OFF",
  "OTHER",
];

const OUTCOMES = [
  "CONTACTED",
  "NO_ANSWER",
  "PROMISE_RECEIVED",
  "REFUSED_TO_PAY",
  "PAYMENT_RECEIVED",
  "ESCALATED",
  "OTHER",
];

const stageBadge = (stage: string) => {
  switch (stage) {
    case "WATCH":
      return <Badge className="bg-yellow-100 text-yellow-700 border-yellow-200 text-[11px]">Watch</Badge>;
    case "SUBSTANDARD":
      return <Badge className="bg-orange-100 text-orange-700 border-orange-200 text-[11px]">Substandard</Badge>;
    case "DOUBTFUL":
      return <Badge className="bg-red-100 text-red-700 border-red-200 text-[11px]">Doubtful</Badge>;
    case "LOSS":
      return <Badge className="bg-red-200 text-red-900 border-red-300 text-[11px]">Loss</Badge>;
    default:
      return <Badge variant="outline" className="text-[11px]">{stage}</Badge>;
  }
};

const priorityBadge = (priority: string) => {
  switch (priority) {
    case "LOW":
      return <Badge variant="outline" className="bg-gray-100 text-gray-600 border-gray-200 text-[11px]">Low</Badge>;
    case "NORMAL":
      return <Badge variant="outline" className="bg-blue-100 text-blue-700 border-blue-200 text-[11px]">Normal</Badge>;
    case "HIGH":
      return <Badge variant="outline" className="bg-orange-100 text-orange-700 border-orange-200 text-[11px]">High</Badge>;
    case "CRITICAL":
      return <Badge variant="outline" className="bg-red-100 text-red-700 border-red-200 text-[11px]">Critical</Badge>;
    default:
      return <Badge variant="outline" className="text-[11px]">{priority}</Badge>;
  }
};

function isOverdue(c: CollectionCase): boolean {
  if (!c.lastActionAt) return true;
  const lastAction = new Date(c.lastActionAt);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  // Consider overdue if last action was more than 3 days ago for critical, 7 for others
  const daysSinceAction = Math.floor((today.getTime() - lastAction.getTime()) / (1000 * 60 * 60 * 24));
  return c.priority === "CRITICAL" ? daysSinceAction > 3 : daysSinceAction > 7;
}

const CollectionsWorkbenchPage = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user } = useAuth();
  const username = user?.name ?? user?.id ?? "";

  const [actionDialogOpen, setActionDialogOpen] = useState(false);
  const [selectedCaseId, setSelectedCaseId] = useState<string | null>(null);
  const [actionForm, setActionForm] = useState<AddActionRequest>({
    actionType: "PHONE_CALL",
    outcome: "",
    notes: "",
    nextActionDate: "",
  });

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ["collections-summary"],
    queryFn: () => collectionsService.getSummary(),
    staleTime: 30_000,
    retry: false,
  });

  const { data: casesPage, isLoading: casesLoading } = useQuery({
    queryKey: ["collections-workbench-cases", username],
    queryFn: () => collectionsService.listCases(0, 200, username ? { assignedTo: username } : undefined),
    staleTime: 30_000,
    retry: false,
  });

  const cases = casesPage?.content ?? [];

  const sortedCases = useMemo(() => {
    return [...cases].sort((a, b) => {
      const aOverdue = isOverdue(a) ? 1 : 0;
      const bOverdue = isOverdue(b) ? 1 : 0;
      if (aOverdue !== bOverdue) return bOverdue - aOverdue;
      return b.currentDpd - a.currentDpd;
    });
  }, [cases]);

  const stats = useMemo(() => {
    const overdue = cases.filter(isOverdue).length;
    const critical = cases.filter((c) => c.priority === "CRITICAL").length;
    return {
      total: cases.length,
      overdue,
      pendingPtp: summary?.pendingPtpCount ?? 0,
      critical,
    };
  }, [cases, summary]);

  const addActionMutation = useMutation({
    mutationFn: ({ caseId, req }: { caseId: string; req: AddActionRequest }) =>
      collectionsService.addAction(caseId, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collections-workbench-cases"] });
      setActionDialogOpen(false);
      setSelectedCaseId(null);
      setActionForm({ actionType: "PHONE_CALL", outcome: "", notes: "", nextActionDate: "" });
    },
  });

  const openActionDialog = (caseId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setSelectedCaseId(caseId);
    setActionDialogOpen(true);
  };

  const handleSubmitAction = () => {
    if (!selectedCaseId) return;
    addActionMutation.mutate({
      caseId: selectedCaseId,
      req: {
        ...actionForm,
        performedBy: username,
        outcome: actionForm.outcome || undefined,
        notes: actionForm.notes || undefined,
        nextActionDate: actionForm.nextActionDate || undefined,
      },
    });
  };

  const isLoading = summaryLoading || casesLoading;

  return (
    <DashboardLayout
      title="Collections Workbench"
      subtitle={`Daily view for ${username || "collections officer"}`}
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Collections" }, { label: "Workbench" }]}
    >
      <div className="space-y-4 animate-fade-in">
        {/* KPI Cards */}
        {isLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">My Cases</span>
                  <Users className="h-4 w-4 text-info" />
                </div>
                <p className="text-2xl font-heading">{stats.total}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Overdue Follow-ups</span>
                  <Clock className="h-4 w-4 text-destructive" />
                </div>
                <p className="text-2xl font-heading text-destructive">{stats.overdue}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Pending PTPs</span>
                  <AlertTriangle className="h-4 w-4 text-warning" />
                </div>
                <p className="text-2xl font-heading">{stats.pendingPtp}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Critical Cases</span>
                  <ShieldAlert className="h-4 w-4 text-destructive" />
                </div>
                <p className="text-2xl font-heading text-destructive">{stats.critical}</p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Cases Table */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">My Assigned Cases — Sorted by Priority</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {casesLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : sortedCases.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No assigned cases</p>
                <p className="text-xs mt-1">You have no collection cases assigned to you.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Case #</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Customer</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">DPD</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Stage</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Priority</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Outstanding</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Last Action</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedCases.map((c) => {
                    const overdue = isOverdue(c);
                    return (
                      <TableRow
                        key={c.id}
                        className={`cursor-pointer table-row-hover ${overdue ? "border-l-4 border-l-red-500 bg-red-50/50" : ""}`}
                        onClick={() => navigate(`/collections/case/${c.id}`)}
                      >
                        <TableCell className="text-xs font-mono font-medium">{c.caseNumber}</TableCell>
                        <TableCell className="text-xs font-sans">{c.customerId ?? "---"}</TableCell>
                        <TableCell className="text-right">
                          <span className={`text-xs font-mono font-bold ${c.currentDpd > 90 ? "text-red-700" : c.currentDpd > 30 ? "text-orange-600" : "text-yellow-600"}`}>
                            {c.currentDpd}
                          </span>
                        </TableCell>
                        <TableCell>{stageBadge(c.currentStage)}</TableCell>
                        <TableCell>{priorityBadge(c.priority)}</TableCell>
                        <TableCell className="text-xs font-mono text-right">{formatKES(c.outstandingAmount)}</TableCell>
                        <TableCell className="text-xs font-sans text-muted-foreground">
                          {c.lastActionAt ? new Date(c.lastActionAt).toLocaleDateString() : "Never"}
                          {overdue && (
                            <Badge className="ml-1.5 bg-red-100 text-red-700 border-red-200 text-[11px]">Overdue</Badge>
                          )}
                        </TableCell>
                        <TableCell>
                          <div className="flex gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 text-[11px] font-sans"
                              onClick={(e) => openActionDialog(c.id, e)}
                            >
                              <Phone className="h-3 w-3 mr-1" /> Log Action
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 text-[11px] font-sans"
                              onClick={(e) => { e.stopPropagation(); navigate(`/collections/case/${c.id}`); }}
                            >
                              <Eye className="h-3 w-3 mr-1" /> View
                            </Button>
                          </div>
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

      {/* Log Action Dialog */}
      <Dialog open={actionDialogOpen} onOpenChange={setActionDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-sm font-medium">Log Collection Action</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1">
              <label className="text-xs font-medium">Action Type</label>
              <Select
                value={actionForm.actionType}
                onValueChange={(v) => setActionForm((f) => ({ ...f, actionType: v }))}
              >
                <SelectTrigger className="h-9 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ACTION_TYPES.map((t) => (
                    <SelectItem key={t} value={t}>{t.replace(/_/g, " ")}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium">Outcome</label>
              <Select
                value={actionForm.outcome ?? ""}
                onValueChange={(v) => setActionForm((f) => ({ ...f, outcome: v }))}
              >
                <SelectTrigger className="h-9 text-xs">
                  <SelectValue placeholder="Select outcome..." />
                </SelectTrigger>
                <SelectContent>
                  {OUTCOMES.map((o) => (
                    <SelectItem key={o} value={o}>{o.replace(/_/g, " ")}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium">Notes</label>
              <Textarea
                className="text-xs min-h-[80px]"
                placeholder="Add notes about the interaction..."
                value={actionForm.notes ?? ""}
                onChange={(e) => setActionForm((f) => ({ ...f, notes: e.target.value }))}
              />
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium">Next Action Date</label>
              <Input
                type="date"
                className="h-9 text-xs"
                value={actionForm.nextActionDate ?? ""}
                onChange={(e) => setActionForm((f) => ({ ...f, nextActionDate: e.target.value }))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setActionDialogOpen(false)}>Cancel</Button>
            <Button
              size="sm"
              onClick={handleSubmitAction}
              disabled={addActionMutation.isPending}
            >
              {addActionMutation.isPending ? "Saving..." : "Log Action"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default CollectionsWorkbenchPage;
