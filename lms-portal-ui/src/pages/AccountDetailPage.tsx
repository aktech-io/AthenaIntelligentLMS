import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
import {
  ArrowLeft, Snowflake, Sun, XCircle, DollarSign,
  ChevronLeft, ChevronRight, Info, CreditCard, TrendingUp, FileText,
  ArrowDownToLine, ArrowUpFromLine, ArrowLeftRight,
} from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  accountService,
  type Account,
  type BalanceResponse,
  type Transaction,
  type InterestSummary,
  type InterestAccrual,
  type InterestPosting,
  type StatementResponse,
  type TransferResponse,
} from "@/services/accountService";
import { depositProductService } from "@/services/depositProductService";

// ─── Helpers ──────────────────────────────────────────

const statusColors: Record<string, string> = {
  ACTIVE: "bg-green-100 text-green-700 border-green-300",
  DORMANT: "bg-amber-100 text-amber-700 border-amber-300",
  FROZEN: "bg-blue-100 text-blue-700 border-blue-300",
  CLOSED: "bg-red-100 text-red-700 border-red-300",
  PENDING_APPROVAL: "bg-purple-100 text-purple-700 border-purple-300",
};

function fmtCurrency(amount: number | undefined | null, currency = "KES"): string {
  if (amount == null) return "--";
  return `${currency} ${amount.toLocaleString("en", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

function fmtDate(d: string | undefined | null): string {
  if (!d) return "--";
  return d.split("T")[0];
}

function resolveAvailable(acc: Account): number {
  if (acc.balance == null) return 0;
  if (typeof acc.balance === "number") return acc.balance;
  return acc.balance.availableBalance ?? 0;
}

function resolveCurrent(acc: Account): number {
  if (acc.balance == null) return 0;
  if (typeof acc.balance === "number") return acc.balance;
  return acc.balance.currentBalance ?? 0;
}

function daysUntil(dateStr: string | undefined | null): number | null {
  if (!dateStr) return null;
  const target = new Date(dateStr);
  const now = new Date();
  const diff = Math.ceil((target.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
  return diff;
}

function defaultDateRange(): { from: string; to: string } {
  const now = new Date();
  const to = now.toISOString().split("T")[0];
  const from = new Date(now.getFullYear(), now.getMonth() - 3, now.getDate())
    .toISOString()
    .split("T")[0];
  return { from, to };
}

// ─── Component ────────────────────────────────────────

const AccountDetailPage = () => {
  const { accountId } = useParams<{ accountId: string }>();
  const navigate = useNavigate();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  // Transaction pagination
  const [txPage, setTxPage] = useState(0);
  const [txPageSize, setTxPageSize] = useState(20);

  // Statement date range
  const defaults = defaultDateRange();
  const [stmtFrom, setStmtFrom] = useState(defaults.from);
  const [stmtTo, setStmtTo] = useState(defaults.to);
  const [stmtRequested, setStmtRequested] = useState(false);

  // Cash transaction (deposit / withdraw) dialog
  const [cashOpen, setCashOpen] = useState(false);
  const [cashMode, setCashMode] = useState<"deposit" | "withdraw">("deposit");
  const [cashAmount, setCashAmount] = useState("");
  const [cashDesc, setCashDesc] = useState("");

  // Transfer dialog
  const [transferOpen, setTransferOpen] = useState(false);
  const [transferDest, setTransferDest] = useState("");
  const [transferAmount, setTransferAmount] = useState("");
  const [transferType, setTransferType] = useState("THIRD_PARTY");
  const [transferNarration, setTransferNarration] = useState("");

  // ─── Queries ──────────────────────────────

  const { data: account, isLoading: accountLoading } = useQuery({
    queryKey: ["account", accountId],
    queryFn: () => accountService.getAccount(accountId!),
    enabled: !!accountId,
    retry: false,
  });

  const { data: balance, isLoading: balanceLoading } = useQuery({
    queryKey: ["account-balance", accountId],
    queryFn: () => accountService.getBalance(accountId!),
    enabled: !!accountId,
    retry: false,
  });

  const { data: txPage_, isLoading: txLoading } = useQuery({
    queryKey: ["account-transactions", accountId, txPage, txPageSize],
    queryFn: () => accountService.getTransactions(accountId!, txPage, txPageSize),
    enabled: !!accountId,
    retry: false,
  });

  const { data: interestSummary, isLoading: interestLoading } = useQuery({
    queryKey: ["account-interest", accountId],
    queryFn: () => accountService.getInterestSummary(accountId!),
    enabled: !!accountId,
    retry: false,
  });

  const { data: statement, isLoading: stmtLoading } = useQuery({
    queryKey: ["account-statement", accountId, stmtFrom, stmtTo],
    queryFn: () => accountService.getStatement(accountId!, stmtFrom, stmtTo),
    enabled: !!accountId && stmtRequested,
    retry: false,
  });

  const { data: transfersPage, isLoading: transfersLoading } = useQuery({
    queryKey: ["account-transfers", accountId],
    queryFn: () => accountService.getTransfersByAccount(accountId!),
    enabled: !!accountId,
    retry: false,
  });

  const { data: depositProduct } = useQuery({
    queryKey: ["deposit-product", account?.depositProductId],
    queryFn: () => depositProductService.getProduct(account!.depositProductId!),
    enabled: !!account?.depositProductId,
    retry: false,
  });

  // ─── Mutations ────────────────────────────

  const postInterestMutation = useMutation({
    mutationFn: () => accountService.postInterest(accountId!),
    onSuccess: () => {
      toast({ title: "Interest Posted", description: "Interest has been posted to the account." });
      queryClient.invalidateQueries({ queryKey: ["account-interest", accountId] });
      queryClient.invalidateQueries({ queryKey: ["account-balance", accountId] });
      queryClient.invalidateQueries({ queryKey: ["account-transactions", accountId] });
    },
    onError: (err: Error) => {
      toast({ title: "Post Interest Failed", description: err.message, variant: "destructive" });
    },
  });

  const freezeMutation = useMutation({
    mutationFn: () => accountService.updateStatus(accountId!, "FROZEN"),
    onSuccess: () => {
      toast({ title: "Account Frozen" });
      queryClient.invalidateQueries({ queryKey: ["account", accountId] });
    },
    onError: (err: Error) => {
      toast({ title: "Freeze Failed", description: err.message, variant: "destructive" });
    },
  });

  const unfreezeMutation = useMutation({
    mutationFn: () => accountService.updateStatus(accountId!, "ACTIVE"),
    onSuccess: () => {
      toast({ title: "Account Unfrozen" });
      queryClient.invalidateQueries({ queryKey: ["account", accountId] });
    },
    onError: (err: Error) => {
      toast({ title: "Unfreeze Failed", description: err.message, variant: "destructive" });
    },
  });

  const closeMutation = useMutation({
    mutationFn: () => accountService.closeAccount(accountId!, "Closed by operator"),
    onSuccess: () => {
      toast({ title: "Account Closed" });
      queryClient.invalidateQueries({ queryKey: ["account", accountId] });
    },
    onError: (err: Error) => {
      toast({ title: "Close Failed", description: err.message, variant: "destructive" });
    },
  });

  const cashMutation = useMutation({
    mutationFn: () => {
      const amt = parseFloat(cashAmount);
      const desc = cashDesc.trim() || undefined;
      return cashMode === "deposit"
        ? accountService.deposit(accountId!, amt, desc)
        : accountService.withdraw(accountId!, amt, desc);
    },
    onSuccess: (result) => {
      const isPending =
        !!result &&
        typeof result === "object" &&
        "status" in result &&
        (result as { status?: string }).status === "PENDING_APPROVAL";
      if (isPending) {
        toast({
          title: "Submitted for Approval",
          description: `Submitted for approval — a second authoriser must approve this ${
            cashMode === "deposit" ? "deposit" : "withdrawal"
          }.`,
        });
      } else {
        toast({
          title: cashMode === "deposit" ? "Deposit Successful" : "Withdrawal Successful",
          description: `${fmtCurrency(parseFloat(cashAmount), account?.currency ?? "KES")} ${
            cashMode === "deposit" ? "credited to" : "debited from"
          } the account.`,
        });
      }
      setCashOpen(false);
      setCashAmount("");
      setCashDesc("");
      queryClient.invalidateQueries({ queryKey: ["account", accountId] });
      queryClient.invalidateQueries({ queryKey: ["account-balance", accountId] });
      queryClient.invalidateQueries({ queryKey: ["account-transactions", accountId] });
    },
    onError: (err: Error) => {
      toast({
        title: cashMode === "deposit" ? "Deposit Failed" : "Withdrawal Failed",
        description: err.message,
        variant: "destructive",
      });
    },
  });

  const transferMutation = useMutation({
    mutationFn: () => {
      const amt = parseFloat(transferAmount);
      return accountService.initiateTransfer({
        sourceAccountId: accountId!,
        destinationAccountNumber: transferDest.trim(),
        amount: amt,
        transferType,
        narration: transferNarration.trim() || undefined,
      });
    },
    onSuccess: (result) => {
      const isPending =
        !!result &&
        typeof result === "object" &&
        "status" in result &&
        (result as { status?: string }).status === "PENDING_APPROVAL";
      if (isPending) {
        toast({
          title: "Submitted for Approval",
          description:
            "Submitted for approval — a second authoriser must approve this transfer.",
        });
      } else {
        toast({
          title: "Transfer Successful",
          description: `${fmtCurrency(
            parseFloat(transferAmount),
            account?.currency ?? "KES"
          )} transferred from the account.`,
        });
      }
      setTransferOpen(false);
      setTransferDest("");
      setTransferAmount("");
      setTransferNarration("");
      queryClient.invalidateQueries({ queryKey: ["account-balance", accountId] });
      queryClient.invalidateQueries({ queryKey: ["account", accountId] });
      queryClient.invalidateQueries({ queryKey: ["account-transactions", accountId] });
      queryClient.invalidateQueries({ queryKey: ["account-transfers", accountId] });
    },
    onError: (err: Error) => {
      toast({ title: "Transfer Failed", description: err.message, variant: "destructive" });
    },
  });

  const openCashDialog = (mode: "deposit" | "withdraw") => {
    setCashMode(mode);
    setCashAmount("");
    setCashDesc("");
    setCashOpen(true);
  };

  const openTransferDialog = () => {
    setTransferDest("");
    setTransferAmount("");
    setTransferType("THIRD_PARTY");
    setTransferNarration("");
    setTransferOpen(true);
  };

  // ─── Loading state ────────────────────────

  if (accountLoading) {
    return (
      <DashboardLayout
        title="Loading..."
        subtitle=""
        breadcrumbs={[
          { label: "Home", href: "/" },
          { label: "Accounts", href: "/accounts" },
        ]}
      >
        <div className="space-y-4 p-4">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-20 w-full" />
            ))}
          </div>
          <Skeleton className="h-64 w-full" />
        </div>
      </DashboardLayout>
    );
  }

  if (!account) {
    return (
      <DashboardLayout
        title="Account Not Found"
        subtitle=""
        breadcrumbs={[
          { label: "Home", href: "/" },
          { label: "Accounts", href: "/accounts" },
          { label: "Not Found" },
        ]}
      >
        <Card>
          <CardContent className="p-8 text-center text-muted-foreground font-sans">
            Account {accountId} not found.
          </CardContent>
        </Card>
      </DashboardLayout>
    );
  }

  // ─── Derived data ─────────────────────────

  const isFrozen = account.status?.toUpperCase() === "FROZEN";
  const isClosed = account.status?.toUpperCase() === "CLOSED";
  const isFixedDeposit = account.accountType?.toUpperCase() === "FIXED_DEPOSIT";
  const maturityDays = daysUntil(account.maturityDate);

  const availBal = balance?.availableBalance ?? resolveAvailable(account);
  const currBal = balance?.currentBalance ?? resolveCurrent(account);
  const accruedInterest = account.accruedInterest ?? 0;
  const rate = account.interestRateOverride ?? 0;
  const currency = account.currency ?? "KES";

  const transactions: Transaction[] = txPage_?.content ?? [];
  const txTotalPages = txPage_?.totalPages ?? 1;

  const accruals: InterestAccrual[] = interestSummary?.recentAccruals ?? [];
  const postings: InterestPosting[] = interestSummary?.postingHistory ?? [];
  const unpostedTotal = interestSummary?.unpostedTotal ?? 0;

  const stmtTransactions: Transaction[] = statement?.transactions?.content ?? [];

  const transfers: TransferResponse[] = transfersPage?.content ?? [];

  // ─── Render ───────────────────────────────

  return (
    <DashboardLayout
      title={account.accountNumber ?? accountId!}
      subtitle={`${account.accountName ?? account.customerId} - ${account.accountType?.replace("_", " ") ?? ""}`}
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Accounts", href: "/accounts" },
        { label: account.accountNumber ?? accountId! },
      ]}
    >
      <div className="space-y-4">
        {/* Top bar */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <Button
            variant="ghost"
            size="sm"
            className="text-xs font-sans"
            onClick={() => navigate("/accounts")}
          >
            <ArrowLeft className="h-3.5 w-3.5 mr-1" /> Back to Accounts
          </Button>
          <div className="flex items-center gap-2 flex-wrap">
            <Badge
              variant="outline"
              className={`text-[10px] font-semibold ${
                statusColors[account.status?.toUpperCase()] ??
                "bg-muted text-muted-foreground"
              }`}
            >
              {account.status}
            </Badge>
            {!isClosed && (
              <>
                <Button
                  size="sm"
                  className="text-xs font-sans bg-green-600 hover:bg-green-700 text-white"
                  onClick={() => openCashDialog("deposit")}
                  disabled={isFrozen}
                >
                  <ArrowDownToLine className="h-3.5 w-3.5 mr-1" /> Deposit
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="text-xs font-sans"
                  onClick={() => openCashDialog("withdraw")}
                  disabled={isFrozen}
                >
                  <ArrowUpFromLine className="h-3.5 w-3.5 mr-1" /> Withdraw
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="text-xs font-sans"
                  onClick={openTransferDialog}
                  disabled={isFrozen}
                >
                  <ArrowLeftRight className="h-3.5 w-3.5 mr-1" /> Transfer
                </Button>
                {isFrozen ? (
                  <Button
                    variant="outline"
                    size="sm"
                    className="text-xs font-sans"
                    onClick={() => unfreezeMutation.mutate()}
                    disabled={unfreezeMutation.isPending}
                  >
                    <Sun className="h-3.5 w-3.5 mr-1" />
                    {unfreezeMutation.isPending ? "Unfreezing..." : "Unfreeze"}
                  </Button>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    className="text-xs font-sans"
                    onClick={() => freezeMutation.mutate()}
                    disabled={freezeMutation.isPending}
                  >
                    <Snowflake className="h-3.5 w-3.5 mr-1" />
                    {freezeMutation.isPending ? "Freezing..." : "Freeze"}
                  </Button>
                )}
                <Button
                  variant="outline"
                  size="sm"
                  className="text-xs font-sans text-destructive"
                  onClick={() => closeMutation.mutate()}
                  disabled={closeMutation.isPending}
                >
                  <XCircle className="h-3.5 w-3.5 mr-1" />
                  {closeMutation.isPending ? "Closing..." : "Close"}
                </Button>
                <Button
                  size="sm"
                  className="text-xs font-sans"
                  onClick={() => postInterestMutation.mutate()}
                  disabled={postInterestMutation.isPending}
                >
                  <DollarSign className="h-3.5 w-3.5 mr-1" />
                  {postInterestMutation.isPending ? "Posting..." : "Post Interest"}
                </Button>
              </>
            )}
          </div>
        </div>

        {/* Deposit / Withdraw dialog */}
        <Dialog open={cashOpen} onOpenChange={setCashOpen}>
          <DialogContent className="sm:max-w-sm">
            <DialogHeader>
              <DialogTitle className="font-heading">
                {cashMode === "deposit" ? "Deposit to" : "Withdraw from"} {account.accountNumber}
              </DialogTitle>
            </DialogHeader>
            <div className="space-y-3 py-1">
              <div className="text-xs text-muted-foreground font-sans">
                Available balance:{" "}
                <span className="font-mono font-semibold text-foreground">
                  {fmtCurrency(availBal, currency)}
                </span>
              </div>
              <div>
                <Label className="text-xs">Amount ({currency})</Label>
                <Input
                  type="number"
                  min="0"
                  step="0.01"
                  className="mt-1 text-sm font-mono"
                  placeholder="0.00"
                  value={cashAmount}
                  onChange={(e) => setCashAmount(e.target.value)}
                  autoFocus
                />
                {cashMode === "withdraw" &&
                  parseFloat(cashAmount || "0") > availBal && (
                    <p className="text-[10px] text-destructive mt-1">
                      Amount exceeds available balance.
                    </p>
                  )}
              </div>
              <div>
                <Label className="text-xs">Description (optional)</Label>
                <Input
                  className="mt-1 text-sm font-sans"
                  placeholder={cashMode === "deposit" ? "Cash deposit" : "Cash withdrawal"}
                  value={cashDesc}
                  onChange={(e) => setCashDesc(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" size="sm" className="text-xs" onClick={() => setCashOpen(false)}>
                Cancel
              </Button>
              <Button
                size="sm"
                className="text-xs"
                onClick={() => cashMutation.mutate()}
                disabled={
                  cashMutation.isPending ||
                  !cashAmount ||
                  parseFloat(cashAmount) <= 0 ||
                  (cashMode === "withdraw" && parseFloat(cashAmount) > availBal)
                }
              >
                {cashMutation.isPending
                  ? "Processing..."
                  : cashMode === "deposit"
                  ? "Confirm Deposit"
                  : "Confirm Withdrawal"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Transfer dialog */}
        <Dialog open={transferOpen} onOpenChange={setTransferOpen}>
          <DialogContent className="sm:max-w-sm">
            <DialogHeader>
              <DialogTitle className="font-heading">
                Transfer from {account.accountNumber}
              </DialogTitle>
            </DialogHeader>
            <div className="space-y-3 py-1">
              <div className="text-xs text-muted-foreground font-sans">
                Available balance:{" "}
                <span className="font-mono font-semibold text-foreground">
                  {fmtCurrency(availBal, currency)}
                </span>
              </div>
              <div>
                <Label className="text-xs">Source account</Label>
                <Input
                  className="mt-1 text-sm font-mono"
                  value={account.accountNumber}
                  readOnly
                  disabled
                />
              </div>
              <div>
                <Label className="text-xs">Destination account number</Label>
                <Input
                  className="mt-1 text-sm font-mono"
                  placeholder="Destination account number"
                  value={transferDest}
                  onChange={(e) => setTransferDest(e.target.value)}
                  autoFocus
                />
              </div>
              <div>
                <Label className="text-xs">Amount ({currency})</Label>
                <Input
                  type="number"
                  min="0"
                  step="0.01"
                  className="mt-1 text-sm font-mono"
                  placeholder="0.00"
                  value={transferAmount}
                  onChange={(e) => setTransferAmount(e.target.value)}
                />
                {parseFloat(transferAmount || "0") > availBal && (
                  <p className="text-[10px] text-destructive mt-1">
                    Amount exceeds available balance.
                  </p>
                )}
              </div>
              <div>
                <Label className="text-xs">Transfer type</Label>
                <Select value={transferType} onValueChange={setTransferType}>
                  <SelectTrigger className="mt-1 h-9 text-sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="INTERNAL" className="text-xs">
                      Internal (same customer)
                    </SelectItem>
                    <SelectItem value="THIRD_PARTY" className="text-xs">
                      Third party
                    </SelectItem>
                    <SelectItem value="EXTERNAL" className="text-xs">
                      External
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label className="text-xs">Narration (optional)</Label>
                <Input
                  className="mt-1 text-sm font-sans"
                  placeholder="Transfer narration"
                  value={transferNarration}
                  onChange={(e) => setTransferNarration(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                size="sm"
                className="text-xs"
                onClick={() => setTransferOpen(false)}
              >
                Cancel
              </Button>
              <Button
                size="sm"
                className="text-xs"
                onClick={() => transferMutation.mutate()}
                disabled={
                  transferMutation.isPending ||
                  !transferDest.trim() ||
                  !transferAmount ||
                  parseFloat(transferAmount) <= 0 ||
                  parseFloat(transferAmount) > availBal
                }
              >
                {transferMutation.isPending ? "Processing..." : "Confirm Transfer"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Summary cards */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <Card>
            <CardContent className="p-4">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                Available Balance
              </p>
              <p className="text-lg font-mono font-bold mt-1">
                {balanceLoading ? (
                  <Skeleton className="h-6 w-32" />
                ) : (
                  fmtCurrency(availBal, currency)
                )}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                Current Balance
              </p>
              <p className="text-lg font-mono font-bold mt-1">
                {balanceLoading ? (
                  <Skeleton className="h-6 w-32" />
                ) : (
                  fmtCurrency(currBal, currency)
                )}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                Accrued Interest
              </p>
              <p className="text-lg font-mono font-bold mt-1 text-green-600">
                {fmtCurrency(accruedInterest, currency)}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-4">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                Interest Rate
              </p>
              <p className="text-lg font-mono font-bold mt-1">
                {rate > 0 ? `${rate}% p.a.` : "--"}
              </p>
            </CardContent>
          </Card>
        </div>

        {/* Fixed Deposit maturity banner */}
        {isFixedDeposit && account.maturityDate && (
          <Card className="border-blue-200 bg-blue-50">
            <CardContent className="p-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2">
              <div>
                <p className="text-xs font-semibold text-blue-800 font-sans">
                  Fixed Deposit Details
                </p>
                <p className="text-xs text-blue-700 font-sans mt-1">
                  Term: {account.termDays ?? "--"} days | Locked Amount:{" "}
                  {fmtCurrency(account.lockedAmount, currency)} | Auto-Renew:{" "}
                  {account.autoRenew ? "Yes" : "No"}
                </p>
              </div>
              <div className="text-right">
                <p className="text-xs text-blue-700 font-sans">
                  Maturity: {fmtDate(account.maturityDate)}
                </p>
                {maturityDays !== null && (
                  <p
                    className={`text-sm font-bold font-mono ${
                      maturityDays <= 0 ? "text-red-600" : maturityDays <= 30 ? "text-amber-600" : "text-blue-800"
                    }`}
                  >
                    {maturityDays <= 0
                      ? "Matured"
                      : `${maturityDays} days remaining`}
                  </p>
                )}
              </div>
            </CardContent>
          </Card>
        )}

        {/* Tabs */}
        <Tabs defaultValue="overview" className="w-full">
          <TabsList className="font-sans text-xs">
            <TabsTrigger value="overview" className="text-xs">
              <Info className="h-3.5 w-3.5 mr-1" /> Overview
            </TabsTrigger>
            <TabsTrigger value="transactions" className="text-xs">
              <CreditCard className="h-3.5 w-3.5 mr-1" /> Transactions
            </TabsTrigger>
            <TabsTrigger value="transfers" className="text-xs">
              <ArrowLeftRight className="h-3.5 w-3.5 mr-1" /> Transfers
            </TabsTrigger>
            <TabsTrigger value="interest" className="text-xs">
              <TrendingUp className="h-3.5 w-3.5 mr-1" /> Interest
            </TabsTrigger>
            <TabsTrigger value="statement" className="text-xs">
              <FileText className="h-3.5 w-3.5 mr-1" /> Statement
            </TabsTrigger>
          </TabsList>

          {/* ─── Overview Tab ─────────────────── */}
          <TabsContent value="overview">
            <Card>
              <CardContent className="p-5 grid grid-cols-2 md:grid-cols-3 gap-4">
                {[
                  ["Account Type", account.accountType?.replace("_", " ") ?? "--"],
                  ["Product", depositProduct?.name ?? account.depositProductId ?? "--"],
                  ["Currency", currency],
                  ["KYC Tier", account.kycTier != null ? `Tier ${account.kycTier}` : "--"],
                  ["Opened Date", fmtDate(account.createdAt)],
                  ["Branch", account.branchId ?? account.branchCode ?? "--"],
                  ["Opened By", account.openedBy ?? "--"],
                  ["Status", account.status ?? "--"],
                  ["Dormant Since", fmtDate(account.dormantSince)],
                  ["Last Transaction", fmtDate(account.lastTransactionDate)],
                  ["Last Interest Accrual", fmtDate(account.lastInterestAccrualDate)],
                  ["Last Interest Posting", fmtDate(account.lastInterestPostingDate)],
                  ...(isFixedDeposit
                    ? [
                        ["Maturity Date", fmtDate(account.maturityDate)],
                        ["Term Days", account.termDays != null ? `${account.termDays} days` : "--"],
                        ["Locked Amount", fmtCurrency(account.lockedAmount, currency)],
                        ["Auto Renew", account.autoRenew ? "Yes" : "No"],
                      ]
                    : []),
                  ...(account.closedAt
                    ? [
                        ["Closed At", fmtDate(account.closedAt)],
                        ["Closure Reason", account.closureReason ?? "--"],
                      ]
                    : []),
                ].map(([label, value]) => (
                  <div key={label as string}>
                    <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans mb-0.5">
                      {label}
                    </p>
                    <p className="text-sm font-sans font-medium">{value}</p>
                  </div>
                ))}
              </CardContent>
            </Card>
          </TabsContent>

          {/* ─── Transactions Tab ─────────────── */}
          <TabsContent value="transactions">
            <Card>
              <CardContent className="p-0">
                {txLoading ? (
                  <div className="p-4 space-y-2">
                    {Array.from({ length: 5 }).map((_, i) => (
                      <Skeleton key={i} className="h-10 w-full" />
                    ))}
                  </div>
                ) : transactions.length === 0 ? (
                  <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                    <p className="text-sm font-medium">No transactions</p>
                    <p className="text-xs mt-1">No transactions found for this account.</p>
                  </div>
                ) : (
                  <>
                    <Table>
                      <TableHeader>
                        <TableRow className="hover:bg-transparent">
                          <TableHead className="text-[10px] uppercase tracking-wider">Date</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider">Type</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider">Description</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider">Reference</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider text-right">Amount</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider text-right">Balance</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {transactions.map((tx) => (
                          <TableRow key={tx.id} className="table-row-hover">
                            <TableCell className="text-xs font-sans">
                              {fmtDate(tx.valueDate ?? tx.createdAt)}
                            </TableCell>
                            <TableCell className="text-xs font-sans">
                              <Badge variant="outline" className="text-[9px]">
                                {tx.transactionType}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-xs text-muted-foreground font-sans">
                              {tx.description ?? "--"}
                            </TableCell>
                            <TableCell className="text-xs font-mono text-muted-foreground">
                              {tx.reference ?? "--"}
                            </TableCell>
                            <TableCell
                              className={`text-xs font-mono text-right font-semibold ${
                                tx.transactionType === "CREDIT" || tx.transactionType === "DEPOSIT"
                                  ? "text-green-600"
                                  : ""
                              }`}
                            >
                              {fmtCurrency(tx.amount, currency)}
                            </TableCell>
                            <TableCell className="text-xs font-mono text-right">
                              {fmtCurrency(tx.runningBalance ?? tx.balanceAfter, currency)}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>

                    {/* Transaction pagination */}
                    <div className="flex items-center justify-between p-3 border-t">
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-muted-foreground font-sans">
                          Rows per page:
                        </span>
                        <Select
                          value={String(txPageSize)}
                          onValueChange={(val) => {
                            setTxPageSize(Number(val));
                            setTxPage(0);
                          }}
                        >
                          <SelectTrigger className="w-[70px] h-8 text-xs">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {[10, 20, 50].map((s) => (
                              <SelectItem key={s} value={String(s)} className="text-xs">
                                {s}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-muted-foreground font-sans">
                          Page {txPage + 1} of {txTotalPages}
                        </span>
                        <Button
                          variant="outline"
                          size="icon"
                          className="h-8 w-8"
                          disabled={txPage === 0}
                          onClick={() => setTxPage((p) => Math.max(0, p - 1))}
                        >
                          <ChevronLeft className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="outline"
                          size="icon"
                          className="h-8 w-8"
                          disabled={txPage >= txTotalPages - 1}
                          onClick={() => setTxPage((p) => p + 1)}
                        >
                          <ChevronRight className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                  </>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          {/* ─── Transfers Tab ────────────────── */}
          <TabsContent value="transfers">
            <Card>
              <CardContent className="p-0">
                {transfersLoading ? (
                  <div className="p-4 space-y-2">
                    {Array.from({ length: 5 }).map((_, i) => (
                      <Skeleton key={i} className="h-10 w-full" />
                    ))}
                  </div>
                ) : transfers.length === 0 ? (
                  <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                    <p className="text-sm font-medium">No transfers</p>
                    <p className="text-xs mt-1">No transfers found for this account.</p>
                  </div>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow className="hover:bg-transparent">
                        <TableHead className="text-[10px] uppercase tracking-wider">Date</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider">Direction</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider">Counterparty</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider">Type</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider">Status</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider">Reference</TableHead>
                        <TableHead className="text-[10px] uppercase tracking-wider text-right">Amount</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {transfers.map((t) => {
                        const isOutgoing = t.sourceAccountId === accountId;
                        const counterparty = isOutgoing
                          ? t.destinationAccountNumber ?? t.destinationAccountId
                          : t.sourceAccountNumber ?? t.sourceAccountId;
                        return (
                          <TableRow key={t.id} className="table-row-hover">
                            <TableCell className="text-xs font-sans">
                              {fmtDate(t.initiatedAt)}
                            </TableCell>
                            <TableCell className="text-xs font-sans">
                              <Badge
                                variant="outline"
                                className={`text-[9px] ${
                                  isOutgoing
                                    ? "bg-red-100 text-red-700 border-red-300"
                                    : "bg-green-100 text-green-700 border-green-300"
                                }`}
                              >
                                {isOutgoing ? "Outgoing" : "Incoming"}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-xs font-mono text-muted-foreground">
                              {counterparty ?? "--"}
                            </TableCell>
                            <TableCell className="text-xs font-sans">
                              <Badge variant="outline" className="text-[9px]">
                                {t.transferType}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-xs font-sans">
                              <Badge
                                variant="outline"
                                className={`text-[9px] ${
                                  statusColors[t.status?.toUpperCase()] ??
                                  "bg-muted text-muted-foreground"
                                }`}
                              >
                                {t.status}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-xs font-mono text-muted-foreground">
                              {t.reference ?? "--"}
                            </TableCell>
                            <TableCell
                              className={`text-xs font-mono text-right font-semibold ${
                                isOutgoing ? "text-red-600" : "text-green-600"
                              }`}
                            >
                              {fmtCurrency(t.amount, t.currency ?? currency)}
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
          </TabsContent>

          {/* ─── Interest Tab ─────────────────── */}
          <TabsContent value="interest">
            <div className="space-y-4">
              {/* Unposted total banner */}
              <Card className="border-green-200 bg-green-50">
                <CardContent className="p-4 flex items-center justify-between">
                  <div>
                    <p className="text-xs font-semibold text-green-800 font-sans">
                      Unposted Interest
                    </p>
                    <p className="text-lg font-mono font-bold text-green-700 mt-0.5">
                      {interestLoading ? (
                        <Skeleton className="h-6 w-24" />
                      ) : (
                        fmtCurrency(unpostedTotal, currency)
                      )}
                    </p>
                  </div>
                  <Button
                    size="sm"
                    className="text-xs font-sans"
                    onClick={() => postInterestMutation.mutate()}
                    disabled={postInterestMutation.isPending || unpostedTotal === 0}
                  >
                    <DollarSign className="h-3.5 w-3.5 mr-1" />
                    {postInterestMutation.isPending ? "Posting..." : "Post Interest"}
                  </Button>
                </CardContent>
              </Card>

              {/* Accrual History */}
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium">Accrual History</CardTitle>
                </CardHeader>
                <CardContent className="p-0">
                  {interestLoading ? (
                    <div className="p-4 space-y-2">
                      {Array.from({ length: 3 }).map((_, i) => (
                        <Skeleton key={i} className="h-10 w-full" />
                      ))}
                    </div>
                  ) : accruals.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                      <p className="text-sm font-medium">No accruals</p>
                      <p className="text-xs mt-1">No interest accruals have been recorded yet.</p>
                    </div>
                  ) : (
                    <Table>
                      <TableHeader>
                        <TableRow className="hover:bg-transparent">
                          <TableHead className="text-[10px] uppercase tracking-wider">Date</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider text-right">Balance Used</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider text-right">Rate</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider text-right">Daily Amount</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider">Posted</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {accruals.map((a) => (
                          <TableRow key={a.id} className="table-row-hover">
                            <TableCell className="text-xs font-sans">{fmtDate(a.accrualDate)}</TableCell>
                            <TableCell className="text-xs font-mono text-right">
                              {fmtCurrency(a.balanceUsed, currency)}
                            </TableCell>
                            <TableCell className="text-xs font-mono text-right">{a.rate}%</TableCell>
                            <TableCell className="text-xs font-mono text-right font-semibold">
                              {fmtCurrency(a.dailyAmount, currency)}
                            </TableCell>
                            <TableCell>
                              <Badge
                                variant="outline"
                                className={`text-[9px] ${
                                  a.posted
                                    ? "bg-green-100 text-green-700 border-green-300"
                                    : "bg-amber-100 text-amber-700 border-amber-300"
                                }`}
                              >
                                {a.posted ? "Posted" : "Pending"}
                              </Badge>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                </CardContent>
              </Card>

              {/* Posting History */}
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium">Posting History</CardTitle>
                </CardHeader>
                <CardContent className="p-0">
                  {interestLoading ? (
                    <div className="p-4 space-y-2">
                      {Array.from({ length: 3 }).map((_, i) => (
                        <Skeleton key={i} className="h-10 w-full" />
                      ))}
                    </div>
                  ) : postings.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                      <p className="text-sm font-medium">No postings</p>
                      <p className="text-xs mt-1">No interest postings have been made yet.</p>
                    </div>
                  ) : (
                    <Table>
                      <TableHeader>
                        <TableRow className="hover:bg-transparent">
                          <TableHead className="text-[10px] uppercase tracking-wider">Period</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider text-right">Gross Interest</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider text-right">WHT</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider text-right">Net Interest</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider">Posted At</TableHead>
                          <TableHead className="text-[10px] uppercase tracking-wider">Posted By</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {postings.map((p) => (
                          <TableRow key={p.id} className="table-row-hover">
                            <TableCell className="text-xs font-sans">
                              {fmtDate(p.periodStart)} to {fmtDate(p.periodEnd)}
                            </TableCell>
                            <TableCell className="text-xs font-mono text-right">
                              {fmtCurrency(p.grossInterest, currency)}
                            </TableCell>
                            <TableCell className="text-xs font-mono text-right text-red-500">
                              {fmtCurrency(p.withholdingTax, currency)}
                            </TableCell>
                            <TableCell className="text-xs font-mono text-right font-semibold text-green-600">
                              {fmtCurrency(p.netInterest, currency)}
                            </TableCell>
                            <TableCell className="text-xs font-sans">
                              {fmtDate(p.postedAt)}
                            </TableCell>
                            <TableCell className="text-xs font-sans text-muted-foreground">
                              {p.postedBy ?? "--"}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                </CardContent>
              </Card>
            </div>
          </TabsContent>

          {/* ─── Statement Tab ────────────────── */}
          <TabsContent value="statement">
            <div className="space-y-4">
              {/* Date range picker */}
              <Card>
                <CardContent className="p-4">
                  <div className="flex flex-col sm:flex-row items-start sm:items-end gap-3">
                    <div>
                      <label className="text-xs font-sans font-medium">From</label>
                      <Input
                        type="date"
                        className="mt-1 text-xs font-sans w-[160px]"
                        value={stmtFrom}
                        onChange={(e) => setStmtFrom(e.target.value)}
                      />
                    </div>
                    <div>
                      <label className="text-xs font-sans font-medium">To</label>
                      <Input
                        type="date"
                        className="mt-1 text-xs font-sans w-[160px]"
                        value={stmtTo}
                        onChange={(e) => setStmtTo(e.target.value)}
                      />
                    </div>
                    <Button
                      size="sm"
                      className="text-xs font-sans"
                      onClick={() => setStmtRequested(true)}
                    >
                      <FileText className="h-3.5 w-3.5 mr-1" /> Generate Statement
                    </Button>
                  </div>
                </CardContent>
              </Card>

              {/* Statement display */}
              {stmtRequested && (
                <Card>
                  {stmtLoading ? (
                    <CardContent className="p-4 space-y-2">
                      {Array.from({ length: 5 }).map((_, i) => (
                        <Skeleton key={i} className="h-10 w-full" />
                      ))}
                    </CardContent>
                  ) : !statement ? (
                    <CardContent className="p-8 text-center text-muted-foreground">
                      <p className="text-sm font-medium">No statement data</p>
                      <p className="text-xs mt-1">Could not generate statement for this period.</p>
                    </CardContent>
                  ) : (
                    <>
                      {/* Statement header */}
                      <CardHeader className="pb-2">
                        <CardTitle className="text-sm font-medium">
                          Account Statement
                        </CardTitle>
                        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mt-3">
                          <div>
                            <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                              Account
                            </p>
                            <p className="text-xs font-mono font-medium">
                              {statement.accountNumber}
                            </p>
                          </div>
                          <div>
                            <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                              Customer
                            </p>
                            <p className="text-xs font-sans font-medium">
                              {statement.customerName}
                            </p>
                          </div>
                          <div>
                            <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                              Opening Balance
                            </p>
                            <p className="text-xs font-mono font-medium">
                              {fmtCurrency(statement.openingBalance, statement.currency ?? currency)}
                            </p>
                          </div>
                          <div>
                            <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                              Closing Balance
                            </p>
                            <p className="text-xs font-mono font-bold">
                              {fmtCurrency(statement.closingBalance, statement.currency ?? currency)}
                            </p>
                          </div>
                        </div>
                      </CardHeader>
                      <CardContent className="p-0">
                        {stmtTransactions.length === 0 ? (
                          <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                            <p className="text-sm font-medium">No transactions in period</p>
                            <p className="text-xs mt-1">
                              {fmtDate(statement.periodFrom)} to {fmtDate(statement.periodTo)}
                            </p>
                          </div>
                        ) : (
                          <Table>
                            <TableHeader>
                              <TableRow className="hover:bg-transparent">
                                <TableHead className="text-[10px] uppercase tracking-wider">Date</TableHead>
                                <TableHead className="text-[10px] uppercase tracking-wider">Type</TableHead>
                                <TableHead className="text-[10px] uppercase tracking-wider">Description</TableHead>
                                <TableHead className="text-[10px] uppercase tracking-wider">Reference</TableHead>
                                <TableHead className="text-[10px] uppercase tracking-wider text-right">Amount</TableHead>
                                <TableHead className="text-[10px] uppercase tracking-wider text-right">Balance</TableHead>
                              </TableRow>
                            </TableHeader>
                            <TableBody>
                              {stmtTransactions.map((tx) => (
                                <TableRow key={tx.id} className="table-row-hover">
                                  <TableCell className="text-xs font-sans">
                                    {fmtDate(tx.valueDate ?? tx.createdAt)}
                                  </TableCell>
                                  <TableCell className="text-xs font-sans">
                                    <Badge variant="outline" className="text-[9px]">
                                      {tx.transactionType}
                                    </Badge>
                                  </TableCell>
                                  <TableCell className="text-xs text-muted-foreground font-sans">
                                    {tx.description ?? "--"}
                                  </TableCell>
                                  <TableCell className="text-xs font-mono text-muted-foreground">
                                    {tx.reference ?? "--"}
                                  </TableCell>
                                  <TableCell
                                    className={`text-xs font-mono text-right font-semibold ${
                                      tx.transactionType === "CREDIT" || tx.transactionType === "DEPOSIT"
                                        ? "text-green-600"
                                        : ""
                                    }`}
                                  >
                                    {fmtCurrency(tx.amount, statement.currency ?? currency)}
                                  </TableCell>
                                  <TableCell className="text-xs font-mono text-right">
                                    {fmtCurrency(tx.runningBalance ?? tx.balanceAfter, statement.currency ?? currency)}
                                  </TableCell>
                                </TableRow>
                              ))}
                            </TableBody>
                          </Table>
                        )}
                      </CardContent>
                    </>
                  )}
                </Card>
              )}
            </div>
          </TabsContent>
        </Tabs>
      </div>
    </DashboardLayout>
  );
};

export default AccountDetailPage;
