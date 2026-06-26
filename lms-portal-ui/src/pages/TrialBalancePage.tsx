import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Download } from "lucide-react";
import { formatKES } from "@/lib/format";
import { useQuery } from "@tanstack/react-query";
import { accountingService, type TrialBalanceAccount } from "@/services/accountingService";
import { PeriodSelector } from "@/components/PeriodSelector";
import { downloadFile } from "@/lib/api";
import { useToast } from "@/hooks/use-toast";

const TrialBalancePage = () => {
  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth() + 1);
  const { toast } = useToast();

  const handleDownloadCsv = async () => {
    try {
      await downloadFile(
        accountingService.csvUrl("trial-balance", year, month),
        `trial-balance-${year}-${String(month).padStart(2, "0")}.csv`
      );
    } catch (err) {
      toast({
        title: "Download failed",
        description: err instanceof Error ? err.message : "Could not export CSV.",
        variant: "destructive",
      });
    }
  };

  const { data, isLoading, isError } = useQuery({
    queryKey: ["accounting", "trial-balance", year, month],
    queryFn: () => accountingService.getTrialBalance(year, month),
    staleTime: 300_000,
    retry: false,
  });

  const accounts: TrialBalanceAccount[] = data?.accounts ?? [];
  const totalDebits = data?.totalDebits ?? 0;
  const totalCredits = data?.totalCredits ?? 0;
  const balanced = data?.balanced ?? false;

  return (
    <DashboardLayout
      title="Trial Balance"
      subtitle="General ledger — all accounts and balances"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Finance" }, { label: "Trial Balance" }]}
    >
      <div className="space-y-4 max-w-4xl">
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">Period:</span>
          <div className="flex items-center gap-2">
            <PeriodSelector year={year} month={month} onYearChange={setYear} onMonthChange={setMonth} />
            <Button variant="outline" size="sm" onClick={handleDownloadCsv} className="gap-1.5">
              <Download className="h-4 w-4" />
              Download CSV
            </Button>
          </div>
        </div>
        <Card>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 10 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : isError ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">Unable to load trial balance</p>
                <p className="text-xs mt-1">Accounting service returned an error.</p>
              </div>
            ) : accounts.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No GL accounts</p>
                <p className="text-xs mt-1">No trial balance data available.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-[10px] font-sans w-20">Code</TableHead>
                    <TableHead className="text-[10px] font-sans">Account Name</TableHead>
                    <TableHead className="text-[10px] font-sans">Type</TableHead>
                    <TableHead className="text-[10px] font-sans text-right">Debit (KES)</TableHead>
                    <TableHead className="text-[10px] font-sans text-right">Credit (KES)</TableHead>
                    <TableHead className="text-[10px] font-sans text-right">Balance (KES)</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {accounts.map((row) => {
                    const debit = row.balanceType === "DEBIT" ? row.balance : 0;
                    const credit = row.balanceType === "CREDIT" ? row.balance : 0;
                    return (
                      <TableRow key={row.accountId} className="table-row-hover">
                        <TableCell className="text-xs font-mono">{row.accountCode}</TableCell>
                        <TableCell className="text-xs font-sans">{row.accountName}</TableCell>
                        <TableCell className="text-xs font-sans text-muted-foreground">{row.accountType}</TableCell>
                        <TableCell className="text-xs font-mono text-right">
                          {debit ? formatKES(debit) : "—"}
                        </TableCell>
                        <TableCell className="text-xs font-mono text-right">
                          {credit ? formatKES(credit) : "—"}
                        </TableCell>
                        <TableCell className={`text-xs font-mono text-right font-semibold ${row.balance < 0 ? "text-destructive" : ""}`}>
                          {row.balance != null ? formatKES(Math.abs(row.balance)) : "—"}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                  {/* Totals */}
                  <TableRow className="border-t-2 bg-muted/30">
                    <TableCell colSpan={3} className="text-xs font-sans font-bold">Total</TableCell>
                    <TableCell className="text-sm font-mono text-right font-bold">{formatKES(totalDebits)}</TableCell>
                    <TableCell className="text-sm font-mono text-right font-bold">{formatKES(totalCredits)}</TableCell>
                    <TableCell />
                  </TableRow>
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {!isLoading && !isError && accounts.length > 0 && (
          <Card className={`border-2 ${balanced ? "border-success/30" : "border-destructive/30"}`}>
            <CardContent className="p-4 flex items-center justify-between">
              <span className="text-sm font-sans font-bold">Debit / Credit Check</span>
              <Badge
                variant="outline"
                className={`text-xs font-sans ${
                  balanced
                    ? "bg-success/15 text-success border-success/30"
                    : "bg-destructive/15 text-destructive border-destructive/30"
                }`}
              >
                {balanced
                  ? "Balanced"
                  : `Difference: ${formatKES(Math.abs(totalDebits - totalCredits))}`}
              </Badge>
            </CardContent>
          </Card>
        )}
      </div>
    </DashboardLayout>
  );
};

export default TrialBalancePage;
