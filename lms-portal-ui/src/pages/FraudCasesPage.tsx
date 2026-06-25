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
import { Label } from "@/components/ui/label";
import {
  Briefcase, Plus, Search, Filter, MessageSquare, Clock, AlertTriangle, History,
} from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fraudService, type FraudCase, type CaseStatus, type AlertSeverity, type CaseTimeline,
} from "@/services/fraudService";
import { formatKES } from "@/lib/format";
import { useToast } from "@/hooks/use-toast";

const severityColor: Record<AlertSeverity, string> = {
  CRITICAL: "bg-red-500/15 text-red-600 border-red-500/30",
  HIGH: "bg-destructive/15 text-destructive border-destructive/30",
  MEDIUM: "bg-warning/15 text-warning border-warning/30",
  LOW: "bg-info/15 text-info border-info/30",
};

const caseStatusColor: Record<string, string> = {
  OPEN: "bg-destructive/15 text-destructive border-destructive/30",
  INVESTIGATING: "bg-warning/15 text-warning border-warning/30",
  PENDING_REVIEW: "bg-info/15 text-info border-info/30",
  ESCALATED: "bg-red-500/15 text-red-600 border-red-500/30",
  CLOSED_CONFIRMED: "bg-red-600/15 text-red-700 border-red-600/30",
  CLOSED_FALSE_POSITIVE: "bg-success/15 text-success border-success/30",
  CLOSED_INCONCLUSIVE: "bg-muted text-muted-foreground border-muted",
};

const FraudCasesPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [page, setPage] = useState(0);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [detailCase, setDetailCase] = useState<FraudCase | null>(null);
  const [noteContent, setNoteContent] = useState("");
  const [timeline, setTimeline] = useState<CaseTimeline | null>(null);

  // Create case form state
  const [newTitle, setNewTitle] = useState("");
  const [newDescription, setNewDescription] = useState("");
  const [newPriority, setNewPriority] = useState("MEDIUM");
  const [newCustomerId, setNewCustomerId] = useState("");

  const queryStatus = statusFilter !== "all" ? statusFilter as CaseStatus : undefined;

  const { data: casesPage, isLoading } = useQuery({
    queryKey: ["fraud", "cases", page, queryStatus],
    queryFn: () => fraudService.listCases(page, 20, queryStatus),
    staleTime: 30_000,
    retry: false,
  });

  const cases = casesPage?.content ?? [];
  const filtered = cases.filter((c) => {
    if (!searchQuery) return true;
    const q = searchQuery.toLowerCase();
    return (
      c.caseNumber?.toLowerCase().includes(q) ||
      c.title?.toLowerCase().includes(q) ||
      c.customerId?.toLowerCase().includes(q) ||
      c.assignedTo?.toLowerCase().includes(q)
    );
  });

  const createMutation = useMutation({
    mutationFn: () => fraudService.createCase({
      title: newTitle,
      description: newDescription,
      priority: newPriority,
      customerId: newCustomerId || undefined,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fraud", "cases"] });
      setCreateDialogOpen(false);
      setNewTitle("");
      setNewDescription("");
      setNewPriority("MEDIUM");
      setNewCustomerId("");
      toast({ title: "Case created" });
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const addNoteMutation = useMutation({
    mutationFn: (caseId: string) => fraudService.addCaseNote(caseId, {
      content: noteContent, author: "admin",
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fraud", "cases"] });
      if (detailCase) {
        fraudService.getCase(detailCase.id).then(setDetailCase);
      }
      setNoteContent("");
      toast({ title: "Note added" });
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const updateStatusMutation = useMutation({
    mutationFn: (params: { id: string; status: string }) =>
      fraudService.updateCase(params.id, { status: params.status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fraud", "cases"] });
      if (detailCase) {
        fraudService.getCase(detailCase.id).then(setDetailCase);
      }
      toast({ title: "Case status updated" });
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  return (
    <DashboardLayout
      title="Investigation Cases"
      subtitle="Manage fraud investigation cases, evidence, and outcomes"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "Cases" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Badge className="bg-info/10 text-info border-info/20 gap-1.5 px-3 py-1">
              <Briefcase className="h-3 w-3" />
              Case Management
            </Badge>
          </div>
          <Button size="sm" className="h-8 text-xs gap-1.5" onClick={() => setCreateDialogOpen(true)}>
            <Plus className="h-3.5 w-3.5" />
            New Case
          </Button>
        </div>

        {/* Filters */}
        <div className="flex flex-col sm:flex-row gap-3">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search by case number, title, customer..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 h-9 text-sm"
            />
          </div>
          <Select value={statusFilter} onValueChange={(v) => { setStatusFilter(v); setPage(0); }}>
            <SelectTrigger className="h-9 w-[180px] text-xs">
              <Filter className="h-3.5 w-3.5 mr-1.5" />
              <SelectValue placeholder="All Statuses" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Statuses</SelectItem>
              <SelectItem value="OPEN">Open</SelectItem>
              <SelectItem value="INVESTIGATING">Investigating</SelectItem>
              <SelectItem value="PENDING_REVIEW">Pending Review</SelectItem>
              <SelectItem value="ESCALATED">Escalated</SelectItem>
              <SelectItem value="CLOSED_CONFIRMED">Closed - Confirmed</SelectItem>
              <SelectItem value="CLOSED_FALSE_POSITIVE">Closed - False Positive</SelectItem>
              <SelectItem value="CLOSED_INCONCLUSIVE">Closed - Inconclusive</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* Cases Table */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">
              Cases
              {casesPage && <span className="text-muted-foreground font-normal ml-2">({casesPage.totalElements} total)</span>}
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">{Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}</div>
            ) : filtered.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-40 text-muted-foreground">
                <Briefcase className="h-10 w-10 mb-3 opacity-30" />
                <p className="text-sm font-medium">No investigation cases</p>
                <p className="text-xs mt-1">Create a case to begin an investigation</p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="text-xs">Case #</TableHead>
                      <TableHead className="text-xs">Title</TableHead>
                      <TableHead className="text-xs">Customer</TableHead>
                      <TableHead className="text-xs">Priority</TableHead>
                      <TableHead className="text-xs">Status</TableHead>
                      <TableHead className="text-xs">Assigned</TableHead>
                      <TableHead className="text-xs">Exposure</TableHead>
                      <TableHead className="text-xs">SLA</TableHead>
                      <TableHead className="text-xs">Created</TableHead>
                      <TableHead className="text-xs text-right">Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filtered.map((c) => (
                      <TableRow key={c.id} className="table-row-hover cursor-pointer" onClick={() => {
                        fraudService.getCase(c.id).then((fc) => { setDetailCase(fc); fraudService.getCaseTimeline(fc.id).then(setTimeline).catch(() => setTimeline(null)); }).catch(() => setDetailCase(c));
                      }}>
                        <TableCell className="text-xs font-mono font-medium">{c.caseNumber}</TableCell>
                        <TableCell className="text-xs max-w-[200px] truncate">{c.title}</TableCell>
                        <TableCell className="text-xs font-mono">{c.customerId ?? "—"}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[10px] ${severityColor[c.priority]}`}>{c.priority}</Badge>
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[10px] ${caseStatusColor[c.status] ?? ""}`}>
                            {c.status?.replace(/_/g, " ")}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs">{c.assignedTo ?? "—"}</TableCell>
                        <TableCell className="text-xs">{c.totalExposure ? formatKES(c.totalExposure) : "—"}</TableCell>
                        <TableCell className="text-xs">
                          {(c as Record<string, unknown>).slaBreached ? (
                            <Badge variant="outline" className="text-[9px] bg-destructive/15 text-destructive border-destructive/30">
                              Breached
                            </Badge>
                          ) : (c as Record<string, unknown>).slaDeadline ? (
                            <span className="text-muted-foreground">{new Date(String((c as Record<string, unknown>).slaDeadline)).toLocaleDateString()}</span>
                          ) : "—"}
                        </TableCell>
                        <TableCell className="text-xs whitespace-nowrap">{c.createdAt?.split("T")[0]}</TableCell>
                        <TableCell className="text-right">
                          <Button variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={(e) => {
                            e.stopPropagation();
                            fraudService.getCase(c.id).then((fc) => { setDetailCase(fc); fraudService.getCaseTimeline(fc.id).then(setTimeline).catch(() => setTimeline(null)); }).catch(() => setDetailCase(c));
                          }}>
                            View
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}

            {casesPage && casesPage.totalPages > 1 && (
              <div className="flex items-center justify-between px-4 py-3 border-t">
                <span className="text-xs text-muted-foreground">Page {casesPage.number + 1} of {casesPage.totalPages}</span>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" className="h-7 text-xs" disabled={casesPage.first} onClick={() => setPage((p) => Math.max(0, p - 1))}>Previous</Button>
                  <Button variant="outline" size="sm" className="h-7 text-xs" disabled={casesPage.last} onClick={() => setPage((p) => p + 1)}>Next</Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Create Case Dialog */}
      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="text-sm">New Investigation Case</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <Label className="text-xs">Title</Label>
              <Input value={newTitle} onChange={(e) => setNewTitle(e.target.value)} placeholder="Case title..." className="mt-1 text-sm" />
            </div>
            <div>
              <Label className="text-xs">Description</Label>
              <Textarea value={newDescription} onChange={(e) => setNewDescription(e.target.value)} placeholder="Investigation details..." rows={3} className="mt-1 text-sm" />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label className="text-xs">Priority</Label>
                <Select value={newPriority} onValueChange={setNewPriority}>
                  <SelectTrigger className="mt-1 text-xs h-9"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="LOW">Low</SelectItem>
                    <SelectItem value="MEDIUM">Medium</SelectItem>
                    <SelectItem value="HIGH">High</SelectItem>
                    <SelectItem value="CRITICAL">Critical</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label className="text-xs">Customer ID</Label>
                <Input value={newCustomerId} onChange={(e) => setNewCustomerId(e.target.value)} placeholder="Optional" className="mt-1 text-sm h-9" />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
            <Button size="sm" disabled={!newTitle || createMutation.isPending} onClick={() => createMutation.mutate()}>
              {createMutation.isPending ? "Creating..." : "Create Case"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Case Detail Dialog */}
      <Dialog open={!!detailCase} onOpenChange={() => { setDetailCase(null); setTimeline(null); }}>
        <DialogContent className="sm:max-w-2xl max-h-[85vh] overflow-y-auto">
          {detailCase && (
            <>
              <DialogHeader>
                <DialogTitle className="text-sm flex items-center gap-2">
                  <span className="font-mono">{detailCase.caseNumber}</span>
                  <Badge variant="outline" className={`text-[10px] ${caseStatusColor[detailCase.status]}`}>
                    {detailCase.status?.replace(/_/g, " ")}
                  </Badge>
                </DialogTitle>
              </DialogHeader>

              <div className="space-y-4">
                <div>
                  <h4 className="text-sm font-medium">{detailCase.title}</h4>
                  {detailCase.description && <p className="text-xs text-muted-foreground mt-1">{detailCase.description}</p>}
                </div>

                <div className="grid grid-cols-3 gap-3 text-xs">
                  <div className="bg-muted rounded-md p-2">
                    <span className="text-muted-foreground">Priority</span>
                    <p className="font-medium mt-0.5">{detailCase.priority}</p>
                  </div>
                  <div className="bg-muted rounded-md p-2">
                    <span className="text-muted-foreground">Assigned To</span>
                    <p className="font-medium mt-0.5">{detailCase.assignedTo ?? "Unassigned"}</p>
                  </div>
                  <div className="bg-muted rounded-md p-2">
                    <span className="text-muted-foreground">Exposure</span>
                    <p className="font-medium mt-0.5">{detailCase.totalExposure ? formatKES(detailCase.totalExposure) : "—"}</p>
                  </div>
                </div>

                {/* Status Actions */}
                {!detailCase.status.startsWith("CLOSED") && (
                  <div className="flex gap-2 flex-wrap">
                    {detailCase.status === "OPEN" && (
                      <Button size="sm" variant="outline" className="h-7 text-xs"
                        onClick={() => updateStatusMutation.mutate({ id: detailCase.id, status: "INVESTIGATING" })}>
                        Start Investigation
                      </Button>
                    )}
                    {detailCase.status === "INVESTIGATING" && (
                      <Button size="sm" variant="outline" className="h-7 text-xs"
                        onClick={() => updateStatusMutation.mutate({ id: detailCase.id, status: "PENDING_REVIEW" })}>
                        Submit for Review
                      </Button>
                    )}
                    <Button size="sm" variant="destructive" className="h-7 text-xs"
                      onClick={() => updateStatusMutation.mutate({ id: detailCase.id, status: "CLOSED_CONFIRMED" })}>
                      Close - Confirmed Fraud
                    </Button>
                    <Button size="sm" variant="outline" className="h-7 text-xs text-success"
                      onClick={() => updateStatusMutation.mutate({ id: detailCase.id, status: "CLOSED_FALSE_POSITIVE" })}>
                      Close - False Positive
                    </Button>
                  </div>
                )}

                {/* SLA Indicator */}
                {(detailCase as Record<string, unknown>).slaDeadline && (
                  <div className={`rounded-md p-3 text-xs flex items-center gap-2 ${
                    (detailCase as Record<string, unknown>).slaBreached
                      ? "bg-destructive/10 border border-destructive/30"
                      : "bg-muted"
                  }`}>
                    {(detailCase as Record<string, unknown>).slaBreached ? (
                      <AlertTriangle className="h-4 w-4 text-destructive shrink-0" />
                    ) : (
                      <Clock className="h-4 w-4 text-muted-foreground shrink-0" />
                    )}
                    <div>
                      <span className="font-medium">
                        {(detailCase as Record<string, unknown>).slaBreached ? "SLA BREACHED" : "SLA Deadline"}
                      </span>
                      <span className="text-muted-foreground ml-2">
                        {new Date(String((detailCase as Record<string, unknown>).slaDeadline)).toLocaleString()}
                      </span>
                    </div>
                  </div>
                )}

                {/* Case Timeline */}
                {timeline && timeline.events.length > 0 && (
                  <div>
                    <h4 className="text-xs font-medium flex items-center gap-1.5 mb-2">
                      <History className="h-3.5 w-3.5" />
                      Activity Timeline ({timeline.events.length})
                    </h4>
                    <div className="space-y-1 max-h-40 overflow-y-auto border rounded-md p-2">
                      {timeline.events.map((ev, idx) => (
                        <div key={idx} className="flex items-start gap-2 py-1.5 border-b last:border-0">
                          <div className="h-1.5 w-1.5 rounded-full bg-muted-foreground mt-1.5 shrink-0" />
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center justify-between">
                              <span className="text-xs font-medium">{ev.action.replace(/_/g, " ")}</span>
                              <span className="text-[10px] text-muted-foreground">
                                {new Date(ev.timestamp).toLocaleString()}
                              </span>
                            </div>
                            {ev.description && (
                              <p className="text-[10px] text-muted-foreground truncate">{ev.description}</p>
                            )}
                            <p className="text-[10px] text-muted-foreground">by {ev.performedBy}</p>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Notes */}
                <div>
                  <h4 className="text-xs font-medium flex items-center gap-1.5 mb-2">
                    <MessageSquare className="h-3.5 w-3.5" />
                    Investigation Notes ({detailCase.notes?.length ?? 0})
                  </h4>

                  <div className="space-y-2 max-h-48 overflow-y-auto">
                    {(detailCase.notes ?? []).map((note) => (
                      <div key={note.id} className="rounded-md bg-muted p-2.5">
                        <div className="flex items-center justify-between mb-1">
                          <span className="text-xs font-medium">{note.author}</span>
                          <span className="text-[10px] text-muted-foreground flex items-center gap-1">
                            <Clock className="h-2.5 w-2.5" />
                            {note.createdAt?.split("T")[0]}
                          </span>
                        </div>
                        <p className="text-xs text-muted-foreground">{note.content}</p>
                      </div>
                    ))}
                    {(!detailCase.notes || detailCase.notes.length === 0) && (
                      <p className="text-xs text-muted-foreground text-center py-3">No notes yet</p>
                    )}
                  </div>

                  <div className="flex gap-2 mt-2">
                    <Textarea
                      value={noteContent}
                      onChange={(e) => setNoteContent(e.target.value)}
                      placeholder="Add investigation note..."
                      rows={2}
                      className="text-sm flex-1"
                    />
                    <Button size="sm" className="h-auto px-3"
                      disabled={!noteContent || addNoteMutation.isPending}
                      onClick={() => addNoteMutation.mutate(detailCase.id)}>
                      Add
                    </Button>
                  </div>
                </div>
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default FraudCasesPage;
