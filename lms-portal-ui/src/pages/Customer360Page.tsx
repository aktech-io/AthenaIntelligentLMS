import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { formatKES } from "@/lib/format";
import {
  ArrowLeft, User, Wallet, FileText, Mail, Phone, MapPin, Shield,
  AlertTriangle, Network, Briefcase, Zap, CreditCard, ArrowUpDown,
  Download, Receipt, Pencil,
} from "lucide-react";
import { customerService, type UpdateCustomerRequest } from "@/services/customerService";
import { accountService, type BalanceResponse, type Transaction, type StatementResponse } from "@/services/accountService";
import { loanManagementService, type Installment, type Repayment } from "@/services/loanManagementService";
import { fraudService } from "@/services/fraudService";
import { overdraftService } from "@/services/overdraftService";
import { useToast } from "@/hooks/use-toast";

/* ── colour maps ─────────────────────────────────────────────────── */

const statusColors: Record<string, string> = {
  ACTIVE: "bg-success/15 text-success border-success/30",
  INACTIVE: "bg-muted text-muted-foreground border-border",
  SUSPENDED: "bg-destructive/15 text-destructive border-destructive/30",
  BLOCKED: "bg-destructive/15 text-destructive border-destructive/30",
  FROZEN: "bg-warning/15 text-warning border-warning/30",
  DORMANT: "bg-muted text-muted-foreground border-border",
  CLOSED: "bg-muted text-muted-foreground border-border",
};

const loanStatusCls: Record<string, string> = {
  ACTIVE: "bg-success/15 text-success border-success/30",
  DISBURSED: "bg-success/15 text-success border-success/30",
  CLOSED: "bg-muted text-muted-foreground border-border",
  DEFAULTED: "bg-destructive/15 text-destructive border-destructive/30",
  WRITTEN_OFF: "bg-destructive/15 text-destructive border-destructive/30",
};

const kycColors: Record<string, string> = {
  VERIFIED: "bg-success/15 text-success border-success/30",
  PENDING: "bg-warning/15 text-warning border-warning/30",
  REJECTED: "bg-destructive/15 text-destructive border-destructive/30",
};

const severityColors: Record<string, string> = {
  LOW: "bg-muted text-muted-foreground border-border",
  MEDIUM: "bg-warning/15 text-warning border-warning/30",
  HIGH: "bg-orange-100 text-orange-700 border-orange-300",
  CRITICAL: "bg-destructive/15 text-destructive border-destructive/30",
};

const riskColors: Record<string, string> = {
  LOW: "text-success",
  MEDIUM: "text-warning",
  HIGH: "text-orange-600",
  CRITICAL: "text-destructive",
};

const alertStatusColors: Record<string, string> = {
  OPEN: "bg-destructive/15 text-destructive border-destructive/30",
  UNDER_REVIEW: "bg-warning/15 text-warning border-warning/30",
  ESCALATED: "bg-orange-100 text-orange-700 border-orange-300",
  CONFIRMED_FRAUD: "bg-destructive/15 text-destructive border-destructive/30",
  FALSE_POSITIVE: "bg-muted text-muted-foreground border-border",
  CLOSED: "bg-muted text-muted-foreground border-border",
};

const installmentStatusCls: Record<string, string> = {
  PAID: "bg-success/15 text-success border-success/30",
  DUE: "bg-warning/15 text-warning border-warning/30",
  OVERDUE: "bg-destructive/15 text-destructive border-destructive/30",
  FUTURE: "bg-muted text-muted-foreground border-border",
};

/* ── helpers ─────────────────────────────────────────────────────── */

const safeDate = (v: string | undefined | null) =>
  v ? new Date(v).toLocaleDateString() : "—";

const safeDateFull = (v: string | undefined | null) =>
  v ? new Date(v).toLocaleString() : "—";

/* ── page ────────────────────────────────────────────────────────── */

