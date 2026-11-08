import { useState, useMemo } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Banknote, TrendingUp, ArrowDownRight, Percent, ChevronLeft, ChevronRight } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { floatService, type FloatAccount, type FloatTransaction } from "@/services/floatService";
import { formatKES } from "@/lib/format";

const FloatAnalyticsPage = () => {
  const [txnPage, setTxnPage] = useState(0);

  const { data: accounts, isLoading: accountsLoading } = useQuery({
    queryKey: ["float-accounts"],
    queryFn: () => floatService.listFloatAccounts(),
    staleTime: 60_000,
    retry: false,
  });

  const floatAccounts: FloatAccount[] = Array.isArray(accounts) ? accounts : (accounts as { content?: FloatAccount[] })?.content ?? [];
  const primaryAccount = floatAccounts.length > 0 ? floatAccounts[0] : null;

  const { data: txnData, isLoading: txnLoading } = useQuery({
    queryKey: ["float-transactions", primaryAccount?.id, txnPage],
    queryFn: () => floatService.getTransactions(primaryAccount!.id, txnPage, 30),
    enabled: !!primaryAccount?.id,
    staleTime: 60_000,
    retry: false,
  });

  const transactions: FloatTransaction[] = txnData?.content ?? [];
  const txnTotalPages = txnData?.totalPages ?? 1;

  const stats = useMemo(() => {
    if (!primaryAccount) return { limit: 0, drawn: 0, available: 0, utilization: 0 };
    const limit = primaryAccount.floatLimit ?? 0;
    const drawn = primaryAccount.drawnAmount ?? 0;
    const available = primaryAccount.available ?? (limit - drawn);
    const utilization = limit > 0 ? (drawn / limit) * 100 : 0;
    return { limit, drawn, available, utilization };
  }, [primaryAccount]);

  return (
    <DashboardLayout
      title="Float Analytics"
      subtitle="Float deployment and utilisation trends"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Float & Wallet" }, { label: "Float Analytics" }]}
    >
      <div className="space-y-4 animate-fade-in">
        {/* KPI Cards */}
        {accountsLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-24 w-full" />)}
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Float Limit</span>
                  <Banknote className="h-4 w-4 text-info" />
                </div>
                <p className="text-2xl font-heading">{formatKES(stats.limit)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Drawn Amount</span>
                  <TrendingUp className="h-4 w-4 text-warning" />
                </div>
                <p className="text-2xl font-heading">{formatKES(stats.drawn)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Available</span>
                  <ArrowDownRight className="h-4 w-4 text-success" />
                </div>
                <p className="text-2xl font-heading">{formatKES(stats.available)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Utilization</span>
                  <Percent className="h-4 w-4 text-accent" />
                </div>
                <p className="text-2xl font-heading">{stats.utilization.toFixed(1)}%</p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Float Accounts */}
        {floatAccounts.length > 1 && (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">Float Accounts</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Account</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Code</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Limit</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Drawn</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Available</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {floatAccounts.map((acc) => (
                    <TableRow key={acc.id} className="table-row-hover">
                      <TableCell className="text-xs font-sans font-medium">{acc.accountName}</TableCell>
                      <TableCell className="text-xs font-mono text-muted-foreground">{acc.accountCode}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(acc.floatLimit)}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(acc.drawnAmount)}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(acc.available)}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-[11px] font-sans bg-success/15 text-success border-success/30">
                          {acc.status}
                        </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}

        {/* Float Transactions */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">
              Float Transactions {primaryAccount ? `— ${primaryAccount.accountName}` : ""}
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {txnLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : transactions.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                <p className="text-sm font-medium">No transactions</p>
                <p className="text-xs mt-1">No float transactions have been recorded yet.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Reference</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Type</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Amount</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Description</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Date</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {transactions.map((txn) => (
                    <TableRow key={txn.id} className="table-row-hover">
                      <TableCell className="text-xs font-mono font-medium">{txn.reference ?? txn.id?.slice(0, 8)}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`text-[11px] font-sans ${
                          txn.transactionType?.includes("DRAW") ? "bg-warning/15 text-warning border-warning/30" :
                          txn.transactionType?.includes("REPLENISH") ? "bg-success/15 text-success border-success/30" :
                          "bg-muted text-muted-foreground border-border"
                        }`}>
                          {txn.transactionType}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs font-mono text-right font-semibold">{formatKES(txn.amount)}</TableCell>
                      <TableCell className="text-xs font-sans text-muted-foreground">{txn.description ?? "—"}</TableCell>
                      <TableCell className="text-xs font-sans">{txn.valueDate?.split("T")[0] ?? txn.createdAt?.split("T")[0] ?? "—"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Pagination */}
        {!txnLoading && txnTotalPages > 1 && (
          <div className="flex items-center justify-between text-xs text-muted-foreground font-sans">
            <span>Page {txnPage + 1} of {txnTotalPages}</span>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" className="h-7 text-[11px]" disabled={txnPage === 0} onClick={() => setTxnPage(p => p - 1)}>
                <ChevronLeft className="h-3 w-3 mr-1" /> Previous
              </Button>
              <Button variant="outline" size="sm" className="h-7 text-[11px]" disabled={txnPage >= txnTotalPages - 1} onClick={() => setTxnPage(p => p + 1)}>
                Next <ChevronRight className="h-3 w-3 ml-1" />
              </Button>
            </div>
          </div>
        )}
      </div>
    </DashboardLayout>
  );
};

export default FloatAnalyticsPage;
