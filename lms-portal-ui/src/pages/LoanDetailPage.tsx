import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatKES, formatKESFull } from "@/lib/format";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import { ArrowLeft, CreditCard, FileText, Calendar, DollarSign } from "lucide-react";
import { motion } from "framer-motion";
import { useToast } from "@/hooks/use-toast";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  loanManagementService,
  type Loan,
  type Installment,
  type Repayment,
} from "@/services/loanManagementService";

// ─── Adapters ──────────────────────────────────────────
function adaptRealLoan(loan: Loan) {
  const dpd = loan.dpd ?? 0;
  const stage = dpd === 0 ? "current" : dpd <= 7 ? "watch" : dpd <= 90 ? "delinquent" : "npl";
  return {
    id: loan.id,
    customer: loan.customerId,
    product: loan.productId ?? "—",
    principal: loan.disbursedAmount ?? 0,
    outstanding: loan.outstandingPrincipal ?? 0,
    nextDue: loan.nextDueDate ?? "—",
    dpd,
    stage,
    rate: loan.interestRate ?? 0,
    tenor: loan.tenorMonths ?? 0,
    maturity: loan.maturityDate ?? "—",
    disbursedAt: loan.disbursedAt ?? "—",
    status: loan.status,
  };
}

const statusBadge: Record<string, string> = {
  PAID: "bg-success/15 text-success border-success/30",
  OVERDUE: "bg-destructive/15 text-destructive border-destructive/30",
  PENDING: "bg-muted text-muted-foreground border-border",
  COMPLETED: "bg-success/15 text-success border-success/30",
};

const stageLabel: Record<string, { label: string; cls: string }> = {
  current: { label: "Current", cls: "bg-success/15 text-success border-success/30" },
  watch: { label: "Watch", cls: "bg-warning/15 text-warning border-warning/30" },
  delinquent: { label: "Delinquent", cls: "bg-destructive/15 text-destructive border-destructive/30" },
  npl: { label: "NPL", cls: "bg-destructive/25 text-destructive border-destructive/40" },
};

