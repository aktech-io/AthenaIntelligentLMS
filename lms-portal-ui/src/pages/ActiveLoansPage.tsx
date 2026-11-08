import { useState, useMemo } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { formatKES } from "@/lib/format";
import { Search, Download } from "lucide-react";
import { motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { useQuery } from "@tanstack/react-query";
import { loanManagementService, type Loan } from "@/services/loanManagementService";

interface ActiveLoan {
  id: string;
  customer: string;
  product: string;
  outstanding: number;
  emi: number;
  nextDue: string;
  dpd: number;
  stage: "current" | "watch" | "delinquent" | "npl";
  rate: number;
}

const stageConfig: Record<ActiveLoan["stage"], { label: string; cls: string }> = {
  current: { label: "Current", cls: "bg-success/15 text-success border-success/30" },
  watch: { label: "Watch", cls: "bg-warning/15 text-warning border-warning/30" },
  delinquent: { label: "Delinquent", cls: "bg-destructive/15 text-destructive border-destructive/30" },
  npl: { label: "NPL", cls: "bg-destructive/25 text-destructive border-destructive/40 font-bold" },
};

const dpdColor = (dpd: number) =>
  dpd === 0 ? "text-success" : dpd <= 7 ? "text-warning" : dpd <= 30 ? "text-destructive" : "text-destructive font-bold";

function loanToStage(loan: Loan): ActiveLoan["stage"] {
  if (loan.dpd === 0) return "current";
  if (loan.dpd <= 7) return "watch";
  if (loan.dpd <= 90) return "delinquent";
  return "npl";
}

function adaptLoan(loan: Loan): ActiveLoan {
  return {
    id: loan.id,
    customer: loan.customerId,
    product: loan.productId ?? "—",
    outstanding: loan.outstandingPrincipal,
    emi: 0,
    nextDue: loan.nextDueDate ?? "—",
    dpd: loan.dpd,
    stage: loanToStage(loan),
    rate: loan.interestRate ?? 0,
  };
}

const ActiveLoansPage = () => {
  const [search, setSearch] = useState("");
  const [stageFilter, setStageFilter] = useState<ActiveLoan["stage"] | "all">("all");
  const navigate = useNavigate();

  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["loans", "active"],
    queryFn: () => loanManagementService.listLoans(0, 100, "ACTIVE"),
    staleTime: 60_000,
    retry: false,
  });

  const loans: ActiveLoan[] =
    apiPage && apiPage.content.length > 0
      ? apiPage.content.map(adaptLoan)
      : [];

  const filtered = useMemo(
    () =>
      loans.filter((l) => {
        const matchSearch =
          !search ||
          l.customer.toLowerCase().includes(search.toLowerCase()) ||
          l.id.toLowerCase().includes(search.toLowerCase());
        const matchStage = stageFilter === "all" || l.stage === stageFilter;
        return matchSearch && matchStage;
      }),
    [loans, search, stageFilter]
  );

  const summary = useMemo(
    () => ({
      total: loans.length,
      outstanding: loans.reduce((s, l) => s + l.outstanding, 0),
      current: loans.filter((l) => l.stage === "current").length,
      par30: loans.filter((l) => l.dpd > 30).length,
    }),
    [loans]
  );

  const summaryCards = [
    { label: "Total Active", value: summary.total.toLocaleString(), sub: "loans" },
    { label: "Outstanding", value: formatKES(summary.outstanding), sub: "total book" },
    { label: "Current", value: summary.current.toLocaleString(), sub: "0 DPD" },
    { label: "PAR 30+", value: summary.par30.toLocaleString(), sub: "delinquent" },
  ];

  return (
    <DashboardLayout
      title="Active Loans"
      subtitle="Portfolio overview and individual loan monitoring"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Lending" }, { label: "Active Loans" }]}
    >
      <div className="space-y-4">
        {/* Summary bar */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          {isLoading
            ? Array.from({ length: 4 }).map((_, i) => (
                <Card key={i}>
                  <CardContent className="p-4">
                    <Skeleton className="h-4 w-2/3 mb-2" />
                    <Skeleton className="h-7 w-1/2" />
                  </CardContent>
                </Card>
              ))
            : summaryCards.map((c, i) => (
                <motion.div
                  key={c.label}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: i * 0.05 }}
                >
                  <Card>
                    <CardContent className="p-4">
                      <p className="text-[11px] uppercase tracking-wider text-muted-foreground font-sans">
                        {c.label}
                      </p>
                      <p className="text-lg font-mono font-bold mt-0.5">{c.value}</p>
                      <p className="text-[11px] text-muted-foreground font-sans">{c.sub}</p>
                    </CardContent>
                  </Card>
                </motion.div>
              ))}
        </div>

        {/* Toolbar */}
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <div className="relative w-64">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
              <Input
                placeholder="Search loans..."
                className="pl-8 h-9 text-xs font-sans"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <div className="flex gap-1">
              {(["all", "current", "watch", "delinquent", "npl"] as const).map((s) => (
                <Button
                  key={s}
                  size="sm"
                  variant={stageFilter === s ? "default" : "outline"}
                  className="text-[11px] h-7 font-sans capitalize"
                  onClick={() => setStageFilter(s)}
                >
                  {s === "all" ? "All" : stageConfig[s].label}
                </Button>
              ))}
            </div>
          </div>
          <Button size="sm" variant="outline" className="text-xs font-sans h-8">
            <Download className="h-3.5 w-3.5 mr-1.5" /> Export
          </Button>
        </div>

        {/* Table */}
        <Card>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 6 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : filtered.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No active loans found</p>
                <p className="text-xs mt-1">
                  No loan records returned from the backend.
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-[11px] font-sans">Loan ID</TableHead>
                    <TableHead className="text-[11px] font-sans">Customer</TableHead>
                    <TableHead className="text-[11px] font-sans">Product</TableHead>
                    <TableHead className="text-[11px] font-sans text-right">Outstanding</TableHead>
                    <TableHead className="text-[11px] font-sans text-right">EMI</TableHead>
                    <TableHead className="text-[11px] font-sans">Next Due</TableHead>
                    <TableHead className="text-[11px] font-sans text-center">DPD</TableHead>
                    <TableHead className="text-[11px] font-sans">Status</TableHead>
                    <TableHead className="text-[11px] font-sans text-right">Rate</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((loan) => (
                    <TableRow
                      key={loan.id}
                      className="table-row-hover cursor-pointer"
                      onClick={() => navigate(`/loan/${loan.id}`)}
                    >
                      <TableCell className="text-xs font-mono font-medium text-info">{loan.id}</TableCell>
                      <TableCell className="text-xs font-sans">{loan.customer}</TableCell>
                      <TableCell className="text-xs font-sans text-muted-foreground">{loan.product}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(loan.outstanding)}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{loan.emi > 0 ? formatKES(loan.emi) : "—"}</TableCell>
                      <TableCell className="text-xs font-sans text-muted-foreground">{loan.nextDue}</TableCell>
                      <TableCell className={`text-xs font-mono text-center font-semibold ${dpdColor(loan.dpd)}`}>
                        {loan.dpd}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={`text-[11px] font-sans ${stageConfig[loan.stage].cls}`}
                        >
                          {stageConfig[loan.stage].label}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs font-mono text-right">
                        {loan.rate > 0 ? `${loan.rate}%` : "—"}
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

export default ActiveLoansPage;
