import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Phone,
  Mail,
  MessageSquare,
  MapPin,
  FileText,
  RefreshCcw,
  Trash2,
  MoreHorizontal,
  Calendar,
  User,
  ArrowLeft,
  XCircle,
  Banknote,
} from "lucide-react";
import {
  collectionsService,
  type AddActionRequest,
  type AddPtpRequest,
  type CollectionAction,
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

function actionIcon(type: string) {
  switch (type) {
    case "PHONE_CALL": return <Phone className="h-4 w-4 text-blue-500" />;
    case "SMS": return <MessageSquare className="h-4 w-4 text-green-500" />;
    case "EMAIL": return <Mail className="h-4 w-4 text-purple-500" />;
    case "FIELD_VISIT": return <MapPin className="h-4 w-4 text-orange-500" />;
    case "LEGAL_NOTICE": return <FileText className="h-4 w-4 text-red-500" />;
    case "RESTRUCTURE_OFFER": return <RefreshCcw className="h-4 w-4 text-teal-500" />;
    case "WRITE_OFF": return <Trash2 className="h-4 w-4 text-gray-500" />;
    default: return <MoreHorizontal className="h-4 w-4 text-gray-400" />;
  }
}

const stageBadge = (stage: string) => {
  switch (stage) {
    case "WATCH":
      return <Badge className="bg-yellow-100 text-yellow-700 border-yellow-200">Watch</Badge>;
    case "SUBSTANDARD":
      return <Badge className="bg-orange-100 text-orange-700 border-orange-200">Substandard</Badge>;
    case "DOUBTFUL":
      return <Badge className="bg-red-100 text-red-700 border-red-200">Doubtful</Badge>;
    case "LOSS":
      return <Badge className="bg-red-200 text-red-900 border-red-300">Loss</Badge>;
    default:
      return <Badge variant="outline">{stage}</Badge>;
  }
};

const priorityBadge = (priority: string) => {
  switch (priority) {
    case "LOW":
      return <Badge variant="outline" className="bg-gray-100 text-gray-600 border-gray-200">Low</Badge>;
    case "NORMAL":
      return <Badge variant="outline" className="bg-blue-100 text-blue-700 border-blue-200">Normal</Badge>;
    case "HIGH":
      return <Badge variant="outline" className="bg-orange-100 text-orange-700 border-orange-200">High</Badge>;
    case "CRITICAL":
      return <Badge variant="outline" className="bg-red-100 text-red-700 border-red-200">Critical</Badge>;
    default:
      return <Badge variant="outline">{priority}</Badge>;
  }
};

const statusBadge = (status: string) => {
  switch (status) {
    case "OPEN":
      return <Badge className="bg-blue-100 text-blue-700 border-blue-200">Open</Badge>;
    case "CLOSED":
      return <Badge className="bg-green-100 text-green-700 border-green-200">Closed</Badge>;
    case "PENDING_LEGAL":
      return <Badge className="bg-red-100 text-red-700 border-red-200">Pending Legal</Badge>;
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
};

const ptpStatusBadge = (status: string) => {
  switch (status) {
    case "PENDING":
      return <Badge className="bg-yellow-100 text-yellow-700 border-yellow-200 text-[11px]">Pending</Badge>;
    case "FULFILLED":
      return <Badge className="bg-green-100 text-green-700 border-green-200 text-[11px]">Fulfilled</Badge>;
    case "BROKEN":
      return <Badge className="bg-red-100 text-red-700 border-red-200 text-[11px]">Broken</Badge>;
    case "CANCELLED":
      return <Badge className="bg-gray-100 text-gray-500 border-gray-200 text-[11px]">Cancelled</Badge>;
    default:
      return <Badge variant="outline" className="text-[11px]">{status}</Badge>;
  }
};

const CaseDetailPage = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user } = useAuth();
  const username = user?.name ?? user?.id ?? "";

  const [actionDialogOpen, setActionDialogOpen] = useState(false);
  const [ptpDialogOpen, setPtpDialogOpen] = useState(false);
  const [assignDialogOpen, setAssignDialogOpen] = useState(false);

  const [actionForm, setActionForm] = useState<AddActionRequest>({
    actionType: "PHONE_CALL",
    outcome: "",
    notes: "",
    contactPerson: "",
    nextActionDate: "",
  });

  const [ptpForm, setPtpForm] = useState<AddPtpRequest>({
    promisedAmount: 0,
    promiseDate: "",
    notes: "",
  });

  const [assignTo, setAssignTo] = useState("");

  const { data: detail, isLoading, isError } = useQuery({
    queryKey: ["collection-case-detail", id],
    queryFn: () => collectionsService.getCaseDetail(id!),
    enabled: !!id,
    staleTime: 15_000,
    retry: false,
  });

  const caseData = detail?.case;
  const actions = detail?.actions ?? [];
  const ptps = detail?.ptps ?? [];

  const addActionMutation = useMutation({
    mutationFn: (req: AddActionRequest) => collectionsService.addAction(id!, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection-case-detail", id] });
      setActionDialogOpen(false);
      setActionForm({ actionType: "PHONE_CALL", outcome: "", notes: "", contactPerson: "", nextActionDate: "" });
    },
  });

  const addPtpMutation = useMutation({
    mutationFn: (req: AddPtpRequest) => collectionsService.addPtp(id!, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection-case-detail", id] });
      setPtpDialogOpen(false);
      setPtpForm({ promisedAmount: 0, promiseDate: "", notes: "" });
    },
  });

  const updateCaseMutation = useMutation({
    mutationFn: (assignedTo: string) => collectionsService.updateCase(id!, { assignedTo }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection-case-detail", id] });
      setAssignDialogOpen(false);
      setAssignTo("");
    },
  });

  const closeCaseMutation = useMutation({
    mutationFn: () => collectionsService.closeCase(id!),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection-case-detail", id] });
    },
  });

  const handleSubmitAction = () => {
    addActionMutation.mutate({
      ...actionForm,
      performedBy: username,
      outcome: actionForm.outcome || undefined,
      notes: actionForm.notes || undefined,
      contactPerson: actionForm.contactPerson || undefined,
      nextActionDate: actionForm.nextActionDate || undefined,
    });
  };

  const handleSubmitPtp = () => {
    addPtpMutation.mutate({
      ...ptpForm,
      createdBy: username,
      notes: ptpForm.notes || undefined,
    });
  };

  if (isLoading) {
    return (
      <DashboardLayout
        title="Case Detail"
        subtitle="Loading..."
        breadcrumbs={[{ label: "Home", href: "/" }, { label: "Collections", href: "/collections" }, { label: "Case" }]}
      >
        <div className="space-y-4">
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      </DashboardLayout>
    );
  }

  if (isError || !caseData) {
    return (
      <DashboardLayout
        title="Case Detail"
        subtitle="Error"
        breadcrumbs={[{ label: "Home", href: "/" }, { label: "Collections", href: "/collections" }, { label: "Case" }]}
      >
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-16 text-destructive">
            <XCircle className="h-8 w-8 mb-2" />
            <p className="text-sm font-medium">Failed to load case detail</p>
            <p className="text-xs mt-1 text-muted-foreground">The case may not exist or the service is unavailable.</p>
            <Button variant="outline" size="sm" className="mt-4" onClick={() => navigate("/collections")}>
              <ArrowLeft className="h-3 w-3 mr-1" /> Back to Collections
            </Button>
          </CardContent>
        </Card>
      </DashboardLayout>
    );
  }

  const sortedActions = [...actions].sort(
    (a, b) => new Date(b.performedAt).getTime() - new Date(a.performedAt).getTime()
  );

  return (
    <DashboardLayout
      title={`Case ${caseData.caseNumber}`}
      subtitle={`Loan ${caseData.loanId?.slice(0, 8)} | DPD ${caseData.currentDpd}`}
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Collections", href: "/collections" },
        { label: caseData.caseNumber },
      ]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Header Card */}
        <Card>
          <CardContent className="p-6">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div className="space-y-3">
                <div className="flex items-center gap-2 flex-wrap">
                  <h2 className="text-lg font-heading font-semibold">{caseData.caseNumber}</h2>
                  {statusBadge(caseData.status)}
                  {stageBadge(caseData.currentStage)}
                  {priorityBadge(caseData.priority)}
                </div>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-x-8 gap-y-2 text-xs">
                  <div>
                    <span className="text-muted-foreground">Loan ID</span>
                    <p className="font-mono font-medium">{caseData.loanId?.slice(0, 12)}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Customer</span>
                    <p className="font-medium">{caseData.customerId ?? "---"}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">DPD</span>
                    <p className="font-mono font-bold text-base">{caseData.currentDpd}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Outstanding</span>
                    <p className="font-mono font-bold text-base">{formatKES(caseData.outstandingAmount)}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Assigned To</span>
                    <p className="font-medium">{caseData.assignedTo ?? "Unassigned"}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Opened</span>
                    <p className="font-medium">{new Date(caseData.openedAt).toLocaleDateString()}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Last Action</span>
                    <p className="font-medium">{caseData.lastActionAt ? new Date(caseData.lastActionAt).toLocaleDateString() : "Never"}</p>
                  </div>
                  {caseData.closedAt && (
                    <div>
                      <span className="text-muted-foreground">Closed</span>
                      <p className="font-medium">{new Date(caseData.closedAt).toLocaleDateString()}</p>
                    </div>
                  )}
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button size="sm" onClick={() => setActionDialogOpen(true)}>
                  <Phone className="h-3 w-3 mr-1" /> Log Action
                </Button>
                <Button size="sm" variant="outline" onClick={() => setPtpDialogOpen(true)}>
                  <Banknote className="h-3 w-3 mr-1" /> Add PTP
                </Button>
                <Button size="sm" variant="outline" onClick={() => setAssignDialogOpen(true)}>
                  <User className="h-3 w-3 mr-1" /> Assign
                </Button>
                {caseData.status !== "CLOSED" && (
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button size="sm" variant="destructive">
                        <XCircle className="h-3 w-3 mr-1" /> Close Case
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>Close this case?</AlertDialogTitle>
                        <AlertDialogDescription>
                          This will mark case {caseData.caseNumber} as closed. This action cannot be undone.
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                          onClick={() => closeCaseMutation.mutate()}
                          disabled={closeCaseMutation.isPending}
                        >
                          {closeCaseMutation.isPending ? "Closing..." : "Close Case"}
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                )}
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Tabs */}
        <Tabs defaultValue="actions" className="w-full">
          <TabsList>
            <TabsTrigger value="actions" className="text-xs">
              Action Timeline ({actions.length})
            </TabsTrigger>
            <TabsTrigger value="ptps" className="text-xs">
              Promises to Pay ({ptps.length})
            </TabsTrigger>
          </TabsList>

          {/* Action Timeline */}
          <TabsContent value="actions">
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium">Action Timeline</CardTitle>
              </CardHeader>
              <CardContent>
                {sortedActions.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                    <Phone className="h-8 w-8 mb-2 text-muted-foreground/50" />
                    <p className="text-sm font-medium">No actions recorded</p>
                    <p className="text-xs mt-1">Log the first action using the button above.</p>
                  </div>
                ) : (
                  <div className="relative space-y-0">
                    {sortedActions.map((action: CollectionAction, idx: number) => (
                      <div key={action.id} className="flex gap-4 pb-6 relative">
                        {/* Timeline line */}
                        {idx < sortedActions.length - 1 && (
                          <div className="absolute left-[19px] top-8 bottom-0 w-px bg-border" />
                        )}
                        {/* Icon */}
                        <div className="flex-shrink-0 w-10 h-10 rounded-full bg-muted flex items-center justify-center z-10">
                          {actionIcon(action.actionType)}
                        </div>
                        {/* Content */}
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="text-xs font-medium">
                              {action.actionType.replace(/_/g, " ")}
                            </span>
                            {action.outcome && (
                              <Badge variant="outline" className="text-[11px]">
                                {action.outcome.replace(/_/g, " ")}
                              </Badge>
                            )}
                            {action.contactMethod && (
                              <Badge variant="secondary" className="text-[11px]">
                                via {action.contactMethod}
                              </Badge>
                            )}
                          </div>
                          {action.notes && (
                            <p className="text-xs text-muted-foreground mt-1">{action.notes}</p>
                          )}
                          <div className="flex items-center gap-3 mt-1.5 text-[11px] text-muted-foreground">
                            {action.performedBy && (
                              <span className="flex items-center gap-1">
                                <User className="h-3 w-3" /> {action.performedBy}
                              </span>
                            )}
                            <span className="flex items-center gap-1">
                              <Calendar className="h-3 w-3" /> {new Date(action.performedAt).toLocaleString()}
                            </span>
                            {action.contactPerson && (
                              <span>Contact: {action.contactPerson}</span>
                            )}
                            {action.nextActionDate && (
                              <span className="text-blue-600">Next: {new Date(action.nextActionDate).toLocaleDateString()}</span>
                            )}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          {/* Promises to Pay */}
          <TabsContent value="ptps">
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-medium">Promises to Pay</CardTitle>
              </CardHeader>
              <CardContent className="p-0">
                {ptps.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
                    <Banknote className="h-8 w-8 mb-2 text-muted-foreground/50" />
                    <p className="text-sm font-medium">No promises to pay</p>
                    <p className="text-xs mt-1">Record a PTP using the button above.</p>
                  </div>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow className="hover:bg-transparent">
                        <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Amount</TableHead>
                        <TableHead className="text-[11px] uppercase tracking-wider font-sans">Promise Date</TableHead>
                        <TableHead className="text-[11px] uppercase tracking-wider font-sans">Status</TableHead>
                        <TableHead className="text-[11px] uppercase tracking-wider font-sans">Created By</TableHead>
                        <TableHead className="text-[11px] uppercase tracking-wider font-sans">Notes</TableHead>
                        <TableHead className="text-[11px] uppercase tracking-wider font-sans">Created</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {ptps.map((ptp) => (
                        <TableRow key={ptp.id}>
                          <TableCell className="text-xs font-mono text-right font-medium">{formatKES(ptp.promisedAmount)}</TableCell>
                          <TableCell className="text-xs font-sans">{new Date(ptp.promiseDate).toLocaleDateString()}</TableCell>
                          <TableCell>{ptpStatusBadge(ptp.status)}</TableCell>
                          <TableCell className="text-xs font-sans">{ptp.createdBy ?? "---"}</TableCell>
                          <TableCell className="text-xs font-sans text-muted-foreground max-w-[200px] truncate">{ptp.notes ?? "---"}</TableCell>
                          <TableCell className="text-xs font-sans text-muted-foreground">{new Date(ptp.createdAt).toLocaleDateString()}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
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
              <label className="text-xs font-medium">Contact Person</label>
              <Input
                className="h-9 text-xs"
                placeholder="Name of person contacted..."
                value={actionForm.contactPerson ?? ""}
                onChange={(e) => setActionForm((f) => ({ ...f, contactPerson: e.target.value }))}
              />
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

      {/* Add PTP Dialog */}
      <Dialog open={ptpDialogOpen} onOpenChange={setPtpDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-sm font-medium">Record Promise to Pay</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1">
              <label className="text-xs font-medium">Promised Amount (KES)</label>
              <Input
                type="number"
                className="h-9 text-xs"
                placeholder="0"
                value={ptpForm.promisedAmount || ""}
                onChange={(e) => setPtpForm((f) => ({ ...f, promisedAmount: Number(e.target.value) }))}
              />
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium">Promise Date</label>
              <Input
                type="date"
                className="h-9 text-xs"
                value={ptpForm.promiseDate}
                onChange={(e) => setPtpForm((f) => ({ ...f, promiseDate: e.target.value }))}
              />
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium">Notes</label>
              <Textarea
                className="text-xs min-h-[60px]"
                placeholder="Optional notes..."
                value={ptpForm.notes ?? ""}
                onChange={(e) => setPtpForm((f) => ({ ...f, notes: e.target.value }))}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setPtpDialogOpen(false)}>Cancel</Button>
            <Button
              size="sm"
              onClick={handleSubmitPtp}
              disabled={addPtpMutation.isPending || !ptpForm.promisedAmount || !ptpForm.promiseDate}
            >
              {addPtpMutation.isPending ? "Saving..." : "Record PTP"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Assign Dialog */}
      <Dialog open={assignDialogOpen} onOpenChange={setAssignDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-sm font-medium">Assign Case</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1">
              <label className="text-xs font-medium">Assign To</label>
              <Input
                className="h-9 text-xs"
                placeholder="Enter officer name or ID..."
                value={assignTo}
                onChange={(e) => setAssignTo(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setAssignDialogOpen(false)}>Cancel</Button>
            <Button
              size="sm"
              onClick={() => updateCaseMutation.mutate(assignTo)}
              disabled={updateCaseMutation.isPending || !assignTo}
            >
              {updateCaseMutation.isPending ? "Saving..." : "Assign"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default CaseDetailPage;
