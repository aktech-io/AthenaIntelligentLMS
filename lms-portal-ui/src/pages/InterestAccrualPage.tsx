import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { accountService, type Account } from "@/services/accountService";
import { useState } from "react";
import { useToast } from "@/hooks/use-toast";

function formatCurrency(amount: number | undefined, currency = "KES"): string {
  if (amount == null) return "\u2014";
  return `${currency} ${amount.toLocaleString("en", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

function formatDate(dateStr: string | undefined): string {
  if (!dateStr) return "\u2014";
  try {
    return new Date(dateStr).toLocaleDateString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
    });
  } catch {
    return dateStr;
  }
}

function getBalance(acc: Account): number {
  if (acc.balance == null) return 0;
  if (typeof acc.balance === "number") return acc.balance;
  return acc.balance.availableBalance ?? acc.balance.currentBalance ?? 0;
}

function getUserRole(): string | null {
  try {
    const token = localStorage.getItem("athena_jwt");
    if (!token) return null;
    const payload = JSON.parse(atob(token.split(".")[1]));
    return payload.role ?? payload.roles?.[0] ?? null;
  } catch {
    return null;
  }
}

const WHT_RATE = 0.15;

const InterestAccrualPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [postingAccountId, setPostingAccountId] = useState<string | null>(null);
  const [showEodConfirm, setShowEodConfirm] = useState(false);

  const userRole = getUserRole();
  const isAdmin = userRole === "ADMIN" || userRole === "ROLE_ADMIN";

  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["accounts", "list", "interest"],
    queryFn: () => accountService.listAccounts(0, 200),
    staleTime: 60_000,
    retry: false,
  });

  const allAccounts = apiPage?.content ?? [];
  const accountsWithInterest = allAccounts.filter(
    (a) => (a.accruedInterest ?? 0) > 0
  );

  const todayStr = new Date().toISOString().slice(0, 10);
  const monthStr = new Date().toISOString().slice(0, 7);

  const todaysAccrual = accountsWithInterest
    .filter((a) => a.lastInterestAccrualDate?.startsWith(todayStr))
    .reduce((sum, a) => sum + (a.accruedInterest ?? 0), 0);

  const mtdAccrual = accountsWithInterest
    .filter((a) => a.lastInterestAccrualDate?.startsWith(monthStr))
    .reduce((sum, a) => sum + (a.accruedInterest ?? 0), 0);

  const pendingTotal = accountsWithInterest.reduce(
    (sum, a) => sum + (a.accruedInterest ?? 0),
    0
  );

  const totalEarningAccounts = accountsWithInterest.length;

  const totalGross = pendingTotal;
  const totalWHT = totalGross * WHT_RATE;
  const totalNet = totalGross - totalWHT;

  const postInterestMutation = useMutation({
    mutationFn: (accountId: string) => accountService.postInterest(accountId),
    onSuccess: (_data, accountId) => {
      toast({
        title: "Interest Posted",
        description: `Interest posted successfully for account ${accountId}.`,
      });
      queryClient.invalidateQueries({ queryKey: ["accounts"] });
      setPostingAccountId(null);
    },
    onError: (error: Error) => {
      toast({
        title: "Posting Failed",
        description: error.message,
        variant: "destructive",
      });
      setPostingAccountId(null);
    },
  });

  const eodMutation = useMutation({
    mutationFn: () => accountService.runEOD(),
    onSuccess: (result) => {
      toast({
        title: "EOD Completed",
        description: `Accrued: ${result.accountsAccrued}, Dormant: ${result.dormantDetected}, Matured: ${result.maturedProcessed}`,
      });
      queryClient.invalidateQueries({ queryKey: ["accounts"] });
      setShowEodConfirm(false);
    },
    onError: (error: Error) => {
      toast({
        title: "EOD Failed",
        description: error.message,
        variant: "destructive",
      });
      setShowEodConfirm(false);
    },
  });

  const handlePostInterest = (accountId: string) => {
    setPostingAccountId(accountId);
    postInterestMutation.mutate(accountId);
  };

  const handleRunEOD = () => {
    if (!showEodConfirm) {
      setShowEodConfirm(true);
      return;
    }
    eodMutation.mutate();
  };

  return (
    <DashboardLayout
      title="Interest Accrual"
      subtitle="Daily interest accrual monitoring and posting"
    >
      <div className="space-y-4 animate-fade-in">
        {/* KPI Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <Card>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Today's Accrual Total
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-32" />
              ) : (
                <p className="text-2xl font-bold">{formatCurrency(todaysAccrual)}</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                MTD Accrual
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-32" />
              ) : (
                <p className="text-2xl font-bold">{formatCurrency(mtdAccrual)}</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Pending Posting Total
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-32" />
              ) : (
                <p className="text-2xl font-bold text-amber-600">{formatCurrency(pendingTotal)}</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Accounts Earning Interest
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-16" />
              ) : (
                <p className="text-2xl font-bold">{totalEarningAccounts}</p>
              )}
            </CardContent>
          </Card>
        </div>

        {/* EOD Button (admin only) */}
        {isAdmin && (
          <div className="flex items-center gap-3">
            {showEodConfirm ? (
              <>
                <span className="text-sm text-amber-600 font-medium">
                  Are you sure you want to run End-of-Day processing?
                </span>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={handleRunEOD}
                  disabled={eodMutation.isPending}
                >
                  {eodMutation.isPending ? "Running..." : "Confirm Run EOD"}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setShowEodConfirm(false)}
                  disabled={eodMutation.isPending}
                >
                  Cancel
                </Button>
              </>
            ) : (
              <Button size="sm" onClick={handleRunEOD}>
                Run EOD
              </Button>
            )}
          </div>
        )}

        {/* WHT Summary */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">
              Withholding Tax Summary (Pending)
            </CardTitle>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-6 w-48" />
                <Skeleton className="h-6 w-48" />
                <Skeleton className="h-6 w-48" />
              </div>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div>
                  <p className="text-xs text-muted-foreground uppercase tracking-wider">
                    Total Gross Interest
                  </p>
                  <p className="text-lg font-semibold mt-1">{formatCurrency(totalGross)}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground uppercase tracking-wider">
                    Total WHT Deducted (15%)
                  </p>
                  <p className="text-lg font-semibold mt-1 text-red-600">
                    {formatCurrency(totalWHT)}
                  </p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground uppercase tracking-wider">
                    Total Net to Post
                  </p>
                  <p className="text-lg font-semibold mt-1 text-green-600">
                    {formatCurrency(totalNet)}
                  </p>
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Table */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">
              Accounts with Unposted Interest
              <span className="ml-2 text-muted-foreground font-normal">
                ({accountsWithInterest.length})
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : accountsWithInterest.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No pending interest</p>
                <p className="text-xs mt-1">
                  No accounts currently have unposted accrued interest.
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider">Account #</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">Customer</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">Type</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider text-right">Balance</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider text-right">Accrued Interest</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">Last Accrual Date</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {accountsWithInterest.map((acc) => (
                    <TableRow key={acc.id} className="table-row-hover">
                      <TableCell className="text-xs font-mono">{acc.accountNumber}</TableCell>
                      <TableCell className="text-xs font-medium">
                        {acc.accountName || acc.customerId}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-[11px]">
                          {acc.accountType}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs font-medium text-right">
                        {formatCurrency(getBalance(acc), acc.currency)}
                      </TableCell>
                      <TableCell className="text-xs font-medium text-right text-amber-600">
                        {formatCurrency(acc.accruedInterest, acc.currency)}
                      </TableCell>
                      <TableCell className="text-xs">
                        {formatDate(acc.lastInterestAccrualDate)}
                      </TableCell>
                      <TableCell>
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-7 text-xs"
                          disabled={postInterestMutation.isPending && postingAccountId === acc.id}
                          onClick={() => handlePostInterest(acc.id)}
                        >
                          {postInterestMutation.isPending && postingAccountId === acc.id
                            ? "Posting..."
                            : "Post Interest"}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default InterestAccrualPage;
