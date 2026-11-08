import { useMemo } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { BarChart3, TrendingUp, Banknote, AlertTriangle } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { reportingService } from "@/services/reportingService";
import { formatKES } from "@/lib/format";

const ConsolidatedReportsPage = () => {
  const { data: summary, isLoading } = useQuery({
    queryKey: ["consolidated-summary"],
    queryFn: () => reportingService.getSummary(),
    staleTime: 60_000,
    retry: false,
  });

  const { data: snapshotsPage } = useQuery({
    queryKey: ["consolidated-snapshots"],
    queryFn: () => reportingService.getSnapshots(0, 10),
    staleTime: 60_000,
    retry: false,
  });

  const snapshots = snapshotsPage?.content ?? [];

  const rows = useMemo(() => {
    if (!summary) return [];
    return [
      { metric: "Total Loans", value: summary.totalLoans?.toLocaleString() ?? "0" },
      { metric: "Active Loans", value: summary.activeLoans?.toLocaleString() ?? "0" },
      { metric: "Closed Loans", value: summary.closedLoans?.toLocaleString() ?? "0" },
      { metric: "Total Disbursed", value: formatKES(summary.totalDisbursed ?? 0) },
      { metric: "Total Outstanding", value: formatKES(summary.totalOutstanding ?? 0) },
      { metric: "Total Collected", value: formatKES(summary.totalCollected ?? 0) },
      { metric: "PAR 30 Loans", value: summary.par30?.toLocaleString() ?? "0" },
      { metric: "PAR 90 Loans", value: summary.par90?.toLocaleString() ?? "0" },
      { metric: "Watch Loans", value: summary.watchLoans?.toLocaleString() ?? "0" },
      { metric: "Substandard", value: summary.substandardLoans?.toLocaleString() ?? "0" },
      { metric: "Doubtful", value: summary.doubtfulLoans?.toLocaleString() ?? "0" },
      { metric: "Loss", value: summary.lossLoans?.toLocaleString() ?? "0" },
    ];
  }, [summary]);

  return (
    <DashboardLayout
      title="Consolidated Reports"
      subtitle="Multi-entity consolidated financial reporting"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Reports" }, { label: "Consolidated" }]}
    >
      <div className="space-y-4 animate-fade-in">
        {isLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-24 w-full" />)}
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Active Loans</span>
                  <BarChart3 className="h-4 w-4 text-info" />
                </div>
                <p className="text-2xl font-heading">{summary?.activeLoans?.toLocaleString() ?? "0"}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Total Disbursed</span>
                  <Banknote className="h-4 w-4 text-success" />
                </div>
                <p className="text-2xl font-heading">{formatKES(summary?.totalDisbursed ?? 0)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Outstanding</span>
                  <TrendingUp className="h-4 w-4 text-warning" />
                </div>
                <p className="text-2xl font-heading">{formatKES(summary?.totalOutstanding ?? 0)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">PAR 30</span>
                  <AlertTriangle className="h-4 w-4 text-destructive" />
                </div>
                <p className="text-2xl font-heading">{summary?.par30 ?? 0}</p>
              </CardContent>
            </Card>
          </div>
        )}

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Portfolio Summary</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-8 w-full" />)}
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Metric</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Value</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((r) => (
                    <TableRow key={r.metric} className="table-row-hover">
                      <TableCell className="text-xs font-sans">{r.metric}</TableCell>
                      <TableCell className="text-xs font-mono text-right font-semibold">{r.value}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {snapshots.length > 0 && (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium">Recent Snapshots</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Date</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Active</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Disbursed</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Outstanding</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">PAR 30</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {snapshots.map((s) => (
                    <TableRow key={s.id} className="table-row-hover">
                      <TableCell className="text-xs font-sans">{s.snapshotDate?.split("T")[0] ?? "—"}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{s.activeLoans ?? 0}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(s.totalDisbursed ?? 0)}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(s.totalOutstanding ?? 0)}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{s.par30 ?? 0}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}
      </div>
    </DashboardLayout>
  );
};

export default ConsolidatedReportsPage;
