import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Search, ChevronLeft, ChevronRight, Receipt } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { accountingService, type JournalEntry } from "@/services/accountingService";
import { formatKESFull } from "@/lib/format";

function txnType(reference: string): { label: string; cls: string } {
  if (reference?.startsWith("DISB-")) return { label: "Disbursement", cls: "bg-info/15 text-info border-info/30" };
  if (reference?.startsWith("RPMT-")) return { label: "Repayment", cls: "bg-success/15 text-success border-success/30" };
  if (reference?.startsWith("FEE-")) return { label: "Fee", cls: "bg-warning/15 text-warning border-warning/30" };
  if (reference?.startsWith("FLOAT-")) return { label: "Float", cls: "bg-accent/15 text-accent border-accent/30" };
  return { label: "Journal", cls: "bg-muted text-muted-foreground border-border" };
}

const TransactionsPage = () => {
  const [page, setPage] = useState(0);
  const [search, setSearch] = useState("");
  const pageSize = 50;

  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["journal-entries", page],
    queryFn: () => accountingService.listJournalEntries(page, pageSize),
    staleTime: 60_000,
    retry: false,
  });

  const entries: JournalEntry[] = apiPage?.content ?? [];
  const totalElements = apiPage?.totalElements ?? 0;
  const totalPages = apiPage?.totalPages ?? 1;

  const filtered = search
    ? entries.filter(
        (e) =>
          e.reference?.toLowerCase().includes(search.toLowerCase()) ||
          e.description?.toLowerCase().includes(search.toLowerCase())
      )
    : entries;

  return (
    <DashboardLayout
      title="Transactions"
      subtitle="General ledger journal entries"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Finance" }, { label: "Transactions" }]}
    >
      <div className="space-y-4 animate-fade-in">
        <div className="flex items-center justify-between">
          <div className="text-sm text-muted-foreground font-sans">
            {isLoading ? "Loading..." : `${totalElements.toLocaleString()} journal entries`}
          </div>
          <div className="relative w-72">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <Input
              placeholder="Search by reference or description..."
              className="pl-8 h-9 text-xs font-sans"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
        </div>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Receipt className="h-4 w-4" /> Journal Entries
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : filtered.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No journal entries found</p>
                <p className="text-xs mt-1">
                  {search ? "No entries match your search." : "No accounting entries have been recorded yet."}
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Reference</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Date</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Type</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Description</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Debit</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Credit</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((entry) => {
                    const t = txnType(entry.reference);
                    return (
                      <TableRow key={entry.id} className="table-row-hover">
                        <TableCell className="text-xs font-mono font-medium">{entry.reference}</TableCell>
                        <TableCell className="text-xs font-sans">{entry.entryDate?.split("T")[0] ?? "—"}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[11px] font-sans ${t.cls}`}>{t.label}</Badge>
                        </TableCell>
                        <TableCell className="text-xs font-sans text-muted-foreground max-w-[200px] truncate">
                          {entry.description}
                        </TableCell>
                        <TableCell className="text-xs font-mono text-right">
                          {entry.totalDebit > 0 ? formatKESFull(entry.totalDebit) : "—"}
                        </TableCell>
                        <TableCell className="text-xs font-mono text-right">
                          {entry.totalCredit > 0 ? formatKESFull(entry.totalCredit) : "—"}
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className="text-[11px] font-sans bg-success/15 text-success border-success/30">
                            {entry.status ?? "POSTED"}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Pagination */}
        {!isLoading && totalPages > 1 && (
          <div className="flex items-center justify-between text-xs text-muted-foreground font-sans">
            <span>
              Page {page + 1} of {totalPages} ({totalElements.toLocaleString()} entries)
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                className="h-7 text-[11px]"
                disabled={page === 0}
                onClick={() => setPage((p) => p - 1)}
              >
                <ChevronLeft className="h-3 w-3 mr-1" /> Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="h-7 text-[11px]"
                disabled={page >= totalPages - 1}
                onClick={() => setPage((p) => p + 1)}
              >
                Next <ChevronRight className="h-3 w-3 ml-1" />
              </Button>
            </div>
          </div>
        )}
      </div>
    </DashboardLayout>
  );
};

export default TransactionsPage;
