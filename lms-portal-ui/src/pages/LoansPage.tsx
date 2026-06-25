import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Sheet, SheetContent,
} from "@/components/ui/sheet";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Search, Download, Plus, LayoutGrid, List, Eye,
  CheckCircle2, XCircle, FileText, AlertTriangle,
  User, Phone, Hash, Calendar, MapPin, Brain,
} from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";
import { formatKES } from "@/lib/format";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/hooks/use-toast";
import {
  loanOriginationService,
  type LoanApplication as ApiLoanApplication,
} from "@/services/loanOriginationService";
import { customerService, type Customer } from "@/services/customerService";
import { productService } from "@/services/productService";
import { accountService } from "@/services/accountService";

// ─── Local Types ─────────────────────────────────────
type ApplicationStage = "received" | "kyc_pending" | "under_assessment" | "credit_committee" | "approved" | "rejected";

interface LoanApplication {
  id: string;
  customerId: string;
  customerName: string;
  customerInitials: string;
  amount: number;
  product: string;
  productColor: string;
  riskScore: number;
  stage: ApplicationStage;
  daysInStage: number;
  analyst: string;
  date: string;
  phone: string;
  nationalId: string;
  channel: string;
  purpose: string;
  tenor: number;
  compositeScore: number;
  compositeLabel: string;
  aiRecommendation: string;
  aiConfidence: number;
  documents: { name: string; status: "verified" | "pending" | "missing" }[];
  backendStatus: string;
  approvedAmount?: number;
  interestRate?: number;
}

const stageConfig: Record<ApplicationStage, { label: string; color: string }> = {
  received: { label: "Received", color: "bg-info/15 text-info border-info/30" },
  kyc_pending: { label: "KYC Pending", color: "bg-warning/15 text-warning border-warning/30" },
  under_assessment: { label: "Under Assessment", color: "bg-accent/15 text-accent-foreground border-accent/30" },
  credit_committee: { label: "Credit Committee", color: "bg-[hsl(var(--navy-700))]/15 text-[hsl(var(--navy-700))] border-[hsl(var(--navy-700))]/30" },
  approved: { label: "Approved", color: "bg-success/15 text-success border-success/30" },
  rejected: { label: "Rejected", color: "bg-destructive/10 text-destructive border-destructive/20" },
};

function statusToStage(status: string): ApplicationStage {
  const map: Record<string, ApplicationStage> = {
    DRAFT: "received",
    SUBMITTED: "kyc_pending",
    UNDER_REVIEW: "under_assessment",
    PENDING_APPROVAL: "credit_committee",
    APPROVED: "approved",
    REJECTED: "rejected",
    DISBURSED: "approved",
    CANCELLED: "rejected",
  };
  return map[status?.toUpperCase()] ?? "received";
}

function adaptApplication(app: ApiLoanApplication): LoanApplication {
  const stage = statusToStage(app.status);
  return {
    id: app.id,
    customerId: app.customerId,
    customerName: app.customerId,
    customerInitials: app.customerId.slice(0, 2).toUpperCase(),
    amount: app.requestedAmount,
    product: app.productId,
    productColor: "bg-info/15 text-info border-info/30",
    stage,
    riskScore: app.creditScore ?? 650,
    daysInStage: 0,
    analyst: "Unassigned",
    date: app.createdAt ? app.createdAt.split("T")[0] : "—",
    phone: "—",
    nationalId: "—",
    channel: "Digital",
    tenor: app.tenorMonths,
    purpose: app.purpose ?? "—",
    compositeScore: app.creditScore ?? 650,
    compositeLabel: "Standard",
    aiRecommendation: "APPROVAL",
    aiConfidence: 80,
    documents: [],
    backendStatus: app.status,
    approvedAmount: app.approvedAmount,
    interestRate: app.interestRate,
  };
}

const stageOrder: ApplicationStage[] = ["received", "kyc_pending", "under_assessment", "credit_committee", "approved", "rejected"];

