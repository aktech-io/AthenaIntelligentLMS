import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Scale, Banknote, AlertTriangle } from "lucide-react";
import { collectionsService } from "@/services/collectionsService";
import { formatKES } from "@/lib/format";

const stageBadge = (stage: string) => {
  switch (stage) {
    case "WATCH":
      return <Badge className="bg-yellow-100 text-yellow-700 border-yellow-200 text-[11px]">Watch</Badge>;
    case "SUBSTANDARD":
      return <Badge className="bg-orange-100 text-orange-700 border-orange-200 text-[11px]">Substandard</Badge>;
    case "DOUBTFUL":
      return <Badge className="bg-red-100 text-red-700 border-red-200 text-[11px]">Doubtful</Badge>;
    case "LOSS":
      return <Badge className="bg-red-200 text-red-900 border-red-300 text-[11px]">Loss</Badge>;
    default:
      return <Badge variant="outline" className="text-[11px]">{stage}</Badge>;
  }
};

const priorityBadge = (priority: string) => {
  switch (priority) {
    case "LOW":
      return <Badge variant="outline" className="bg-gray-100 text-gray-600 border-gray-200 text-[11px]">Low</Badge>;
    case "NORMAL":
      return <Badge variant="outline" className="bg-blue-100 text-blue-700 border-blue-200 text-[11px]">Normal</Badge>;
    case "HIGH":
      return <Badge variant="outline" className="bg-orange-100 text-orange-700 border-orange-200 text-[11px]">High</Badge>;
    case "CRITICAL":
      return <Badge variant="outline" className="bg-red-100 text-red-700 border-red-200 text-[11px]">Critical</Badge>;
    default:
      return <Badge variant="outline" className="text-[11px]">{priority}</Badge>;
  }
};

const LegalPage = () => {
  const navigate = useNavigate();

  const { data: legalCases, isLoading: legalLoading } = useQuery({
    queryKey: ["legal-cases-pending"],
    queryFn: () => collectionsService.listCases(0, 100, { status: "PENDING_LEGAL" }),
    staleTime: 30_000,
    retry: false,
  });

  const { data: lossCases, isLoading: lossLoading } = useQuery({
    queryKey: ["legal-cases-loss"],
    queryFn: () => collectionsService.listCases(0, 100, { stage: "LOSS" }),
    staleTime: 30_000,
    retry: false,
  });

  const isLoading = legalLoading || lossLoading;

  // Merge and deduplicate
  const allCases = useMemo(() => {
    const map = new Map<string, typeof legalContent[0]>();
    const legalContent = legalCases?.content ?? [];
    const lossContent = lossCases?.content ?? [];
    for (const c of legalContent) map.set(c.id, c);
    for (const c of lossContent) map.set(c.id, c);
    return Array.from(map.values());
  }, [legalCases, lossCases]);

  const stats = useMemo(() => {
    const totalExposure = allCases.reduce((s, c) => s + c.outstandingAmount, 0);
    const avgDpd = allCases.length > 0
      ? Math.round(allCases.reduce((s, c) => s + c.currentDpd, 0) / allCases.length)
      : 0;
    return { total: allCases.length, totalExposure, avgDpd };
  }, [allCases]);

  return (
    <DashboardLayout
      title="Legal & Write-Offs"
      subtitle="Legal recovery and write-off management"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Collections" }, { label: "Legal & Write-Offs" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* KPI Cards */}
        {isLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Total Legal Cases</span>
                  <Scale className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="text-2xl font-heading">{stats.total}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Total Exposure</span>
                  <Banknote className="h-4 w-4 text-destructive" />
                </div>
                <p className="text-2xl font-heading">{formatKES(stats.totalExposure)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Average DPD</span>
                  <AlertTriangle className="h-4 w-4 text-warning" />
                </div>
                <p className="text-2xl font-heading">{stats.avgDpd}</p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Cases Table */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              Legal Cases
              {!isLoading && (
                <Badge variant="secondary" className="text-[11px]">{allCases.length}</Badge>
              )}
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : allCases.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <Scale className="h-8 w-8 mb-2 text-muted-foreground/50" />
                <p className="text-sm font-medium">No legal cases</p>
                <p className="text-xs mt-1">Legal cases will appear here when loans are escalated for recovery.</p>
                <p className="text-[11px] mt-3 text-muted-foreground/70">
                  Cases with status PENDING_LEGAL or stage LOSS are shown here.
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent">
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Case #</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Customer</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Loan ID</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">DPD</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Stage</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Priority</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans text-right">Outstanding</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Assigned To</TableHead>
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Opened</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {allCases.map((c) => (
                    <TableRow
                      key={c.id}
                      className="cursor-pointer table-row-hover"
                      onClick={() => navigate(`/collections/case/${c.id}`)}
                    >
                      <TableCell className="text-xs font-mono font-medium">{c.caseNumber}</TableCell>
                      <TableCell className="text-xs font-sans">{c.customerId ?? "---"}</TableCell>
                      <TableCell className="text-xs font-mono">{c.loanId?.slice(0, 8)}</TableCell>
                      <TableCell className="text-xs font-mono text-right font-bold text-red-700">{c.currentDpd}</TableCell>
                      <TableCell>{stageBadge(c.currentStage)}</TableCell>
                      <TableCell>{priorityBadge(c.priority)}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(c.outstandingAmount)}</TableCell>
                      <TableCell className="text-xs font-sans">{c.assignedTo ?? "Unassigned"}</TableCell>
                      <TableCell className="text-xs font-sans text-muted-foreground">
                        {new Date(c.openedAt).toLocaleDateString()}
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

export default LegalPage;
