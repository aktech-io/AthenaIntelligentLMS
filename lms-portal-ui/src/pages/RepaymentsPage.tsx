import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { CalendarDays, CheckCircle, BookOpen } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { accountingService, type JournalEntry } from "@/services/accountingService";

const RepaymentsPage = () => {
  const { data: page, isLoading, isError } = useQuery({
    queryKey: ["accounting", "journal-entries", "repayments"],
    queryFn: () => accountingService.listJournalEntries(0, 100),
    staleTime: 60_000,
    retry: false,
  });

  // Filter to repayment entries: description starts with RPMT- or sourceEventType is repayment-related
  const entries: JournalEntry[] = (page?.content ?? []).filter(
    (e) =>
      e.description?.startsWith("RPMT-") ||
      e.sourceEventType?.toUpperCase().includes("REPAY") ||
      e.description?.toUpperCase().includes("REPAYMENT")
  );

  const totalEntries = entries.length;
  const totalAmount = entries.reduce((sum, e) => {
    const debit = (e.lines ?? []).reduce((s, l) => s + (l.debitAmount ?? 0), 0);
    return sum + debit;
  }, 0);

  function entryAmount(e: JournalEntry): number {
    return (e.lines ?? []).reduce((s, l) => s + (l.debitAmount ?? 0), 0);
  }

  return (
    <DashboardLayout
      title="Repayments"
      subtitle="Repayment journal entries from accounting service"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Lending" }, { label: "Repayments" }]}
    >
      <div className="space-y-6 animate-fade-in">
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Repayment Entries</span>
                <CalendarDays className="h-4 w-4 text-muted-foreground" />
              </div>
              {isLoading ? (
                <Skeleton className="h-8 w-16" />
              ) : (
                <p className="text-2xl font-heading">{totalEntries}</p>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Total Amount (KES)</span>
                <CheckCircle className="h-4 w-4 text-success" />
              </div>
              {isLoading ? (
                <Skeleton className="h-8 w-32" />
              ) : (
                <p className="text-2xl font-heading">
                  {totalAmount.toLocaleString("en-KE", { maximumFractionDigits: 0 })}
                </p>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-muted-foreground font-sans">Source</span>
                <BookOpen className="h-4 w-4 text-info" />
              </div>
              <p className="text-sm font-sans text-muted-foreground">Accounting GL</p>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">Repayment Journal Entries</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : isError ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">Unable to load repayments</p>
                <p className="text-xs mt-1">Accounting service returned an error.</p>
              </div>
            ) : entries.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No repayment entries</p>
                <p className="text-xs mt-1">No RPMT journal entries found in the accounting ledger.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Reference</TableHead>
                    <TableHead className="text-xs">Description</TableHead>
                    <TableHead className="text-xs">Date</TableHead>
                    <TableHead className="text-xs text-right">Amount (KES)</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((e) => (
                    <TableRow key={e.id} className="table-row-hover">
                      <TableCell className="text-xs font-mono">{e.description?.split(" ")[0] ?? e.id}</TableCell>
                      <TableCell className="text-xs">{e.description}</TableCell>
                      <TableCell className="text-xs">{e.journalDate}</TableCell>
                      <TableCell className="text-xs text-right font-mono">
                        {entryAmount(e).toLocaleString("en-KE", { maximumFractionDigits: 0 })}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={`text-[11px] ${
                            e.status === "POSTED"
                              ? "bg-success/10 text-success border-success/20"
                              : "bg-muted text-muted-foreground"
                          }`}
                        >
                          {e.status}
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

export default RepaymentsPage;