const LoanDetailPage = () => {
  const { loanId } = useParams();
  const navigate = useNavigate();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [paymentOpen, setPaymentOpen] = useState(false);
  const [paymentAmount, setPaymentAmount] = useState("");

  const { data: rawLoan, isLoading: loanLoading } = useQuery({
    queryKey: ["loan", loanId],
    queryFn: () => loanManagementService.getLoan(loanId!),
    enabled: !!loanId,
    retry: false,
  });

  const { data: schedule, isLoading: scheduleLoading } = useQuery({
    queryKey: ["loan-schedule", loanId],
    queryFn: () => loanManagementService.getLoanSchedule(loanId!),
    enabled: !!loanId,
    retry: false,
  });

  const { data: repayments, isLoading: repaymentsLoading } = useQuery({
    queryKey: ["loan-repayments", loanId],
    queryFn: () => loanManagementService.getLoanRepayments(loanId!),
    enabled: !!loanId,
    retry: false,
  });

  const paymentMutation = useMutation({
    mutationFn: (amount: number) =>
      loanManagementService.applyRepayment(loanId!, { amount, paymentMethod: "BANK_TRANSFER" }),
    onSuccess: () => {
      toast({ title: "Payment Posted", description: `${formatKES(parseFloat(paymentAmount))} applied successfully` });
      setPaymentOpen(false);
      setPaymentAmount("");
      queryClient.invalidateQueries({ queryKey: ["loan", loanId] });
      queryClient.invalidateQueries({ queryKey: ["loan-repayments", loanId] });
      queryClient.invalidateQueries({ queryKey: ["loan-schedule", loanId] });
    },
    onError: (err: Error) => {
      toast({ title: "Payment Failed", description: err.message, variant: "destructive" });
    },
  });

  if (loanLoading) {
    return (
      <DashboardLayout
        title="Loading..."
        subtitle=""
        breadcrumbs={[{ label: "Home", href: "/" }, { label: "Active Loans", href: "/active-loans" }]}
      >
        <div className="space-y-4 p-4">
          <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-16 w-full" />
            ))}
          </div>
          <Skeleton className="h-64 w-full" />
        </div>
      </DashboardLayout>
    );
  }

  const loan = rawLoan ? adaptRealLoan(rawLoan) : null;

  if (!loan) {
    return (
      <DashboardLayout
        title="Loan Not Found"
        subtitle=""
        breadcrumbs={[{ label: "Home", href: "/" }, { label: "Active Loans", href: "/active-loans" }, { label: "Not Found" }]}
      >
        <Card>
          <CardContent className="p-8 text-center text-muted-foreground font-sans">
            Loan {loanId} not found.
          </CardContent>
        </Card>
      </DashboardLayout>
    );
  }

  const st = stageLabel[loan.stage] ?? stageLabel["current"];

  const infoCards = [
    { label: "Principal", value: formatKES(loan.principal) },
    { label: "Outstanding", value: formatKES(loan.outstanding) },
    { label: "Interest Rate", value: loan.rate > 0 ? `${loan.rate}% p.a.` : "—" },
    { label: "Tenor", value: loan.tenor > 0 ? `${loan.tenor} months` : "—" },
    { label: "Next Due Date", value: loan.nextDue },
    { label: "DPD", value: loan.dpd.toString(), highlight: loan.dpd > 0 },
  ];

  const handlePayment = () => {
    const amt = parseFloat(paymentAmount);
    if (!amt || amt <= 0) return;
    paymentMutation.mutate(amt);
  };

  const installments = schedule ?? [];
  const repaymentList = repayments ?? [];

  return (
    <DashboardLayout
      title={loan.id}
      subtitle={`${loan.customer} — ${loan.product}`}
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Active Loans", href: "/active-loans" }, { label: loan.id }]}
    >
      <div className="space-y-4">
        {/* Top bar */}
        <div className="flex items-center justify-between">
          <Button variant="ghost" size="sm" className="text-xs font-sans" onClick={() => navigate("/active-loans")}>
            <ArrowLeft className="h-3.5 w-3.5 mr-1" /> Back to Loans
          </Button>
          <div className="flex items-center gap-2">
            <Badge variant="outline" className={`text-[10px] font-sans ${st.cls}`}>{st.label}</Badge>
            <Button size="sm" className="text-xs font-sans" onClick={() => setPaymentOpen(true)}>
              <DollarSign className="h-3.5 w-3.5 mr-1" /> Post Payment
            </Button>
          </div>
        </div>

        {/* Summary cards */}
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3">
          {infoCards.map((c, i) => (
            <motion.div key={c.label} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: i * 0.04 }}>
              <Card>
                <CardContent className="p-3">
                  <p className="text-[9px] uppercase tracking-wider text-muted-foreground font-sans">{c.label}</p>
                  <p className={`text-sm font-mono font-bold mt-0.5 ${c.highlight ? "text-destructive" : ""}`}>{c.value}</p>
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </div>

        {/* Tabs */}
        <Tabs defaultValue="schedule" className="w-full">
          <TabsList className="font-sans text-xs">
            <TabsTrigger value="schedule" className="text-xs"><Calendar className="h-3.5 w-3.5 mr-1" /> Schedule ({installments.length})</TabsTrigger>
            <TabsTrigger value="transactions" className="text-xs"><CreditCard className="h-3.5 w-3.5 mr-1" /> Repayments ({repaymentList.length})</TabsTrigger>
            <TabsTrigger value="details" className="text-xs"><FileText className="h-3.5 w-3.5 mr-1" /> Details</TabsTrigger>
          </TabsList>

          <TabsContent value="schedule">
            <Card>
              <CardContent className="p-0">
                {scheduleLoading ? (
                  <div className="p-4 space-y-2">
                    {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
                  </div>
                ) : installments.length === 0 ? (
                  <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                    <p className="text-sm font-medium">No schedule available</p>
                    <p className="text-xs mt-1">This loan does not have a repayment schedule generated yet.</p>
                  </div>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="text-[10px] font-sans">#</TableHead>
                        <TableHead className="text-[10px] font-sans">Due Date</TableHead>
                        <TableHead className="text-[10px] font-sans text-right">Principal</TableHead>
                        <TableHead className="text-[10px] font-sans text-right">Interest</TableHead>
                        <TableHead className="text-[10px] font-sans text-right">Total Due</TableHead>
                        <TableHead className="text-[10px] font-sans text-right">Balance</TableHead>
                        <TableHead className="text-[10px] font-sans">Status</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {installments.map((row: Installment) => (
                        <TableRow key={row.installmentNo} className="table-row-hover">
                          <TableCell className="text-xs font-mono">{row.installmentNo}</TableCell>
                          <TableCell className="text-xs font-sans">{row.dueDate?.split("T")[0] ?? "—"}</TableCell>
                          <TableCell className="text-xs font-mono text-right">{formatKES(row.principalDue)}</TableCell>
                          <TableCell className="text-xs font-mono text-right">{formatKES(row.interestDue)}</TableCell>
                          <TableCell className="text-xs font-mono text-right font-semibold">{formatKES(row.totalDue)}</TableCell>
                          <TableCell className="text-xs font-mono text-right">{formatKESFull(row.totalDue - (row.totalPaid ?? 0))}</TableCell>
                          <TableCell>
                            <Badge variant="outline" className={`text-[9px] font-sans capitalize ${statusBadge[row.status?.toUpperCase()] ?? statusBadge["PENDING"]}`}>
                              {row.status ?? "pending"}
                            </Badge>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="transactions">
            <Card>
              <CardContent className="p-0">
                {repaymentsLoading ? (
                  <div className="p-4 space-y-2">
                    {Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
                  </div>
                ) : repaymentList.length === 0 ? (
                  <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                    <p className="text-sm font-medium">No repayments yet</p>
                    <p className="text-xs mt-1">No repayments have been recorded for this loan.</p>
                  </div>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="text-[10px] font-sans">ID</TableHead>
                        <TableHead className="text-[10px] font-sans">Date</TableHead>
                        <TableHead className="text-[10px] font-sans text-right">Amount</TableHead>
                        <TableHead className="text-[10px] font-sans">Method</TableHead>
                        <TableHead className="text-[10px] font-sans">Reference</TableHead>
                        <TableHead className="text-[10px] font-sans">Status</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {repaymentList.map((tx: Repayment) => (
                        <TableRow key={tx.id} className="table-row-hover">
                          <TableCell className="text-xs font-mono text-info">{tx.id?.slice(0, 8) ?? "—"}</TableCell>
                          <TableCell className="text-xs font-sans">{tx.paymentDate?.split("T")[0] ?? tx.createdAt?.split("T")[0] ?? "—"}</TableCell>
                          <TableCell className="text-xs font-mono text-right font-semibold">
                            {formatKES(tx.amount)}
                            {(tx.unallocatedAmount ?? 0) > 0 && (
                              <div className="text-[9px] font-sans font-normal text-warning">{formatKES(tx.unallocatedAmount!)} unallocated</div>
                            )}
                          </TableCell>
                          <TableCell className="text-xs font-sans">{tx.paymentMethod ?? "—"}</TableCell>
                          <TableCell className="text-xs font-mono text-muted-foreground">{tx.reference ?? "—"}</TableCell>
                          <TableCell>
                            <Badge variant="outline" className={`text-[9px] font-sans capitalize ${statusBadge[tx.status?.toUpperCase() ?? ""] ?? "bg-muted text-muted-foreground"}`}>
                              {tx.status ?? "completed"}
                            </Badge>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="details">
            <Card>
              <CardContent className="p-5 grid grid-cols-2 md:grid-cols-3 gap-4">
                {[
                  ["Product", loan.product],
                  ["Principal Amount", formatKES(loan.principal)],
                  ["Outstanding Balance", formatKES(loan.outstanding)],
                  ["Interest Rate", loan.rate > 0 ? `${loan.rate}% p.a.` : "—"],
                  ["Tenor", loan.tenor > 0 ? `${loan.tenor} months` : "—"],
                  ["Days Past Due", `${loan.dpd} days`],
                  ["Classification", st.label],
                  ["Next Due Date", loan.nextDue],
                  ["Disbursed", loan.disbursedAt?.split("T")[0] ?? "—"],
                  ["Maturity", loan.maturity?.split("T")[0] ?? "—"],
                  ["Status", loan.status],
                ].map(([label, value]) => (
                  <div key={label}>
                    <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans mb-0.5">{label}</p>
                    <p className="text-sm font-sans font-medium">{value}</p>
                  </div>
                ))}
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>

        {/* Payment Dialog */}
        <Dialog open={paymentOpen} onOpenChange={setPaymentOpen}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle className="font-heading">Post Payment — {loan.id}</DialogTitle>
            </DialogHeader>
            <div className="space-y-3 py-2">
              <div>
                <p className="text-xs text-muted-foreground font-sans mb-1">Outstanding Balance</p>
                <p className="text-lg font-mono font-bold">{formatKES(loan.outstanding)}</p>
              </div>
              <div>
                <label className="text-xs font-sans font-medium">Payment Amount (KES)</label>
                <Input
                  type="number"
                  placeholder="Enter amount"
                  className="mt-1 font-mono"
                  value={paymentAmount}
                  onChange={(e) => setPaymentAmount(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" size="sm" onClick={() => setPaymentOpen(false)} className="text-xs font-sans">Cancel</Button>
              <Button
                size="sm"
                onClick={handlePayment}
                disabled={paymentMutation.isPending}
                className="text-xs font-sans"
              >
                {paymentMutation.isPending ? "Posting..." : "Confirm Payment"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  );
};

export default LoanDetailPage;