const LoansPage = () => {
  const [viewMode, setViewMode] = useState<"kanban" | "list">("kanban");
  const [searchQuery, setSearchQuery] = useState("");
  const [stageFilter, setStageFilter] = useState("all");
  const [selectedApp, setSelectedApp] = useState<LoanApplication | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [newAppOpen, setNewAppOpen] = useState(false);

  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["loan-applications", "list"],
    queryFn: () => loanOriginationService.listApplications(0, 200),
    staleTime: 60_000,
    retry: false,
  });

  const allApplications: LoanApplication[] =
    apiPage && apiPage.content.length > 0
      ? apiPage.content.map(adaptApplication)
      : [];

  const filteredApps = allApplications.filter((app) => {
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      if (!app.customerName.toLowerCase().includes(q) && !app.id.toLowerCase().includes(q)) return false;
    }
    if (stageFilter !== "all" && app.stage !== stageFilter) return false;
    return true;
  });

  const openDetail = (app: LoanApplication) => {
    setSelectedApp(app);
    setDrawerOpen(true);
  };

  const stageCounts = stageOrder.reduce((acc, stage) => {
    acc[stage] = filteredApps.filter(a => a.stage === stage).length;
    return acc;
  }, {} as Record<ApplicationStage, number>);

  return (
    <DashboardLayout
      title="Loan Applications"
      subtitle="Full application pipeline — Kanban & list views"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Lending" }, { label: "Loan Applications" }]}
    >
      <div className="space-y-4">
        {/* Action bar */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div className="flex items-center gap-2 w-full sm:w-auto">
            <div className="relative flex-1 sm:w-72">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
              <Input
                placeholder="Search by ID, customer name..."
                className="pl-8 h-9 text-xs font-sans"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
            </div>
            <Select value={stageFilter} onValueChange={setStageFilter}>
              <SelectTrigger className="w-40 h-9 text-xs font-sans">
                <SelectValue placeholder="Stage" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Stages</SelectItem>
                {stageOrder.map(s => (
                  <SelectItem key={s} value={s}>{stageConfig[s].label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <div className="flex border border-border rounded-md overflow-hidden">
              <button
                onClick={() => setViewMode("kanban")}
                className={`p-2 transition-colors ${viewMode === "kanban" ? "bg-primary text-primary-foreground" : "bg-card text-muted-foreground hover:bg-muted"}`}
              >
                <LayoutGrid className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => setViewMode("list")}
                className={`p-2 transition-colors ${viewMode === "list" ? "bg-primary text-primary-foreground" : "bg-card text-muted-foreground hover:bg-muted"}`}
              >
                <List className="h-3.5 w-3.5" />
              </button>
            </div>
            <Button variant="outline" size="sm" className="text-xs font-sans">
              <Download className="mr-1.5 h-3.5 w-3.5" /> Export
            </Button>
            <Button size="sm" className="text-xs font-sans bg-primary hover:bg-primary/90" onClick={() => setNewAppOpen(true)}>
              <Plus className="mr-1.5 h-3.5 w-3.5" /> New Application
            </Button>
          </div>
        </div>

        {isLoading && (
          <div className="flex gap-3 overflow-x-auto pb-4">
            {stageOrder.map((stage) => (
              <div key={stage} className="min-w-[260px] w-[260px] shrink-0 space-y-2">
                <Skeleton className="h-6 w-32" />
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className="h-24 w-full" />
                ))}
              </div>
            ))}
          </div>
        )}

        {/* Kanban View */}
        {!isLoading && viewMode === "kanban" && (
          <div className="flex gap-3 overflow-x-auto pb-4">
            {stageOrder.map((stage) => {
              const config = stageConfig[stage];
              const apps = filteredApps.filter(a => a.stage === stage);
              return (
                <div key={stage} className="min-w-[260px] w-[260px] shrink-0">
                  <div className="flex items-center justify-between mb-2 px-1">
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-sans font-semibold">{config.label}</span>
                      <Badge variant="outline" className="text-[10px] font-mono h-5 min-w-[24px] justify-center">
                        {stageCounts[stage]}
                      </Badge>
                    </div>
                  </div>
                  <div className="space-y-2 max-h-[calc(100vh-280px)] overflow-y-auto pr-1">
                    <AnimatePresence>
                      {apps.map((app) => (
                        <motion.div
                          key={app.id}
                          layout
                          initial={{ opacity: 0, scale: 0.95 }}
                          animate={{ opacity: 1, scale: 1 }}
                          exit={{ opacity: 0, scale: 0.95 }}
                          className="kanban-card"
                          onClick={() => openDetail(app)}
                        >
                          <div className="flex items-center gap-2 mb-2">
                            <div className="h-7 w-7 rounded-full bg-primary/10 flex items-center justify-center text-[10px] font-semibold text-primary font-sans shrink-0">
                              {app.customerInitials}
                            </div>
                            <div className="flex-1 min-w-0">
                              <p className="text-xs font-sans font-medium truncate">{app.customerName}</p>
                              <p className="text-[10px] text-muted-foreground font-mono">{app.id}</p>
                            </div>
                          </div>
                          <div className="flex items-center justify-between mb-2">
                            <span className="text-sm font-mono font-bold">{formatKES(app.amount)}</span>
                          </div>
                          <div className="flex items-center gap-1.5 mb-2">
                            <Badge variant="outline" className={`text-[9px] font-sans ${app.productColor}`}>{app.product}</Badge>
                          </div>
                          <div className="flex items-center justify-between text-[10px] text-muted-foreground font-sans">
                            <div className="flex items-center gap-1">
                              <div className="h-4 w-4 rounded-full border-2 flex items-center justify-center" style={{
                                borderColor: app.riskScore > 700 ? "hsl(160, 60%, 38%)" : app.riskScore > 600 ? "hsl(38, 92%, 48%)" : "hsl(0, 72%, 51%)"
                              }}>
                                <span className="text-[7px] font-mono font-bold">{Math.floor(app.riskScore / 10)}</span>
                              </div>
                              <span>{app.backendStatus}</span>
                            </div>
                            <span>{app.date}</span>
                          </div>
                        </motion.div>
                      ))}
                    </AnimatePresence>
                    {apps.length === 0 && (
                      <div className="text-center py-8 text-xs text-muted-foreground font-sans">
                        No applications
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* List View */}
        {!isLoading && viewMode === "list" && (
          <Card>
            <CardContent className="p-0">
              {filteredApps.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                  <p className="text-sm font-medium">No applications found</p>
                  <p className="text-xs mt-1">No loan applications returned from the backend.</p>
                </div>
              ) : (
                <>
                  <Table>
                    <TableHeader>
                      <TableRow className="hover:bg-transparent">
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans">Application ID</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans">Customer</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans">Product</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans text-right">Amount</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans">Score</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans">Stage</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans">Status</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans">Date</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider font-sans">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredApps.slice(0, 50).map((app) => (
                        <TableRow key={app.id} className="table-row-hover cursor-pointer" onClick={() => openDetail(app)}>
                          <TableCell className="text-xs font-mono font-medium">{app.id}</TableCell>
                          <TableCell>
                            <div className="flex items-center gap-2">
                              <div className="h-6 w-6 rounded-full bg-primary/10 flex items-center justify-center text-[9px] font-semibold text-primary font-sans">
                                {app.customerInitials}
                              </div>
                              <span className="text-xs font-sans font-medium">{app.customerName}</span>
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge variant="outline" className={`text-[10px] font-sans ${app.productColor}`}>{app.product}</Badge>
                          </TableCell>
                          <TableCell className="text-xs font-mono font-medium text-right">{formatKES(app.amount)}</TableCell>
                          <TableCell>
                            <span className={`text-xs font-mono font-semibold ${app.riskScore > 700 ? "text-success" : app.riskScore > 600 ? "text-warning" : "text-destructive"}`}>
                              {app.riskScore}
                            </span>
                          </TableCell>
                          <TableCell>
                            <Badge variant="outline" className={`text-[10px] font-sans font-semibold ${stageConfig[app.stage].color}`}>
                              {stageConfig[app.stage].label}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-xs font-sans text-muted-foreground">{app.backendStatus}</TableCell>
                          <TableCell className="text-xs font-sans text-muted-foreground">{app.date}</TableCell>
                          <TableCell>
                            <Button variant="ghost" size="sm" className="h-7 text-[10px] font-sans">
                              <Eye className="h-3 w-3 mr-1" /> View
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  <div className="flex items-center justify-between px-4 py-3 border-t text-xs text-muted-foreground font-sans">
                    <span>Showing {Math.min(50, filteredApps.length)} of {filteredApps.length} applications</span>
                  </div>
                </>
              )}
            </CardContent>
          </Card>
        )}
      </div>

      {/* Application Detail Drawer */}
      <Sheet open={drawerOpen} onOpenChange={setDrawerOpen}>
        <SheetContent className="w-full sm:max-w-[720px] overflow-y-auto p-0">
          {selectedApp && (
            <ApplicationDetail
              app={selectedApp}
              onClose={() => setDrawerOpen(false)}
            />
          )}
        </SheetContent>
      </Sheet>

      {/* New Application Dialog */}
      <NewApplicationDialog open={newAppOpen} onOpenChange={setNewAppOpen} />
    </DashboardLayout>
  );
};

// ─── New Application Dialog ───
function NewApplicationDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (v: boolean) => void }) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [selectedCustomer, setSelectedCustomer] = useState<Customer | null>(null);
  const [customerSearch, setCustomerSearch] = useState("");
  const [productId, setProductId] = useState("");
  const [amount, setAmount] = useState("");
  const [tenor, setTenor] = useState("");
  const [purpose, setPurpose] = useState("");

  const customerId = selectedCustomer?.customerId ?? "";

  const customerResults = useQuery({
    queryKey: ["customers-search", customerSearch],
    queryFn: () => customerService.searchCustomers(customerSearch),
    enabled: customerSearch.trim().length >= 2 && !selectedCustomer,
    staleTime: 30_000,
  });

  const productsQuery = useQuery({
    queryKey: ["products", "loan-active"],
    queryFn: () => productService.listProducts(0, 100),
    enabled: open,
    staleTime: 60_000,
  });
  const loanProducts = (productsQuery.data?.content ?? []).filter(
    (p) => p.status === "ACTIVE" && (p.productType?.toUpperCase().includes("LOAN") ?? false)
  );

  const resetForm = () => {
    setSelectedCustomer(null); setCustomerSearch(""); setProductId("");
    setAmount(""); setTenor(""); setPurpose("");
  };

  const createMutation = useMutation({
    mutationFn: () =>
      loanOriginationService.createApplication({
        customerId,
        productId,
        requestedAmount: parseFloat(amount),
        tenorMonths: parseInt(tenor),
        purpose: purpose || undefined,
        currency: "KES",
      }),
    onSuccess: (app) => {
      toast({ title: "Application Created", description: `${formatKES(parseFloat(amount))} loan application created and ready for submission` });
      queryClient.invalidateQueries({ queryKey: ["loan-applications"] });
      onOpenChange(false);
      resetForm();
    },
    onError: (err: Error) => {
      toast({ title: "Create Failed", description: err.message, variant: "destructive" });
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="font-heading">New Loan Application</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 py-2">
          <div>
            <label className="text-xs font-sans font-medium">Customer</label>
            {selectedCustomer ? (
              <div className="mt-1 flex items-center justify-between rounded-md border px-3 py-2">
                <div className="min-w-0">
                  <p className="text-xs font-sans font-medium truncate">{selectedCustomer.firstName} {selectedCustomer.lastName}</p>
                  <p className="text-[10px] font-mono text-muted-foreground">{selectedCustomer.customerId}</p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 text-[10px] font-sans shrink-0"
                  onClick={() => { setSelectedCustomer(null); setCustomerSearch(""); }}
                >
                  Change
                </Button>
              </div>
            ) : (
              <div className="relative mt-1">
                <Input
                  className="text-xs font-sans"
                  placeholder="Search by name or ID..."
                  value={customerSearch}
                  onChange={e => setCustomerSearch(e.target.value)}
                />
                {customerSearch.trim().length >= 2 && (
                  <div className="absolute z-50 mt-1 max-h-48 w-full overflow-y-auto rounded-md border bg-popover shadow-md">
                    {customerResults.isLoading ? (
                      <div className="px-3 py-2 text-xs text-muted-foreground font-sans">Searching...</div>
                    ) : (customerResults.data?.length ?? 0) === 0 ? (
                      <div className="px-3 py-2 text-xs text-muted-foreground font-sans">No customers found</div>
                    ) : (
                      customerResults.data!.map((c) => (
                        <button
                          key={c.id}
                          type="button"
                          onClick={() => { setSelectedCustomer(c); setCustomerSearch(""); }}
                          className="flex w-full flex-col items-start px-3 py-2 text-left hover:bg-muted"
                        >
                          <span className="text-xs font-sans font-medium">{c.firstName} {c.lastName}</span>
                          <span className="text-[10px] font-mono text-muted-foreground">{c.customerId}</span>
                        </button>
                      ))
                    )}
                  </div>
                )}
              </div>
            )}
          </div>
          <div>
            <label className="text-xs font-sans font-medium">Product</label>
            <Select value={productId} onValueChange={setProductId}>
              <SelectTrigger className="mt-1 text-xs font-sans">
                <SelectValue placeholder={productsQuery.isLoading ? "Loading products..." : "Select a loan product"} />
              </SelectTrigger>
              <SelectContent>
                {loanProducts.length === 0 ? (
                  <div className="px-2 py-1.5 text-xs text-muted-foreground font-sans">No active loan products</div>
                ) : (
                  loanProducts.map((p) => (
                    <SelectItem key={p.id} value={p.id} className="text-xs font-sans">{p.name}</SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs font-sans font-medium">Amount (KES)</label>
              <Input type="number" className="mt-1 text-xs font-mono" placeholder="50000" value={amount} onChange={e => setAmount(e.target.value)} />
            </div>
            <div>
              <label className="text-xs font-sans font-medium">Tenor (months)</label>
              <Input type="number" className="mt-1 text-xs font-mono" placeholder="12" value={tenor} onChange={e => setTenor(e.target.value)} />
            </div>
          </div>
          <div>
            <label className="text-xs font-sans font-medium">Purpose</label>
            <Input className="mt-1 text-xs font-sans" placeholder="Working capital" value={purpose} onChange={e => setPurpose(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)} className="text-xs font-sans">Cancel</Button>
          <Button
            size="sm"
            onClick={() => createMutation.mutate()}
            disabled={!customerId || !productId || !amount || !tenor || createMutation.isPending}
            className="text-xs font-sans"
          >
            {createMutation.isPending ? "Creating..." : "Create Application"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ─── Application Detail Component ───
function ApplicationDetail({ app, onClose }: { app: LoanApplication; onClose: () => void }) {
  const [activeTab, setActiveTab] = useState("summary");
  const [approveAmount, setApproveAmount] = useState(app.amount.toString());
  const [approveRate, setApproveRate] = useState(app.interestRate?.toString() ?? "15");
  const [declineNotes, setDeclineNotes] = useState("");
  const [disburseOpen, setDisburseOpen] = useState(false);
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const status = (app.backendStatus ?? "").toUpperCase();
  // Human-friendly label for notifications — never show a raw UUID to the user.
  const who = app.customerName || `application #${String(app.id).slice(0, 8)}`;

  const submitMutation = useMutation({
    mutationFn: () => loanOriginationService.submitApplication(app.id),
    onSuccess: () => {
      toast({ title: "Application Submitted", description: `${who}'s application submitted for review` });
      queryClient.invalidateQueries({ queryKey: ["loan-applications"] });
      onClose();
    },
    onError: (err: Error) => {
      toast({ title: "Submit Failed", description: err.message, variant: "destructive" });
    },
  });

  const reviewMutation = useMutation({
    mutationFn: () => loanOriginationService.startReview(app.id),
    onSuccess: () => {
      toast({ title: "Review Started", description: `${who}'s application is now under review` });
      queryClient.invalidateQueries({ queryKey: ["loan-applications"] });
      onClose();
    },
    onError: (err: Error) => {
      toast({ title: "Start Review Failed", description: err.message, variant: "destructive" });
    },
  });

  const approveMutation = useMutation({
    mutationFn: () =>
      loanOriginationService.approveApplication(app.id, {
        approvedAmount: parseFloat(approveAmount),
        interestRate: parseFloat(approveRate),
      }),
    onSuccess: () => {
      toast({ title: "Application Approved", description: `${who}'s application approved for ${formatKES(parseFloat(approveAmount))}` });
      queryClient.invalidateQueries({ queryKey: ["loan-applications"] });
      onClose();
    },
    onError: (err: Error) => {
      toast({ title: "Approval Failed", description: err.message, variant: "destructive" });
    },
  });

  const declineMutation = useMutation({
    mutationFn: () =>
      loanOriginationService.rejectApplication(app.id, { reviewNotes: declineNotes }),
    onSuccess: () => {
      toast({ title: "Application Declined", description: `${who}'s application has been rejected` });
      queryClient.invalidateQueries({ queryKey: ["loan-applications"] });
      onClose();
    },
    onError: (err: Error) => {
      toast({ title: "Decline Failed", description: err.message, variant: "destructive" });
    },
  });

  const stageSteps = ["received", "kyc_pending", "under_assessment", "credit_committee", "approved"];
  const currentStepIdx = stageSteps.indexOf(app.stage);
  const progressPct = app.stage === "rejected" ? 0 : ((currentStepIdx + 1) / stageSteps.length) * 100;

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="p-5 border-b bg-muted/30">
        <div className="flex items-center gap-3 mb-3">
          <div className="h-12 w-12 rounded-full bg-primary/10 flex items-center justify-center text-lg font-semibold text-primary font-sans">
            {app.customerInitials}
          </div>
          <div className="flex-1">
            <h3 className="font-heading text-lg">{app.customerName}</h3>
            <div className="flex items-center gap-2 mt-0.5">
              <span className="text-xs font-mono text-muted-foreground">{app.id}</span>
              <Badge variant="outline" className={`text-[10px] font-sans font-semibold ${stageConfig[app.stage].color}`}>
                {stageConfig[app.stage].label}
              </Badge>
              <Badge variant="outline" className="text-[10px] font-mono">{app.backendStatus}</Badge>
            </div>
          </div>
        </div>
        <div className="space-y-1.5">
          <Progress value={progressPct} className="h-1.5" />
          <div className="flex justify-between text-[9px] text-muted-foreground font-sans">
            {stageSteps.map((s) => (
              <span key={s} className={s === app.stage ? "text-primary font-semibold" : ""}>
                {stageConfig[s as ApplicationStage].label}
              </span>
            ))}
          </div>
        </div>
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab} className="flex-1">
        <TabsList className="w-full justify-start rounded-none border-b bg-transparent h-10 px-5">
          <TabsTrigger value="summary" className="text-xs font-sans">Summary</TabsTrigger>
          <TabsTrigger value="credit" className="text-xs font-sans">Credit Assessment</TabsTrigger>
          <TabsTrigger value="documents" className="text-xs font-sans">Documents</TabsTrigger>
          <TabsTrigger value="decision" className="text-xs font-sans">Decision</TabsTrigger>
        </TabsList>

        <div className="p-5 overflow-y-auto">
          <TabsContent value="summary" className="mt-0 space-y-4">
            <div className="grid grid-cols-2 gap-4">
              {[
                { icon: User, label: "Customer", value: app.customerName },
                { icon: Phone, label: "Phone", value: app.phone },
                { icon: Hash, label: "National ID", value: app.nationalId },
                { icon: FileText, label: "Product", value: app.product },
                { icon: Calendar, label: "Application Date", value: app.date },
                { icon: MapPin, label: "Channel", value: app.channel },
              ].map((field) => (
                <div key={field.label} className="flex items-start gap-2.5">
                  <field.icon className="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
                  <div>
                    <p className="text-[10px] text-muted-foreground font-sans uppercase tracking-wider">{field.label}</p>
                    <p className="text-xs font-sans font-medium">{field.value}</p>
                  </div>
                </div>
              ))}
            </div>
            <div className="grid grid-cols-3 gap-3 mt-4">
              <Card className="p-3 text-center">
                <p className="text-[10px] text-muted-foreground font-sans mb-1">Loan Amount</p>
                <p className="text-sm font-mono font-bold">{formatKES(app.amount)}</p>
              </Card>
              <Card className="p-3 text-center">
                <p className="text-[10px] text-muted-foreground font-sans mb-1">Tenor</p>
                <p className="text-sm font-mono font-bold">{app.tenor} months</p>
              </Card>
              <Card className="p-3 text-center">
                <p className="text-[10px] text-muted-foreground font-sans mb-1">Purpose</p>
                <p className="text-sm font-sans font-semibold">{app.purpose}</p>
              </Card>
            </div>
          </TabsContent>

          <TabsContent value="credit" className="mt-0 space-y-4">
            <Card className="p-4">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <p className="text-[10px] text-muted-foreground font-sans uppercase tracking-wider">AthenaScore</p>
                  <p className="text-3xl font-mono font-bold mt-1">{app.compositeScore}</p>
                  <Badge variant="outline" className={`text-[10px] mt-1 ${
                    app.compositeScore > 750 ? "bg-success/15 text-success border-success/30" :
                    app.compositeScore > 680 ? "bg-info/15 text-info border-info/30" :
                    app.compositeScore > 600 ? "bg-warning/15 text-warning border-warning/30" :
                    "bg-destructive/15 text-destructive border-destructive/30"
                  }`}>
                    {app.compositeLabel}
                  </Badge>
                </div>
                <Brain className="h-10 w-10 text-accent" />
              </div>
              <Card className={`p-4 border-l-4 mt-4 ${app.aiRecommendation === "APPROVAL" ? "border-l-success bg-success/5" : "border-l-destructive bg-destructive/5"}`}>
                <div className="flex items-center gap-2">
                  <Brain className="h-5 w-5 text-accent" />
                  <p className="text-xs font-sans font-semibold">
                    {app.aiRecommendation === "APPROVAL" ? "AI recommends APPROVAL" : "AI recommends DECLINE"} — {app.aiConfidence}% confidence
                  </p>
                </div>
              </Card>
            </Card>
          </TabsContent>

          <TabsContent value="documents" className="mt-0">
            {app.documents.length === 0 ? (
              <div className="text-center py-8 text-xs text-muted-foreground font-sans">
                No documents available for this application.
              </div>
            ) : (
              <div className="space-y-3">
                {app.documents.map((doc) => (
                  <div key={doc.name} className="flex items-center justify-between p-3 border rounded-lg hover:bg-muted/30 transition-colors">
                    <div className="flex items-center gap-3">
                      {doc.status === "verified" ? (
                        <CheckCircle2 className="h-4 w-4 text-success" />
                      ) : doc.status === "pending" ? (
                        <AlertTriangle className="h-4 w-4 text-warning" />
                      ) : (
                        <XCircle className="h-4 w-4 text-destructive" />
                      )}
                      <div>
                        <p className="text-xs font-sans font-medium">{doc.name}</p>
                        <p className="text-[10px] text-muted-foreground font-sans capitalize">{doc.status}</p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </TabsContent>

          <TabsContent value="decision" className="mt-0 space-y-4">
            {status === "DRAFT" && (
              <Card className="p-4 space-y-3 border-l-4 border-l-info">
                <p className="text-xs font-sans font-semibold text-info">Submit Application</p>
                <p className="text-[11px] text-muted-foreground font-sans">
                  Submit this draft application to begin the review process.
                </p>
                <Button
                  className="w-full bg-info hover:bg-info/90 text-white font-sans text-xs"
                  onClick={() => submitMutation.mutate()}
                  disabled={submitMutation.isPending}
                >
                  <FileText className="h-4 w-4 mr-1.5" />
                  {submitMutation.isPending ? "Submitting..." : "Submit"}
                </Button>
              </Card>
            )}

            {status === "SUBMITTED" && (
              <Card className="p-4 space-y-3 border-l-4 border-l-accent">
                <p className="text-xs font-sans font-semibold">Start Review</p>
                <p className="text-[11px] text-muted-foreground font-sans">
                  Move this application into review for credit assessment.
                </p>
                <Button
                  className="w-full font-sans text-xs"
                  onClick={() => reviewMutation.mutate()}
                  disabled={reviewMutation.isPending}
                >
                  <Eye className="h-4 w-4 mr-1.5" />
                  {reviewMutation.isPending ? "Starting..." : "Start Review"}
                </Button>
              </Card>
            )}

            {status === "APPROVED" && (
              <Card className="p-4 space-y-3 border-l-4 border-l-success">
                <p className="text-xs font-sans font-semibold text-success">Disburse Loan</p>
                <p className="text-[11px] text-muted-foreground font-sans">
                  Approved for {formatKES(app.approvedAmount ?? app.amount)}. Disburse funds to the borrower's account.
                </p>
                <Button
                  className="w-full bg-success hover:bg-success/90 text-white font-sans text-xs"
                  onClick={() => setDisburseOpen(true)}
                >
                  <CheckCircle2 className="h-4 w-4 mr-1.5" />
                  Disburse
                </Button>
              </Card>
            )}

            {status === "UNDER_REVIEW" && (
              <>
                {/* Approve Section */}
                <Card className="p-4 space-y-3 border-l-4 border-l-success">
                  <p className="text-xs font-sans font-semibold text-success">Approve Application</p>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="text-[10px] text-muted-foreground font-sans uppercase tracking-wider">Approved Amount (KES)</label>
                      <Input
                        type="number"
                        className="mt-1 text-xs font-mono"
                        value={approveAmount}
                        onChange={e => setApproveAmount(e.target.value)}
                      />
                    </div>
                    <div>
                      <label className="text-[10px] text-muted-foreground font-sans uppercase tracking-wider">Interest Rate (%)</label>
                      <Input
                        type="number"
                        className="mt-1 text-xs font-mono"
                        value={approveRate}
                        onChange={e => setApproveRate(e.target.value)}
                      />
                    </div>
                  </div>
                  <Button
                    className="w-full bg-success hover:bg-success/90 text-white font-sans text-xs"
                    onClick={() => approveMutation.mutate()}
                    disabled={approveMutation.isPending}
                  >
                    <CheckCircle2 className="h-4 w-4 mr-1.5" />
                    {approveMutation.isPending ? "Approving..." : "Approve"}
                  </Button>
                </Card>

                {/* Decline Section */}
                <Card className="p-4 space-y-3 border-l-4 border-l-destructive">
                  <p className="text-xs font-sans font-semibold text-destructive">Decline Application</p>
                  <div>
                    <label className="text-[10px] text-muted-foreground font-sans uppercase tracking-wider">Reason / Notes</label>
                    <Textarea
                      placeholder="Enter decline reason..."
                      className="mt-1 text-xs font-sans"
                      rows={3}
                      value={declineNotes}
                      onChange={e => setDeclineNotes(e.target.value)}
                    />
                  </div>
                  <Button
                    variant="destructive"
                    className="w-full font-sans text-xs"
                    onClick={() => declineMutation.mutate()}
                    disabled={declineMutation.isPending}
                  >
                    <XCircle className="h-4 w-4 mr-1.5" />
                    {declineMutation.isPending ? "Declining..." : "Decline"}
                  </Button>
                </Card>
              </>
            )}

            {!["DRAFT", "SUBMITTED", "UNDER_REVIEW", "APPROVED"].includes(status) && (
              <div className="text-center py-8 text-muted-foreground">
                <p className="text-sm font-medium">No actions available</p>
                <p className="text-xs mt-1">This application is {app.backendStatus.toLowerCase()}.</p>
              </div>
            )}
          </TabsContent>
        </div>
      </Tabs>

      <DisburseDialog app={app} open={disburseOpen} onOpenChange={setDisburseOpen} onDone={onClose} />
    </div>
  );
}

// ─── Disburse Dialog ───
function DisburseDialog({
  app, open, onOpenChange, onDone,
}: {
  app: LoanApplication;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDone: () => void;
}) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [disbursedAmount, setDisbursedAmount] = useState((app.approvedAmount ?? app.amount).toString());
  const [accountId, setAccountId] = useState("");

  const customerQuery = useQuery({
    queryKey: ["customers-search", "resolve", app.customerId],
    queryFn: () => customerService.searchCustomers(app.customerId),
    enabled: open && !!app.customerId,
    staleTime: 60_000,
  });
  const internalCustomer =
    customerQuery.data?.find((c) => c.customerId === app.customerId) ?? customerQuery.data?.[0];

  const accountsQuery = useQuery({
    queryKey: ["customer-accounts", internalCustomer?.id],
    queryFn: () => accountService.getCustomerAccounts(internalCustomer!.id),
    enabled: open && !!internalCustomer?.id,
    staleTime: 30_000,
  });
  const accounts = accountsQuery.data ?? [];

  const disburseMutation = useMutation({
    mutationFn: () =>
      loanOriginationService.disburseApplication(app.id, {
        disbursedAmount: parseFloat(disbursedAmount),
        disbursementAccount: accountId,
      }),
    onSuccess: () => {
      toast({ title: "Loan Disbursed", description: `${app.customerName || "Loan"} disbursed ${formatKES(parseFloat(disbursedAmount))} — funds credited to the borrower's account` });
      queryClient.invalidateQueries({ queryKey: ["loan-applications"] });
      onOpenChange(false);
      onDone();
    },
    onError: (err: Error) => {
      toast({ title: "Disbursement Failed", description: err.message, variant: "destructive" });
    },
  });

  const accountsLoading = customerQuery.isLoading || accountsQuery.isLoading;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="font-heading">Disburse Loan</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 py-2">
          <div>
            <label className="text-xs font-sans font-medium">Disbursed Amount (KES)</label>
            <Input
              type="number"
              className="mt-1 text-xs font-mono"
              value={disbursedAmount}
              onChange={e => setDisbursedAmount(e.target.value)}
            />
          </div>
          <div>
            <label className="text-xs font-sans font-medium">Disbursement Account</label>
            <Select value={accountId} onValueChange={setAccountId}>
              <SelectTrigger className="mt-1 text-xs font-sans">
                <SelectValue placeholder={accountsLoading ? "Loading accounts..." : "Select account"} />
              </SelectTrigger>
              <SelectContent>
                {accounts.length === 0 ? (
                  <div className="px-2 py-1.5 text-xs text-muted-foreground font-sans">No accounts found</div>
                ) : (
                  accounts.map((a) => (
                    <SelectItem key={a.id} value={a.id} className="text-xs font-sans">
                      {a.accountNumber} · {a.accountType}
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)} className="text-xs font-sans">Cancel</Button>
          <Button
            size="sm"
            onClick={() => disburseMutation.mutate()}
            disabled={!disbursedAmount || !accountId || disburseMutation.isPending}
            className="text-xs font-sans"
          >
            {disburseMutation.isPending ? "Disbursing..." : "Disburse"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default LoansPage;
