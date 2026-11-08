import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { accountService, type Account } from "@/services/accountService";
import { useState } from "react";

type FilterMode = "all" | "this_week" | "this_month" | "matured";

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

function isWithinDays(dateStr: string | undefined, days: number): boolean {
  if (!dateStr) return false;
  const maturity = new Date(dateStr);
  const now = new Date();
  const diffMs = maturity.getTime() - now.getTime();
  return diffMs >= 0 && diffMs <= days * 24 * 60 * 60 * 1000;
}

function isMatured(dateStr: string | undefined): boolean {
  if (!dateStr) return false;
  return new Date(dateStr).getTime() < Date.now();
}

const FixedDepositsPage = () => {
  const navigate = useNavigate();
  const [filter, setFilter] = useState<FilterMode>("all");

  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["accounts", "list", "fd"],
    queryFn: () => accountService.listAccounts(0, 200),
    staleTime: 60_000,
    retry: false,
  });

  const allFDs = (apiPage?.content ?? []).filter(
    (a) => a.accountType === "FIXED_DEPOSIT"
  );

  const filteredFDs = allFDs.filter((fd) => {
    switch (filter) {
      case "this_week":
        return isWithinDays(fd.maturityDate, 7);
      case "this_month":
        return isWithinDays(fd.maturityDate, 30);
      case "matured":
        return isMatured(fd.maturityDate);
      default:
        return true;
    }
  });

  const totalPortfolio = allFDs.reduce((sum, fd) => sum + (fd.lockedAmount ?? 0), 0);
  const maturingThisMonth = allFDs.filter((fd) => isWithinDays(fd.maturityDate, 30)).length;
  const averageRate =
    allFDs.length > 0
      ? allFDs.reduce((sum, fd) => sum + (fd.interestRateOverride ?? 0), 0) / allFDs.length
      : 0;
  const totalFDAccounts = allFDs.length;

  const statusColor = (status: string) => {
    const s = status?.toUpperCase();
    if (s === "ACTIVE") return "bg-success/15 text-success border-success/30";
    if (s === "MATURED") return "bg-amber-500/15 text-amber-600 border-amber-500/30";
    if (s === "CLOSED") return "bg-muted text-muted-foreground";
    return "bg-muted text-muted-foreground";
  };

  return (
    <DashboardLayout
      title="Fixed Deposits"
      subtitle="Term deposit portfolio management"
    >
      <div className="space-y-4 animate-fade-in">
        {/* KPI Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <Card>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Total FD Portfolio
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-32" />
              ) : (
                <p className="text-2xl font-bold">{formatCurrency(totalPortfolio)}</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Maturing This Month
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-16" />
              ) : (
                <p className="text-2xl font-bold">{maturingThisMonth}</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Average Rate
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-16" />
              ) : (
                <p className="text-2xl font-bold">{averageRate.toFixed(2)}%</p>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1">
              <CardTitle className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                Total FD Accounts
              </CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-8 w-16" />
              ) : (
                <p className="text-2xl font-bold">{totalFDAccounts}</p>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Filter Buttons */}
        <div className="flex gap-2">
          {([
            { key: "all", label: "All" },
            { key: "this_week", label: "Maturing This Week" },
            { key: "this_month", label: "Maturing This Month" },
            { key: "matured", label: "Matured" },
          ] as { key: FilterMode; label: string }[]).map((f) => (
            <Button
              key={f.key}
              variant={filter === f.key ? "default" : "outline"}
              size="sm"
              onClick={() => setFilter(f.key)}
            >
              {f.label}
            </Button>
          ))}
        </div>

        {/* Table */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">
              Fixed Deposit Accounts
              <span className="ml-2 text-muted-foreground font-normal">
                ({filteredFDs.length})
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
            ) : filteredFDs.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No fixed deposits found</p>
                <p className="text-xs mt-1">
                  {filter === "all"
                    ? "No FIXED_DEPOSIT accounts exist yet."
                    : "No deposits match the selected filter."}
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider">Account #</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">Customer</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider text-right">Amount</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider text-right">Rate</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider text-right">Term</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">Maturity Date</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">Auto-Renew</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredFDs.map((fd) => (
                    <TableRow
                      key={fd.id}
                      className="table-row-hover cursor-pointer"
                      onClick={() => navigate(`/account/${fd.id}`)}
                    >
                      <TableCell className="text-xs font-mono">{fd.accountNumber}</TableCell>
                      <TableCell className="text-xs font-medium">
                        {fd.accountName || fd.customerId}
                      </TableCell>
                      <TableCell className="text-xs font-medium text-right">
                        {formatCurrency(fd.lockedAmount, fd.currency)}
                      </TableCell>
                      <TableCell className="text-xs text-right">
                        {fd.interestRateOverride != null
                          ? `${fd.interestRateOverride.toFixed(2)}%`
                          : "\u2014"}
                      </TableCell>
                      <TableCell className="text-xs text-right">
                        {fd.termDays != null ? `${fd.termDays}d` : "\u2014"}
                      </TableCell>
                      <TableCell className="text-xs">{formatDate(fd.maturityDate)}</TableCell>
                      <TableCell className="text-xs">
                        {fd.autoRenew ? (
                          <Badge variant="outline" className="text-[11px] bg-blue-500/15 text-blue-600 border-blue-500/30">
                            Yes
                          </Badge>
                        ) : (
                          <span className="text-muted-foreground">No</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={`text-[11px] font-semibold ${statusColor(fd.status)}`}
                        >
                          {fd.status}
                        </Badge>
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

export default FixedDepositsPage;