const Customer360Page = () => {
  const { customerId } = useParams<{ customerId: string }>();
  const navigate = useNavigate();
  const { toast } = useToast();
  const qc = useQueryClient();

  // Edit dialog state
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState<UpdateCustomerRequest>({});

  // Statement controls
  const [stmtAccountId, setStmtAccountId] = useState<string | null>(null);
  const [stmtType, setStmtType] = useState<"full" | "mini">("mini");
  const [stmtFrom, setStmtFrom] = useState(() => {
    const d = new Date();
    d.setMonth(d.getMonth() - 1);
    return d.toISOString().slice(0, 10);
  });
  const [stmtTo, setStmtTo] = useState(() => new Date().toISOString().slice(0, 10));

  /* ── queries ──────────────────────────────────────────────────── */

  const { data: customer, isLoading: custLoading } = useQuery({
    queryKey: ["customer", customerId],
    queryFn: () => customerService.getCustomer(customerId!),
    enabled: !!customerId,
  });

  const { data: accounts } = useQuery({
    queryKey: ["customer-accounts", customer?.customerId],
    queryFn: () => accountService.getCustomerAccounts(customer!.customerId),
    enabled: !!customer?.customerId,
  });

  // Fetch balances for each account
  const { data: balances } = useQuery({
    queryKey: ["customer-balances", accounts?.map((a) => a.id)],
    queryFn: async () => {
      if (!accounts || accounts.length === 0) return [] as BalanceResponse[];
      const results = await Promise.allSettled(
        accounts.map((a) => accountService.getBalance(a.id))
      );
      return results.map((r) => (r.status === "fulfilled" ? r.value : null));
    },
    enabled: !!accounts && accounts.length > 0,
  });

  // Transactions across all accounts
  const { data: txnData } = useQuery({
    queryKey: ["customer-transactions", accounts?.map((a) => a.id)],
    queryFn: async () => {
      if (!accounts || accounts.length === 0) return [] as (Transaction & { accountNumber: string })[];
      const results = await Promise.allSettled(
        accounts.map((a) =>
          accountService.getTransactions(a.id, 0, 20).then((page) =>
            (page.content ?? []).map((t) => ({ ...t, accountNumber: a.accountNumber }))
          )
        )
      );
      const all = results.flatMap((r) => (r.status === "fulfilled" ? r.value : []));
      return all.sort((a, b) => (b.createdAt ?? "").localeCompare(a.createdAt ?? ""));
    },
    enabled: !!accounts && accounts.length > 0,
  });

  const { data: loansData } = useQuery({
    queryKey: ["customer-loans", customer?.customerId],
    queryFn: () => loanManagementService.getLoansByCustomer(customer!.customerId),
    enabled: !!customer?.customerId,
  });

  // Repayments for all loans
  const { data: repaymentsData } = useQuery({
    queryKey: ["customer-repayments", loansData?.map((l) => l.id)],
    queryFn: async () => {
      if (!loansData || loansData.length === 0) return [] as (Repayment & { loanId: string })[];
      const results = await Promise.allSettled(
        loansData.map((l) =>
          loanManagementService.getLoanRepayments(l.id).then((reps) =>
            reps.map((r) => ({ ...r, loanId: l.id }))
          )
        )
      );
      const all = results.flatMap((r) => (r.status === "fulfilled" ? r.value : []));
      return all.sort((a, b) => (b.paymentDate ?? "").localeCompare(a.paymentDate ?? ""));
    },
    enabled: !!loansData && loansData.length > 0,
  });

  // Loan schedules
  const [selectedLoanId, setSelectedLoanId] = useState<string | null>(null);
  const { data: scheduleData } = useQuery({
    queryKey: ["loan-schedule", selectedLoanId],
    queryFn: () => loanManagementService.getLoanSchedule(selectedLoanId!),
    enabled: !!selectedLoanId,
  });

  // Wallet & Overdraft
  const { data: wallet } = useQuery({
    queryKey: ["customer-wallet", customer?.customerId],
    queryFn: () => overdraftService.getWalletByCustomer(customer!.customerId),
    enabled: !!customer?.customerId,
    retry: false,
  });

  const { data: overdraftFacility } = useQuery({
    queryKey: ["customer-overdraft", wallet?.id],
    queryFn: () => overdraftService.getFacility(wallet!.id),
    enabled: !!wallet?.id,
    retry: false,
  });

  const { data: walletTxns } = useQuery({
    queryKey: ["customer-wallet-txns", wallet?.id],
    queryFn: () => overdraftService.getTransactions(wallet!.id, 0, 20),
    enabled: !!wallet?.id,
    retry: false,
  });

  const { data: interestCharges } = useQuery({
    queryKey: ["customer-interest-charges", wallet?.id],
    queryFn: () => overdraftService.getCharges(wallet!.id),
    enabled: !!wallet?.id,
    retry: false,
  });

  // Statement
  const { data: statementData, isFetching: stmtFetching } = useQuery({
    queryKey: ["customer-statement", stmtAccountId, stmtType, stmtFrom, stmtTo],
    queryFn: () => accountService.getStatement(stmtAccountId!, stmtFrom, stmtTo, 0, 100),
    enabled: !!stmtAccountId && stmtType === "full",
    retry: false,
  });

  const { data: miniStatementData, isFetching: miniStmtFetching } = useQuery({
    queryKey: ["customer-mini-statement", stmtAccountId],
    queryFn: async () => {
      // mini-statement endpoint returns last N transactions
      const result = await accountService.getTransactions(stmtAccountId!, 0, 10);
      return result;
    },
    enabled: !!stmtAccountId && stmtType === "mini",
    retry: false,
  });

  // Fraud
  const { data: riskProfile } = useQuery({
    queryKey: ["customer-risk", customer?.customerId],
    queryFn: () => fraudService.getCustomerRisk(customer!.customerId),
    enabled: !!customer?.customerId,
    retry: false,
  });

  const { data: alertsData } = useQuery({
    queryKey: ["customer-alerts", customer?.customerId],
    queryFn: () => fraudService.listCustomerAlerts(customer!.customerId, 0, 50),
    enabled: !!customer?.customerId,
    retry: false,
  });

  const { data: networkData } = useQuery({
    queryKey: ["customer-network", customer?.customerId],
    queryFn: () => fraudService.getCustomerNetwork(customer!.customerId),
    enabled: !!customer?.customerId,
    retry: false,
  });

  const { data: casesData } = useQuery({
    queryKey: ["customer-fraud-cases", customer?.customerId],
    queryFn: () => fraudService.listCases(0, 50),
    enabled: !!customer?.customerId,
    retry: false,
  });

  const { data: scoringHistoryData } = useQuery({
    queryKey: ["customer-scoring-history", customer?.customerId],
    queryFn: () => fraudService.getCustomerScoringHistory(customer!.customerId, 0, 20),
    enabled: !!customer?.customerId,
    retry: false,
  });

  /* ── mutations ────────────────────────────────────────────────── */

  const updateMutation = useMutation({
    mutationFn: (req: UpdateCustomerRequest) =>
      customerService.updateCustomer(customerId!, req),
    onSuccess: () => {
      toast({ title: "Customer updated" });
      qc.invalidateQueries({ queryKey: ["customer", customerId] });
      setEditOpen(false);
    },
    onError: (err: Error) => {
      toast({ title: "Update failed", description: err.message, variant: "destructive" });
    },
  });

  const statusMutation = useMutation({
    mutationFn: (status: string) => customerService.updateStatus(customerId!, status),
    onSuccess: () => {
      toast({ title: "Status updated" });
      qc.invalidateQueries({ queryKey: ["customer", customerId] });
    },
    onError: (err: Error) => {
      toast({ title: "Status update failed", description: err.message, variant: "destructive" });
    },
  });

  /* ── derived data ─────────────────────────────────────────────── */

  const customerLoans = loansData ?? [];
  const customerAlerts = alertsData?.content ?? [];
  const customerCases = (casesData?.content ?? []).filter(
    (c) => c.customerId === customer?.customerId
  );
  const scoringHistory = scoringHistoryData?.content ?? [];
  const allTransactions = txnData ?? [];
  const allRepayments = repaymentsData ?? [];

  const totalBalance = (balances ?? []).reduce(
    (sum, b) => sum + (b?.availableBalance ?? 0), 0
  );

  /* ── statement download helper ────────────────────────────────── */
  const downloadStatementCSV = (stmt: StatementResponse) => {
    const rows = [
      ["Date", "Type", "Amount", "Balance", "Reference", "Description"],
      ...(stmt.transactions?.content ?? []).map((t) => [
        t.createdAt ?? t.valueDate,
        t.transactionType,
        String(t.amount),
        String(t.runningBalance ?? ""),
        t.reference ?? "",
        t.description ?? "",
      ]),
    ];
    const csv = rows.map((r) => r.map((c) => `"${c}"`).join(",")).join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `statement_${stmt.accountNumber}_${stmtFrom}_${stmtTo}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  /* ── render ───────────────────────────────────────────────────── */

  if (!customerId) {
    return (
      <DashboardLayout title="Customer 360" subtitle="Customer detail view"
        breadcrumbs={[{ label: "Home", href: "/" }, { label: "Customers", href: "/borrowers" }]}>
        <Card>
          <CardContent className="p-8 text-center text-muted-foreground font-sans">
            Enter a customer ID to view details.
          </CardContent>
        </Card>
      </DashboardLayout>
    );
  }

  return (
    <DashboardLayout title="Customer 360"
      subtitle={customer ? `${customer.firstName} ${customer.lastName}` : "Loading..."}
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Customers", href: "/borrowers" },
        { label: customer?.customerId ?? customerId },
      ]}>
      <div className="space-y-4">
        <Button variant="ghost" size="sm" className="text-xs font-sans"
          onClick={() => navigate("/borrowers")}>
          <ArrowLeft className="h-3.5 w-3.5 mr-1" /> Back to Directory
        </Button>

        {custLoading ? (
          <div className="flex items-center justify-center h-32 text-muted-foreground text-xs">
            Loading customer...
          </div>
        ) : !customer ? (
          <div className="flex items-center justify-center h-32 text-destructive text-xs">
            Customer not found.
          </div>
        ) : (
          <>
            {/* Profile Card */}
            <Card>
              <CardContent className="p-5">
                <div className="flex items-start gap-5">
                  <div className="h-14 w-14 rounded-full bg-primary/10 flex items-center justify-center shrink-0">
                    <User className="h-7 w-7 text-primary" />
                  </div>
                  <div className="flex-1 grid grid-cols-1 sm:grid-cols-3 gap-4">
                    <div>
                      <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Name</p>
                      <p className="text-sm font-semibold mt-0.5">{customer.firstName} {customer.lastName}</p>
                    </div>
                    <div>
                      <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Customer ID</p>
                      <p className="text-sm font-mono mt-0.5">{customer.customerId}</p>
                    </div>
                    <div>
                      <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Type</p>
                      <p className="text-sm mt-0.5">{customer.customerType}</p>
                    </div>
                    {customer.phone && (
                      <div className="flex items-center gap-1.5">
                        <Phone className="h-3 w-3 text-muted-foreground" />
                        <span className="text-xs">{customer.phone}</span>
                      </div>
                    )}
                    {customer.email && (
                      <div className="flex items-center gap-1.5">
                        <Mail className="h-3 w-3 text-muted-foreground" />
                        <span className="text-xs">{customer.email}</span>
                      </div>
                    )}
                    {customer.address && (
                      <div className="flex items-center gap-1.5">
                        <MapPin className="h-3 w-3 text-muted-foreground" />
                        <span className="text-xs">{customer.address}</span>
                      </div>
                    )}
                  </div>
                  <div className="flex flex-col items-end gap-2">
                    <Badge variant="outline"
                      className={`text-[11px] font-semibold ${statusColors[customer.status] ?? ""}`}>
                      {customer.status}
                    </Badge>
                    <Badge variant="outline"
                      className={`text-[11px] font-semibold ${kycColors[customer.kycStatus ?? ""] ?? ""}`}>
                      <Shield className="h-2.5 w-2.5 mr-1" />
                      KYC: {customer.kycStatus ?? "—"}
                    </Badge>
                    {riskProfile && (
                      <Badge variant="outline"
                        className={`text-[11px] font-semibold ${severityColors[riskProfile.riskLevel] ?? ""}`}>
                        <AlertTriangle className="h-2.5 w-2.5 mr-1" />
                        Risk: {riskProfile.riskLevel}
                      </Badge>
                    )}
                    <Button variant="outline" size="sm" className="text-[11px] h-7 mt-1"
                      onClick={() => {
                        setEditForm({
                          firstName: customer.firstName,
                          lastName: customer.lastName,
                          email: customer.email ?? "",
                          phone: customer.phone ?? "",
                          address: customer.address ?? "",
                          nationalId: customer.nationalId ?? "",
                          dateOfBirth: customer.dateOfBirth ?? "",
                          gender: customer.gender ?? "",
                        });
                        setEditOpen(true);
                      }}>
                      <Pencil className="h-2.5 w-2.5 mr-1" /> Edit
                    </Button>
                  </div>
                </div>
                {/* Balance summary */}
                {accounts && accounts.length > 0 && (
                  <div className="mt-4 pt-3 border-t flex items-center gap-6">
                    <div>
                      <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Total Available Balance</p>
                      <p className="text-lg font-bold font-mono">{formatKES(totalBalance)}</p>
                    </div>
                    <div>
                      <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Accounts</p>
                      <p className="text-lg font-bold font-mono">{accounts.length}</p>
                    </div>
                    <div>
                      <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Active Loans</p>
                      <p className="text-lg font-bold font-mono">
                        {customerLoans.filter((l) => l.status === "ACTIVE" || l.status === "DISBURSED").length}
                      </p>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Edit Dialog */}
            <Dialog open={editOpen} onOpenChange={setEditOpen}>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Edit Customer</DialogTitle>
                </DialogHeader>
                <div className="grid gap-3 py-2">
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label className="text-xs">First Name</Label>
                      <Input className="text-xs mt-1" value={editForm.firstName ?? ""}
                        onChange={(e) => setEditForm({ ...editForm, firstName: e.target.value })} />
                    </div>
                    <div>
                      <Label className="text-xs">Last Name</Label>
                      <Input className="text-xs mt-1" value={editForm.lastName ?? ""}
                        onChange={(e) => setEditForm({ ...editForm, lastName: e.target.value })} />
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label className="text-xs">Email</Label>
                      <Input className="text-xs mt-1" value={editForm.email ?? ""}
                        onChange={(e) => setEditForm({ ...editForm, email: e.target.value })} />
                    </div>
                    <div>
                      <Label className="text-xs">Phone</Label>
                      <Input className="text-xs mt-1" value={editForm.phone ?? ""}
                        onChange={(e) => setEditForm({ ...editForm, phone: e.target.value })} />
                    </div>
                  </div>
                  <div>
                    <Label className="text-xs">Address</Label>
                    <Input className="text-xs mt-1" value={editForm.address ?? ""}
                      onChange={(e) => setEditForm({ ...editForm, address: e.target.value })} />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label className="text-xs">National ID</Label>
                      <Input className="text-xs mt-1" value={editForm.nationalId ?? ""}
                        onChange={(e) => setEditForm({ ...editForm, nationalId: e.target.value })} />
                    </div>
                    <div>
                      <Label className="text-xs">Date of Birth</Label>
                      <Input type="date" className="text-xs mt-1" value={editForm.dateOfBirth ?? ""}
                        onChange={(e) => setEditForm({ ...editForm, dateOfBirth: e.target.value })} />
                    </div>
                  </div>
                  <div className="flex items-center justify-between mt-2">
                    <div className="flex gap-2">
                      {customer.status !== "SUSPENDED" && (
                        <Button size="sm" variant="destructive" className="text-xs"
                          onClick={() => statusMutation.mutate("SUSPENDED")}>
                          Suspend
                        </Button>
                      )}
                      {customer.status === "SUSPENDED" && (
                        <Button size="sm" variant="outline" className="text-xs"
                          onClick={() => statusMutation.mutate("ACTIVE")}>
                          Reactivate
                        </Button>
                      )}
                    </div>
                    <Button size="sm" className="text-xs"
                      disabled={updateMutation.isPending}
                      onClick={() => updateMutation.mutate(editForm)}>
                      {updateMutation.isPending ? "Saving..." : "Save Changes"}
                    </Button>
                  </div>
                </div>
              </DialogContent>
            </Dialog>

            <Tabs defaultValue="accounts" className="w-full">
              <TabsList className="flex-wrap h-auto gap-1">
                <TabsTrigger value="accounts" className="text-xs">Accounts</TabsTrigger>
                <TabsTrigger value="transactions" className="text-xs">Transactions</TabsTrigger>
                <TabsTrigger value="loans" className="text-xs">Loans ({customerLoans.length})</TabsTrigger>
                <TabsTrigger value="payments" className="text-xs">Payments</TabsTrigger>
                <TabsTrigger value="wallet" className="text-xs">Wallet & Overdraft</TabsTrigger>
                <TabsTrigger value="statements" className="text-xs">Statements</TabsTrigger>
                <TabsTrigger value="fraud" className="text-xs">
                  Fraud & Risk
                  {riskProfile && (riskProfile.riskLevel === "HIGH" || riskProfile.riskLevel === "CRITICAL") && (
                    <span className="ml-1.5 h-2 w-2 rounded-full bg-destructive inline-block" />
                  )}
                </TabsTrigger>
              </TabsList>

              {/* ─── Accounts Tab ───────────────────────────────── */}
              <TabsContent value="accounts">
                <ErrorBoundary>
                  <Card>
                    <CardHeader className="pb-2">
                      <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                        <Wallet className="h-3.5 w-3.5" /> Accounts ({accounts?.length ?? 0})
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="p-0">
                      {!accounts || accounts.length === 0 ? (
                        <div className="p-4 text-center text-xs text-muted-foreground">
                          No accounts linked to this customer.
                        </div>
                      ) : (
                        <Table>
                          <TableHeader>
                            <TableRow className="hover:bg-transparent">
                              <TableHead className="text-[11px] uppercase tracking-wider">Account Number</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Type</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Currency</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Balance</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Available</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Status</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Created</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {accounts.map((acc, idx) => {
                              const bal = balances?.[idx];
                              return (
                                <TableRow key={acc.id} className="table-row-hover">
                                  <TableCell className="text-xs font-mono">{acc.accountNumber}</TableCell>
                                  <TableCell className="text-xs">{acc.accountType}</TableCell>
                                  <TableCell className="text-xs font-mono">{acc.currency}</TableCell>
                                  <TableCell className="text-xs font-mono text-right">
                                    {bal ? formatKES(bal.currentBalance) : "—"}
                                  </TableCell>
                                  <TableCell className="text-xs font-mono text-right">
                                    {bal ? formatKES(bal.availableBalance) : "—"}
                                  </TableCell>
                                  <TableCell>
                                    <Badge variant="outline"
                                      className={`text-[11px] ${statusColors[acc.status] ?? ""}`}>
                                      {acc.status}
                                    </Badge>
                                  </TableCell>
                                  <TableCell className="text-xs text-muted-foreground">
                                    {safeDate(acc.createdAt)}
                                  </TableCell>
                                </TableRow>
                              );
                            })}
                            {/* Totals row */}
                            <TableRow className="bg-muted/40 font-semibold">
                              <TableCell className="text-xs" colSpan={3}>Total</TableCell>
                              <TableCell className="text-xs font-mono text-right">
                                {formatKES((balances ?? []).reduce((s, b) => s + (b?.currentBalance ?? 0), 0))}
                              </TableCell>
                              <TableCell className="text-xs font-mono text-right">
                                {formatKES(totalBalance)}
                              </TableCell>
                              <TableCell colSpan={2} />
                            </TableRow>
                          </TableBody>
                        </Table>
                      )}
                    </CardContent>
                  </Card>
                </ErrorBoundary>
              </TabsContent>

              {/* ─── Transactions Tab ──────────────────────────── */}
              <TabsContent value="transactions">
                <ErrorBoundary>
                  <Card>
                    <CardHeader className="pb-2">
                      <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                        <ArrowUpDown className="h-3.5 w-3.5" /> Recent Transactions ({allTransactions.length})
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="p-0">
                      {allTransactions.length === 0 ? (
                        <div className="p-4 text-center text-xs text-muted-foreground">
                          No transactions found.
                        </div>
                      ) : (
                        <Table>
                          <TableHeader>
                            <TableRow className="hover:bg-transparent">
                              <TableHead className="text-[11px] uppercase tracking-wider">Date</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Account</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Type</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Amount</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Balance After</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Reference</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {allTransactions.slice(0, 50).map((t) => (
                              <TableRow key={t.id} className="table-row-hover">
                                <TableCell className="text-xs text-muted-foreground">
                                  {safeDateFull(t.createdAt ?? t.valueDate)}
                                </TableCell>
                                <TableCell className="text-xs font-mono">{(t as any).accountNumber ?? "—"}</TableCell>
                                <TableCell className="text-xs">{t.transactionType}</TableCell>
                                <TableCell className={`text-xs font-mono text-right ${
                                  t.transactionType === "CREDIT" ? "text-success" : "text-destructive"
                                }`}>
                                  {t.transactionType === "CREDIT" ? "+" : "-"}{formatKES(t.amount ?? 0)}
                                </TableCell>
                                <TableCell className="text-xs font-mono text-right">
                                  {t.runningBalance != null ? formatKES(t.runningBalance) : "—"}
                                </TableCell>
                                <TableCell className="text-xs font-mono text-muted-foreground truncate max-w-[150px]">
                                  {t.reference ?? "—"}
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      )}
                    </CardContent>
                  </Card>
                </ErrorBoundary>
              </TabsContent>

              {/* ─── Loans Tab ─────────────────────────────────── */}
              <TabsContent value="loans">
                <ErrorBoundary>
                  <Card>
                    <CardHeader className="pb-2">
                      <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                        <FileText className="h-3.5 w-3.5" /> Loans ({customerLoans.length})
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="p-0">
                      {customerLoans.length === 0 ? (
                        <div className="p-4 text-center text-xs text-muted-foreground">
                          No loans associated with this customer.
                        </div>
                      ) : (
                        <Table>
                          <TableHeader>
                            <TableRow className="hover:bg-transparent">
                              <TableHead className="text-[11px] uppercase tracking-wider">Loan ID</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Product</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Disbursed</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Outstanding</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-center">DPD</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Status</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider" />
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {customerLoans.map((loan) => (
                              <TableRow key={loan.id} className="table-row-hover cursor-pointer"
                                onClick={() => navigate(`/loan/${loan.id}`)}>
                                <TableCell className="text-xs font-mono text-info">{loan.id}</TableCell>
                                <TableCell className="text-xs text-muted-foreground">{loan.productId ?? "—"}</TableCell>
                                <TableCell className="text-xs font-mono text-right">{formatKES(loan.disbursedAmount ?? 0)}</TableCell>
                                <TableCell className="text-xs font-mono text-right">{formatKES(loan.outstandingPrincipal ?? 0)}</TableCell>
                                <TableCell className={`text-xs font-mono text-center font-semibold ${
                                  (loan.dpd ?? 0) > 30 ? "text-destructive" : (loan.dpd ?? 0) > 0 ? "text-warning" : "text-foreground"
                                }`}>{loan.dpd ?? 0}</TableCell>
                                <TableCell>
                                  <Badge variant="outline"
                                    className={`text-[11px] capitalize ${loanStatusCls[loan.status] ?? "bg-muted text-muted-foreground border-border"}`}>
                                    {loan.status}
                                  </Badge>
                                </TableCell>
                                <TableCell>
                                  <Button variant="ghost" size="sm" className="text-[11px] h-6"
                                    onClick={(e) => { e.stopPropagation(); setSelectedLoanId(loan.id); }}>
                                    Schedule
                                  </Button>
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      )}
                    </CardContent>
                  </Card>

                  {/* Schedule for selected loan */}
                  {selectedLoanId && scheduleData && (
                    <Card className="mt-4">
                      <CardHeader className="pb-2 flex flex-row items-center justify-between">
                        <CardTitle className="text-xs font-semibold uppercase tracking-wider">
                          Repayment Schedule — Loan {selectedLoanId.slice(0, 8)}...
                        </CardTitle>
                        <Button variant="ghost" size="sm" className="text-[11px] h-6"
                          onClick={() => setSelectedLoanId(null)}>Close</Button>
                      </CardHeader>
                      <CardContent className="p-0">
                        <Table>
                          <TableHeader>
                            <TableRow className="hover:bg-transparent">
                              <TableHead className="text-[11px] uppercase tracking-wider">#</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Due Date</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Principal</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Interest</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Total</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Status</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {scheduleData.map((inst: Installment) => (
                              <TableRow key={inst.installmentNo} className="table-row-hover">
                                <TableCell className="text-xs font-mono">{inst.installmentNo}</TableCell>
                                <TableCell className="text-xs">{safeDate(inst.dueDate)}</TableCell>
                                <TableCell className="text-xs font-mono text-right">{formatKES(inst.principalDue ?? 0)}</TableCell>
                                <TableCell className="text-xs font-mono text-right">{formatKES(inst.interestDue ?? 0)}</TableCell>
                                <TableCell className="text-xs font-mono text-right font-semibold">{formatKES(inst.totalDue ?? 0)}</TableCell>
                                <TableCell>
                                  <Badge variant="outline"
                                    className={`text-[11px] ${installmentStatusCls[inst.status] ?? "bg-muted text-muted-foreground border-border"}`}>
                                    {inst.status}
                                  </Badge>
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </CardContent>
                    </Card>
                  )}
                </ErrorBoundary>
              </TabsContent>

              {/* ─── Payments Tab ──────────────────────────────── */}
              <TabsContent value="payments">
                <ErrorBoundary>
                  <Card>
                    <CardHeader className="pb-2">
                      <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                        <Receipt className="h-3.5 w-3.5" /> Loan Repayment History ({allRepayments.length})
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="p-0">
                      {allRepayments.length === 0 ? (
                        <div className="p-4 text-center text-xs text-muted-foreground">
                          No repayment records found.
                        </div>
                      ) : (
                        <Table>
                          <TableHeader>
                            <TableRow className="hover:bg-transparent">
                              <TableHead className="text-[11px] uppercase tracking-wider">Date</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Loan</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider text-right">Amount</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Method</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Reference</TableHead>
                              <TableHead className="text-[11px] uppercase tracking-wider">Status</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {allRepayments.map((r) => (
                              <TableRow key={r.id} className="table-row-hover">
                                <TableCell className="text-xs">{safeDate(r.paymentDate)}</TableCell>
                                <TableCell className="text-xs font-mono text-info cursor-pointer"
                                  onClick={() => navigate(`/loan/${r.loanId}`)}>
                                  {r.loanId.slice(0, 8)}...
                                </TableCell>
                                <TableCell className="text-xs font-mono text-right text-success">
                                  +{formatKES(r.amount ?? 0)}
                                </TableCell>
                                <TableCell className="text-xs">{r.paymentMethod ?? "—"}</TableCell>
                                <TableCell className="text-xs font-mono text-muted-foreground truncate max-w-[150px]">
                                  {r.reference ?? "—"}
                                </TableCell>
                                <TableCell>
                                  <Badge variant="outline" className="text-[11px] bg-success/15 text-success border-success/30">
                                    {r.status ?? "COMPLETED"}
                                  </Badge>
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      )}
                    </CardContent>
                  </Card>
                </ErrorBoundary>
              </TabsContent>

              {/* ─── Wallet & Overdraft Tab ────────────────────── */}
              <TabsContent value="wallet">
                <ErrorBoundary>
                  <div className="space-y-4">
                    {/* Wallet */}
                    <Card>
                      <CardHeader className="pb-2">
                        <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                          <Wallet className="h-3.5 w-3.5" /> Digital Wallet
                        </CardTitle>
                      </CardHeader>
                      <CardContent>
                        {!wallet ? (
                          <p className="text-xs text-muted-foreground">No wallet found for this customer.</p>
                        ) : (
                          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Account</p>
                              <p className="text-sm font-mono">{wallet.accountNumber}</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Current Balance</p>
                              <p className="text-lg font-bold font-mono">{formatKES(wallet.currentBalance ?? 0)}</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Available</p>
                              <p className="text-lg font-bold font-mono">{formatKES(wallet.availableBalance ?? 0)}</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Status</p>
                              <Badge variant="outline" className={`text-[11px] mt-1 ${statusColors[wallet.status] ?? ""}`}>
                                {wallet.status}
                              </Badge>
                            </div>
                          </div>
                        )}
                      </CardContent>
                    </Card>

                    {/* Overdraft Facility */}
                    {overdraftFacility && (
                      <Card>
                        <CardHeader className="pb-2">
                          <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                            <CreditCard className="h-3.5 w-3.5" /> Overdraft Facility
                          </CardTitle>
                        </CardHeader>
                        <CardContent>
                          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Approved Limit</p>
                              <p className="text-lg font-bold font-mono">{formatKES(overdraftFacility.approvedLimit ?? 0)}</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Drawn</p>
                              <p className="text-lg font-bold font-mono text-warning">{formatKES(overdraftFacility.drawnAmount ?? 0)}</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Available</p>
                              <p className="text-lg font-bold font-mono text-success">{formatKES(overdraftFacility.availableOverdraft ?? 0)}</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Interest Rate</p>
                              <p className="text-sm font-mono">{(overdraftFacility.interestRate ?? 0).toFixed(2)}%</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Credit Band</p>
                              <Badge variant="outline" className="text-[11px]">{overdraftFacility.creditBand}</Badge>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Status</p>
                              <Badge variant="outline" className={`text-[11px] ${statusColors[overdraftFacility.status] ?? ""}`}>
                                {overdraftFacility.status}
                              </Badge>
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    )}

                    {/* Wallet Transactions */}
                    {wallet && (
                      <Card>
                        <CardHeader className="pb-2">
                          <CardTitle className="text-xs font-semibold uppercase tracking-wider">
                            Recent Wallet Transactions
                          </CardTitle>
                        </CardHeader>
                        <CardContent className="p-0">
                          {!walletTxns?.content?.length ? (
                            <div className="p-4 text-center text-xs text-muted-foreground">
                              No wallet transactions.
                            </div>
                          ) : (
                            <Table>
                              <TableHeader>
                                <TableRow className="hover:bg-transparent">
                                  <TableHead className="text-[11px] uppercase tracking-wider">Date</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider">Type</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider text-right">Amount</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider text-right">Balance After</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider">Reference</TableHead>
                                </TableRow>
                              </TableHeader>
                              <TableBody>
                                {walletTxns.content.map((wt) => (
                                  <TableRow key={wt.id} className="table-row-hover">
                                    <TableCell className="text-xs">{safeDateFull(wt.createdAt)}</TableCell>
                                    <TableCell className="text-xs">{wt.transactionType}</TableCell>
                                    <TableCell className={`text-xs font-mono text-right ${
                                      wt.transactionType === "DEPOSIT" ? "text-success" : "text-destructive"
                                    }`}>
                                      {wt.transactionType === "DEPOSIT" ? "+" : "-"}{formatKES(wt.amount ?? 0)}
                                    </TableCell>
                                    <TableCell className="text-xs font-mono text-right">{formatKES(wt.balanceAfter ?? 0)}</TableCell>
                                    <TableCell className="text-xs font-mono text-muted-foreground truncate max-w-[150px]">
                                      {wt.reference ?? "—"}
                                    </TableCell>
                                  </TableRow>
                                ))}
                              </TableBody>
                            </Table>
                          )}
                        </CardContent>
                      </Card>
                    )}

                    {/* Interest Charges */}
                    {interestCharges && interestCharges.length > 0 && (
                      <Card>
                        <CardHeader className="pb-2">
                          <CardTitle className="text-xs font-semibold uppercase tracking-wider">
                            Interest Charges
                          </CardTitle>
                        </CardHeader>
                        <CardContent className="p-0">
                          <Table>
                            <TableHeader>
                              <TableRow className="hover:bg-transparent">
                                <TableHead className="text-[11px] uppercase tracking-wider">Date</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider text-right">Drawn</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider text-right">Rate</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider text-right">Interest</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Ref</TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {interestCharges.map((ch) => (
                                <TableRow key={ch.id} className="table-row-hover">
                                  <TableCell className="text-xs">{safeDate(ch.chargeDate)}</TableCell>
                                  <TableCell className="text-xs font-mono text-right">{formatKES(ch.drawnAmount ?? 0)}</TableCell>
                                  <TableCell className="text-xs font-mono text-right">{(ch.dailyRate ?? 0).toFixed(4)}%</TableCell>
                                  <TableCell className="text-xs font-mono text-right text-destructive">{formatKES(ch.interestCharged ?? 0)}</TableCell>
                                  <TableCell className="text-xs font-mono text-muted-foreground">{ch.reference}</TableCell>
                                </TableRow>
                              ))}
                            </TableBody>
                          </Table>
                        </CardContent>
                      </Card>
                    )}
                  </div>
                </ErrorBoundary>
              </TabsContent>

              {/* ─── Statements Tab ────────────────────────────── */}
              <TabsContent value="statements">
                <ErrorBoundary>
                  <Card>
                    <CardHeader className="pb-2">
                      <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                        <FileText className="h-3.5 w-3.5" /> Account Statements
                      </CardTitle>
                    </CardHeader>
                    <CardContent>
                      {!accounts || accounts.length === 0 ? (
                        <p className="text-xs text-muted-foreground">No accounts to generate statements for.</p>
                      ) : (
                        <div className="space-y-4">
                          {/* Statement controls */}
                          <div className="flex flex-wrap items-end gap-3">
                            <div>
                              <Label className="text-[11px] uppercase tracking-wider text-muted-foreground">Account</Label>
                              <select className="w-48 h-8 rounded-md border text-xs px-2 mt-1 block"
                                value={stmtAccountId ?? ""}
                                onChange={(e) => setStmtAccountId(e.target.value || null)}>
                                <option value="">Select account...</option>
                                {accounts.map((a) => (
                                  <option key={a.id} value={a.id}>{a.accountNumber} ({a.accountType})</option>
                                ))}
                              </select>
                            </div>
                            <div>
                              <Label className="text-[11px] uppercase tracking-wider text-muted-foreground">Type</Label>
                              <select className="w-28 h-8 rounded-md border text-xs px-2 mt-1 block"
                                value={stmtType}
                                onChange={(e) => setStmtType(e.target.value as "full" | "mini")}>
                                <option value="mini">Mini</option>
                                <option value="full">Full</option>
                              </select>
                            </div>
                            {stmtType === "full" && (
                              <>
                                <div>
                                  <Label className="text-[11px] uppercase tracking-wider text-muted-foreground">From</Label>
                                  <Input type="date" className="w-36 h-8 text-xs mt-1" value={stmtFrom}
                                    onChange={(e) => setStmtFrom(e.target.value)} />
                                </div>
                                <div>
                                  <Label className="text-[11px] uppercase tracking-wider text-muted-foreground">To</Label>
                                  <Input type="date" className="w-36 h-8 text-xs mt-1" value={stmtTo}
                                    onChange={(e) => setStmtTo(e.target.value)} />
                                </div>
                              </>
                            )}
                            {statementData && stmtType === "full" && (
                              <Button variant="outline" size="sm" className="text-xs h-8"
                                onClick={() => downloadStatementCSV(statementData)}>
                                <Download className="h-3 w-3 mr-1" /> Download CSV
                              </Button>
                            )}
                          </div>

                          {/* Statement results */}
                          {(stmtFetching || miniStmtFetching) && (
                            <div className="text-xs text-muted-foreground py-4 text-center">Loading statement...</div>
                          )}

                          {stmtType === "full" && statementData && (
                            <div className="space-y-3">
                              <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
                                <div>
                                  <p className="text-[11px] text-muted-foreground uppercase">Account</p>
                                  <p className="font-mono">{statementData.accountNumber}</p>
                                </div>
                                <div>
                                  <p className="text-[11px] text-muted-foreground uppercase">Opening Balance</p>
                                  <p className="font-mono">{formatKES(statementData.openingBalance ?? 0)}</p>
                                </div>
                                <div>
                                  <p className="text-[11px] text-muted-foreground uppercase">Closing Balance</p>
                                  <p className="font-mono font-semibold">{formatKES(statementData.closingBalance ?? 0)}</p>
                                </div>
                                <div>
                                  <p className="text-[11px] text-muted-foreground uppercase">Period</p>
                                  <p className="font-mono">{statementData.periodFrom} — {statementData.periodTo}</p>
                                </div>
                              </div>
                              <Table>
                                <TableHeader>
                                  <TableRow className="hover:bg-transparent">
                                    <TableHead className="text-[11px] uppercase tracking-wider">Date</TableHead>
                                    <TableHead className="text-[11px] uppercase tracking-wider">Type</TableHead>
                                    <TableHead className="text-[11px] uppercase tracking-wider text-right">Amount</TableHead>
                                    <TableHead className="text-[11px] uppercase tracking-wider text-right">Balance</TableHead>
                                    <TableHead className="text-[11px] uppercase tracking-wider">Reference</TableHead>
                                    <TableHead className="text-[11px] uppercase tracking-wider">Description</TableHead>
                                  </TableRow>
                                </TableHeader>
                                <TableBody>
                                  {(statementData.transactions?.content ?? []).map((t) => (
                                    <TableRow key={t.id} className="table-row-hover">
                                      <TableCell className="text-xs">{safeDate(t.createdAt ?? t.valueDate)}</TableCell>
                                      <TableCell className="text-xs">{t.transactionType}</TableCell>
                                      <TableCell className={`text-xs font-mono text-right ${
                                        t.transactionType === "CREDIT" ? "text-success" : "text-destructive"
                                      }`}>
                                        {formatKES(t.amount ?? 0)}
                                      </TableCell>
                                      <TableCell className="text-xs font-mono text-right">
                                        {t.runningBalance != null ? formatKES(t.runningBalance) : "—"}
                                      </TableCell>
                                      <TableCell className="text-xs font-mono text-muted-foreground">{t.reference ?? "—"}</TableCell>
                                      <TableCell className="text-xs text-muted-foreground truncate max-w-[200px]">{t.description ?? "—"}</TableCell>
                                    </TableRow>
                                  ))}
                                </TableBody>
                              </Table>
                            </div>
                          )}

                          {stmtType === "mini" && miniStatementData && (
                            <Table>
                              <TableHeader>
                                <TableRow className="hover:bg-transparent">
                                  <TableHead className="text-[11px] uppercase tracking-wider">Date</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider">Type</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider text-right">Amount</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider text-right">Balance</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider">Reference</TableHead>
                                </TableRow>
                              </TableHeader>
                              <TableBody>
                                {(miniStatementData.content ?? []).map((t) => (
                                  <TableRow key={t.id} className="table-row-hover">
                                    <TableCell className="text-xs">{safeDate(t.createdAt ?? t.valueDate)}</TableCell>
                                    <TableCell className="text-xs">{t.transactionType}</TableCell>
                                    <TableCell className={`text-xs font-mono text-right ${
                                      t.transactionType === "CREDIT" ? "text-success" : "text-destructive"
                                    }`}>
                                      {formatKES(t.amount ?? 0)}
                                    </TableCell>
                                    <TableCell className="text-xs font-mono text-right">
                                      {t.runningBalance != null ? formatKES(t.runningBalance) : "—"}
                                    </TableCell>
                                    <TableCell className="text-xs font-mono text-muted-foreground">{t.reference ?? "—"}</TableCell>
                                  </TableRow>
                                ))}
                              </TableBody>
                            </Table>
                          )}
                        </div>
                      )}
                    </CardContent>
                  </Card>
                </ErrorBoundary>
              </TabsContent>

              {/* ─── Fraud & Risk Tab ──────────────────────────── */}
              <TabsContent value="fraud">
                <ErrorBoundary>
                  <div className="space-y-4">
                    {/* Risk Profile */}
                    <Card>
                      <CardHeader className="pb-2">
                        <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                          <Shield className="h-3.5 w-3.5" /> Risk Profile
                        </CardTitle>
                      </CardHeader>
                      <CardContent>
                        {!riskProfile ? (
                          <p className="text-xs text-muted-foreground">No risk profile available.</p>
                        ) : (
                          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Risk Score</p>
                              <p className={`text-2xl font-bold font-mono ${riskColors[riskProfile.riskLevel] ?? ""}`}>
                                {riskProfile.riskScore}
                              </p>
                              <Badge variant="outline" className={`text-[11px] mt-1 ${severityColors[riskProfile.riskLevel] ?? ""}`}>
                                {riskProfile.riskLevel}
                              </Badge>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Total Alerts</p>
                              <p className="text-2xl font-bold font-mono">{riskProfile.totalAlerts}</p>
                              <p className="text-[11px] text-muted-foreground">{riskProfile.openAlerts} open</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">Confirmed Fraud</p>
                              <p className="text-2xl font-bold font-mono text-destructive">{riskProfile.confirmedFraud}</p>
                            </div>
                            <div>
                              <p className="text-[11px] text-muted-foreground uppercase tracking-wider">False Positives</p>
                              <p className="text-2xl font-bold font-mono text-muted-foreground">{riskProfile.falsePositives}</p>
                            </div>
                          </div>
                        )}
                      </CardContent>
                    </Card>

                    {/* Fraud Alerts */}
                    <Card>
                      <CardHeader className="pb-2">
                        <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                          <AlertTriangle className="h-3.5 w-3.5" /> Fraud Alerts ({customerAlerts.length})
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="p-0">
                        {customerAlerts.length === 0 ? (
                          <div className="p-4 text-center text-xs text-muted-foreground">
                            No fraud alerts for this customer.
                          </div>
                        ) : (
                          <Table>
                            <TableHeader>
                              <TableRow className="hover:bg-transparent">
                                <TableHead className="text-[11px] uppercase tracking-wider">Type</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Severity</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Status</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Description</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Date</TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {customerAlerts.map((alert) => (
                                <TableRow key={alert.id} className="table-row-hover cursor-pointer"
                                  onClick={() => navigate("/fraud")}>
                                  <TableCell className="text-xs font-mono">{alert.alertType}</TableCell>
                                  <TableCell>
                                    <Badge variant="outline" className={`text-[11px] ${severityColors[alert.severity] ?? ""}`}>
                                      {alert.severity}
                                    </Badge>
                                  </TableCell>
                                  <TableCell>
                                    <Badge variant="outline" className={`text-[11px] ${alertStatusColors[alert.status] ?? ""}`}>
                                      {alert.status}
                                    </Badge>
                                  </TableCell>
                                  <TableCell className="text-xs max-w-[300px] truncate">{alert.description}</TableCell>
                                  <TableCell className="text-xs text-muted-foreground">
                                    {safeDate(alert.createdAt)}
                                  </TableCell>
                                </TableRow>
                              ))}
                            </TableBody>
                          </Table>
                        )}
                      </CardContent>
                    </Card>

                    {/* Network Links */}
                    <Card>
                      <CardHeader className="pb-2">
                        <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                          <Network className="h-3.5 w-3.5" /> Network Links
                        </CardTitle>
                      </CardHeader>
                      <CardContent>
                        {!networkData || !networkData.links?.length ? (
                          <p className="text-xs text-muted-foreground">No network links detected.</p>
                        ) : (
                          <div className="space-y-2">
                            <p className="text-xs text-muted-foreground">
                              {networkData.linkCount ?? networkData.links.length} connection{(networkData.linkCount ?? networkData.links.length) !== 1 ? "s" : ""} detected
                            </p>
                            <Table>
                              <TableHeader>
                                <TableRow className="hover:bg-transparent">
                                  <TableHead className="text-[11px] uppercase tracking-wider">Linked Customer</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider">Link Type</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider">Value</TableHead>
                                  <TableHead className="text-[11px] uppercase tracking-wider">Flagged</TableHead>
                                </TableRow>
                              </TableHeader>
                              <TableBody>
                                {networkData.links.map((link, idx) => (
                                  <TableRow key={idx} className="table-row-hover">
                                    <TableCell className="text-xs font-mono text-info cursor-pointer"
                                      onClick={() => navigate(`/customer/${link.linkedCustomerId}`)}>
                                      {link.linkedCustomerId}
                                    </TableCell>
                                    <TableCell className="text-xs">{(link.linkType ?? "").replace("SHARED_", "")}</TableCell>
                                    <TableCell className="text-xs font-mono">{link.linkValue}</TableCell>
                                    <TableCell>
                                      {link.flagged ? (
                                        <Badge variant="outline" className="text-[11px] bg-destructive/15 text-destructive border-destructive/30">
                                          Flagged
                                        </Badge>
                                      ) : (
                                        <span className="text-xs text-muted-foreground">—</span>
                                      )}
                                    </TableCell>
                                  </TableRow>
                                ))}
                              </TableBody>
                            </Table>
                          </div>
                        )}
                      </CardContent>
                    </Card>

                    {/* ML Scoring History */}
                    <Card>
                      <CardHeader className="pb-2">
                        <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                          <Zap className="h-3.5 w-3.5" /> ML Scoring History ({scoringHistory.length})
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="p-0">
                        {scoringHistory.length === 0 ? (
                          <div className="p-4 text-center text-xs text-muted-foreground">
                            No ML scoring records for this customer.
                          </div>
                        ) : (
                          <Table>
                            <TableHeader>
                              <TableRow className="hover:bg-transparent">
                                <TableHead className="text-[11px] uppercase tracking-wider">Event</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">ML Score</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Risk</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider text-right">Amount</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Latency</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Date</TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {scoringHistory.map((s) => (
                                <TableRow key={s.id} className="table-row-hover">
                                  <TableCell className="text-xs font-mono">{s.eventType ?? "—"}</TableCell>
                                  <TableCell className="text-xs font-mono font-semibold">
                                    {(s.mlScore ?? 0).toFixed(3)}
                                  </TableCell>
                                  <TableCell>
                                    <Badge variant="outline" className={`text-[11px] ${severityColors[s.riskLevel] ?? ""}`}>
                                      {s.riskLevel}
                                    </Badge>
                                  </TableCell>
                                  <TableCell className="text-xs font-mono text-right">
                                    {s.amount != null ? Number(s.amount).toLocaleString() : "—"}
                                  </TableCell>
                                  <TableCell className="text-xs text-muted-foreground">
                                    {s.latencyMs != null ? `${Number(s.latencyMs).toFixed(0)}ms` : "—"}
                                  </TableCell>
                                  <TableCell className="text-xs text-muted-foreground">
                                    {safeDate(s.createdAt)}
                                  </TableCell>
                                </TableRow>
                              ))}
                            </TableBody>
                          </Table>
                        )}
                      </CardContent>
                    </Card>

                    {/* Investigation Cases */}
                    <Card>
                      <CardHeader className="pb-2">
                        <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                          <Briefcase className="h-3.5 w-3.5" /> Investigation Cases ({customerCases.length})
                        </CardTitle>
                      </CardHeader>
                      <CardContent className="p-0">
                        {customerCases.length === 0 ? (
                          <div className="p-4 text-center text-xs text-muted-foreground">
                            No investigation cases for this customer.
                          </div>
                        ) : (
                          <Table>
                            <TableHeader>
                              <TableRow className="hover:bg-transparent">
                                <TableHead className="text-[11px] uppercase tracking-wider">Case #</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Title</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Priority</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Status</TableHead>
                                <TableHead className="text-[11px] uppercase tracking-wider">Opened</TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {customerCases.map((c) => (
                                <TableRow key={c.id} className="table-row-hover cursor-pointer"
                                  onClick={() => navigate("/fraud-cases")}>
                                  <TableCell className="text-xs font-mono text-info">{c.caseNumber}</TableCell>
                                  <TableCell className="text-xs max-w-[250px] truncate">{c.title}</TableCell>
                                  <TableCell>
                                    <Badge variant="outline" className={`text-[11px] ${severityColors[c.priority] ?? ""}`}>
                                      {c.priority}
                                    </Badge>
                                  </TableCell>
                                  <TableCell>
                                    <Badge variant="outline" className={`text-[11px] ${alertStatusColors[c.status] ?? ""}`}>
                                      {(c.status ?? "").replace(/_/g, " ")}
                                    </Badge>
                                  </TableCell>
                                  <TableCell className="text-xs text-muted-foreground">
                                    {safeDate(c.createdAt)}
                                  </TableCell>
                                </TableRow>
                              ))}
                            </TableBody>
                          </Table>
                        )}
                      </CardContent>
                    </Card>
                  </div>
                </ErrorBoundary>
              </TabsContent>
            </Tabs>
          </>
        )}
      </div>
    </DashboardLayout>
  );
};

export default Customer360Page;
