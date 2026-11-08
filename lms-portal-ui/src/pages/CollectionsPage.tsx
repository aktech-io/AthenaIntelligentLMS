import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  AlertTriangle,
  Search,
  ChevronLeft,
  ChevronRight,
  Banknote,
  Eye,
  ShieldAlert,
  TriangleAlert,
  XCircle,
} from "lucide-react";
import { collectionsService } from "@/services/collectionsService";
import { formatKES } from "@/lib/format";

const STAGES = ["", "WATCH", "SUBSTANDARD", "DOUBTFUL", "LOSS"] as const;
const PRIORITIES = ["", "LOW", "NORMAL", "HIGH", "CRITICAL"] as const;

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

const CollectionsPage = () => {
  const navigate = useNavigate();
  const [page, setPage] = useState(0);
  const [stage, setStage] = useState("");
  const [priority, setPriority] = useState("");
  const [dpdMin, setDpdMin] = useState("");
  const [dpdMax, setDpdMax] = useState("");
  const [search, setSearch] = useState("");
  const pageSize = 20;

  const filterParams: Record<string, string> = {};
  if (stage) filterParams.stage = stage;
  if (priority) filterParams.priority = priority;
  if (dpdMin) filterParams.dpdMin = dpdMin;
  if (dpdMax) filterParams.dpdMax = dpdMax;
  if (search) filterParams.search = search;

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ["collections-summary"],
    queryFn: () => collectionsService.getSummary(),
    staleTime: 30_000,
    retry: false,
  });

  const { data: casesPage, isLoading: casesLoading, isError } = useQuery({
    queryKey: ["collections-cases", page, pageSize, filterParams],
    queryFn: () => collectionsService.listCases(page, pageSize, filterParams),
    staleTime: 30_000,
    retry: false,
  });

  const cases = casesPage?.content ?? [];
  const totalPages = casesPage?.totalPages ?? 0;
  const totalElements = casesPage?.totalElements ?? 0;

  return (
    <DashboardLayout
      title="Collections Queue"
      subtitle="Delinquency management and case tracking"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Collections" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* KPI Cards */}
        {summaryLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Total Open</span>
                  <AlertTriangle className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="text-2xl font-heading">{summary?.totalOpenCases ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Watch</span>
                  <Eye className="h-4 w-4 text-yellow-500" />
                </div>
                <p className="text-2xl font-heading text-yellow-600">{summary?.watchCases ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Substandard</span>
                  <ShieldAlert className="h-4 w-4 text-orange-500" />
                </div>
                <p className="text-2xl font-heading text-orange-600">{summary?.substandardCases ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Doubtful</span>
                  <TriangleAlert className="h-4 w-4 text-red-500" />
                </div>
                <p className="text-2xl font-heading text-red-600">{summary?.doubtfulCases ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Loss</span>
                  <XCircle className="h-4 w-4 text-red-700" />
                </div>
                <p className="text-2xl font-heading text-red-800">{summary?.lossCases ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Total Outstanding</span>
                  <Banknote className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="text-xl font-heading">{formatKES(summary?.totalOutstandingAmount ?? 0)}</p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Filter Bar */}
        <Card>
          <CardContent className="p-4">
            <div className="flex flex-wrap items-end gap-3">
              <div className="space-y-1">
                <label className="text-[11px] uppercase tracking-wider text-muted-foreground font-sans">Stage</label>
                <Select value={stage} onValueChange={(v) => { setStage(v === "ALL" ? "" : v); setPage(0); }}>
                  <SelectTrigger className="w-[140px] h-9 text-xs">
                    <SelectValue placeholder="All Stages" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ALL">All Stages</SelectItem>
                    {STAGES.filter(Boolean).map((s) => (
                      <SelectItem key={s} value={s}>{s}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <label className="text-[11px] uppercase tracking-wider text-muted-foreground font-sans">Priority</label>
                <Select value={priority} onValueChange={(v) => { setPriority(v === "ALL" ? "" : v); setPage(0); }}>
                  <SelectTrigger className="w-[140px] h-9 text-xs">
                    <SelectValue placeholder="All Priorities" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ALL">All Priorities</SelectItem>
                    {PRIORITIES.filter(Boolean).map((p) => (
                      <SelectItem key={p} value={p}>{p}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <label className="text-[11px] uppercase tracking-wider text-muted-foreground font-sans">DPD Min</label>
                <Input
                  type="number"
                  placeholder="0"
                  className="w-[80px] h-9 text-xs"
                  value={dpdMin}
                  onChange={(e) => { setDpdMin(e.target.value); setPage(0); }}
                />
              </div>
              <div className="space-y-1">
                <label className="text-[11px] uppercase tracking-wider text-muted-foreground font-sans">DPD Max</label>
                <Input
                  type="number"
                  placeholder="999"
                  className="w-[80px] h-9 text-xs"
                  value={dpdMax}
                  onChange={(e) => { setDpdMax(e.target.value); setPage(0); }}
                />
              </div>
              <div className="space-y-1 flex-1 min-w-[200px]">
                <label className="text-[11px] uppercase tracking-wider text-muted-foreground font-sans">Search</label>
                <div className="relative">
                  <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                  <Input
                    placeholder="Case number, customer ID, loan ID..."
                    className="pl-9 h-9 text-xs"
                    value={search}
                    onChange={(e) => { setSearch(e.target.value); setPage(0); }}
                  />
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Cases Table */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              Collection Cases
              {!casesLoading && (
                <Badge variant="secondary" className="text-[11px]">{totalElements} total</Badge>
              )}
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {casesLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : isError ? (
              <div className="flex items-center justify-center py-16 text-destructive text-sm">
                Failed to load collections data. Ensure collections-service is reachable.
              </div>
            ) : cases.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
                <AlertTriangle className="h-8 w-8 mb-2 text-muted-foreground/50" />
                <p className="text-sm font-medium">No cases found</p>
                <p className="text-xs mt-1">Adjust filters or check if the collections service is running.</p>
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
                    <TableHead className="text-[11px] uppercase tracking-wider font-sans">Last Action</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {cases.map((c) => (
                    <TableRow
                      key={c.id}
                      className="cursor-pointer table-row-hover"
                      onClick={() => navigate(`/collections/case/${c.id}`)}
                    >
                      <TableCell className="text-xs font-mono font-medium">{c.caseNumber}</TableCell>
                      <TableCell className="text-xs font-sans">{c.customerId ?? "---"}</TableCell>
                      <TableCell className="text-xs font-mono">{c.loanId?.slice(0, 8)}</TableCell>
                      <TableCell className="text-xs font-mono text-right font-bold">
                        <span className={c.currentDpd > 90 ? "text-red-700" : c.currentDpd > 30 ? "text-orange-600" : "text-yellow-600"}>
                          {c.currentDpd}
                        </span>
                      </TableCell>
                      <TableCell>{stageBadge(c.currentStage)}</TableCell>
                      <TableCell>{priorityBadge(c.priority)}</TableCell>
                      <TableCell className="text-xs font-mono text-right">{formatKES(c.outstandingAmount)}</TableCell>
                      <TableCell className="text-xs font-sans">{c.assignedTo ?? "Unassigned"}</TableCell>
                      <TableCell className="text-xs font-sans text-muted-foreground">
                        {c.lastActionAt ? new Date(c.lastActionAt).toLocaleDateString() : "---"}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}

            {/* Pagination */}
            {!casesLoading && totalPages > 1 && (
              <div className="flex items-center justify-between px-4 py-3 border-t">
                <p className="text-xs text-muted-foreground">
                  Page {page + 1} of {totalPages} ({totalElements} cases)
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-7 text-xs"
                    disabled={page === 0}
                    onClick={() => setPage((p) => Math.max(0, p - 1))}
                  >
                    <ChevronLeft className="h-3 w-3 mr-1" /> Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-7 text-xs"
                    disabled={page >= totalPages - 1}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    Next <ChevronRight className="h-3 w-3 ml-1" />
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default CollectionsPage;
