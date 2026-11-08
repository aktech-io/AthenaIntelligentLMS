import { useMemo } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { BarChart3, TrendingUp, Banknote, AlertTriangle, RefreshCw } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { reportingService } from "@/services/reportingService";
import { formatKES } from "@/lib/format";
import { useToast } from "@/hooks/use-toast";

const ReportsPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ["reporting-summary"],
    queryFn: () => reportingService.getSummary(),
    staleTime: 60_000,
    retry: false,
  });

  const { data: eventsPage, isLoading: eventsLoading } = useQuery({
    queryKey: ["reporting-events"],
    queryFn: () => reportingService.getEvents(0, 20),
    staleTime: 60_000,
    retry: false,
  });

  const snapshotMutation = useMutation({
    mutationFn: () => reportingService.generateSnapshot(),
    onSuccess: () => {
      toast({ title: "Snapshot Generated", description: "Portfolio snapshot has been generated" });
      queryClient.invalidateQueries({ queryKey: ["reporting-summary"] });
      queryClient.invalidateQueries({ queryKey: ["reporting-events"] });
    },
    onError: (err: Error) => {
      toast({ title: "Snapshot Failed", description: err.message, variant: "destructive" });
    },
  });

  const events = eventsPage?.content ?? [];

  const kpis = useMemo(() => {
    if (!summary) return [];
    return [
      { label: "Active Loans", value: summary.activeLoans?.toLocaleString() ?? "0", icon: BarChart3, color: "text-info" },
      { label: "Total Disbursed", value: formatKES(summary.totalDisbursed ?? 0), icon: Banknote, color: "text-success" },
      { label: "Outstanding", value: formatKES(summary.totalOutstanding ?? 0), icon: TrendingUp, color: "text-warning" },
      { label: "PAR 30", value: `${summary.par30 ?? 0} loans`, icon: AlertTriangle, color: "text-destructive" },
    ];
  }, [summary]);

  return (
    <DashboardLayout
      title="Reports & Analytics"
      subtitle="Portfolio summary and reporting events"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Reports" }]}
    >
      <div className="space-y-4 animate-fade-in">
        {/* Action bar */}
        <div className="flex items-center justify-between">
          <div className="text-sm text-muted-foreground font-sans">
            {summaryLoading ? "Loading..." : `As of ${summary?.asOfDate ?? "today"}`}
          </div>
          <Button
            size="sm"
            variant="outline"
            className="text-xs font-sans"
            onClick={() => snapshotMutation.mutate()}
            disabled={snapshotMutation.isPending}
          >
            <RefreshCw className={`h-3.5 w-3.5 mr-1.5 ${snapshotMutation.isPending ? "animate-spin" : ""}`} />
            {snapshotMutation.isPending ? "Generating..." : "Generate Snapshot"}
          </Button>
        </div>

        {/* KPI Cards */}
        {summaryLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-24 w-full" />)}
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {kpis.map((kpi) => (
              <Card key={kpi.label}>
                <CardContent className="p-5">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs text-muted-foreground font-sans">{kpi.label}</span>
                    <kpi.icon className={`h-4 w-4 ${kpi.color}`} />
                  </div>
                  <p className="text-2xl font-heading">{kpi.value}</p>
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        {/* Additional Summary */}
        {summary && (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {[
              { label: "Total Loans", value: summary.totalLoans?.toLocaleString() ?? "0" },
              { label: "Closed Loans", value: summary.closedLoans?.toLocaleString() ?? "0" },
              { label: "Total Collected", value: formatKES(summary.totalCollected ?? 0) },
              { label: "Watch Loans", value: summary.watchLoans?.toLocaleString() ?? "0" },
            ].map((item) => (
              <Card key={item.label} className="p-3">
                <p className="text-[11px] text-muted-foreground font-sans uppercase tracking-wider">{item.label}</p>
                <p className="text-sm font-mono font-bold mt-0.5">{item.value}</p>
              </Card>
            ))}
          </div>
        )}

        {/* Recent Events */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Recent Reporting Events</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {eventsLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : events.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                <p className="text-sm font-medium">No events yet</p>
                <p className="text-xs mt-1">Generate a snapshot to see reporting events.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Event Type</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Category</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Date</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {events.map((evt) => (
                    <TableRow key={evt.id} className="table-row-hover">
                      <TableCell className="text-xs font-mono">{evt.eventType ?? "—"}</TableCell>
                      <TableCell className="text-xs font-sans">{evt.eventCategory ?? "—"}</TableCell>
                      <TableCell className="text-xs font-sans">{evt.occurredAt?.split("T")[0] ?? evt.createdAt?.split("T")[0] ?? "—"}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-[11px] bg-success/15 text-success border-success/30">
                          Processed
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

export default ReportsPage;
