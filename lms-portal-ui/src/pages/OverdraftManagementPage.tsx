import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { CreditCard, TrendingDown, Users, AlertCircle, Loader2 } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { overdraftService, type OverdraftFacility, type OverdraftSummary } from "@/services/overdraftService";
import { toast } from "sonner";

const bandBadgeStyle: Record<string, string> = {
  A: "bg-success/10 text-success border-success/20",
  B: "bg-blue-500/10 text-blue-600 border-blue-500/20",
  C: "bg-warning/10 text-warning border-warning/20",
  D: "bg-destructive/10 text-destructive border-destructive/20",
};

const statusStyle: Record<string, string> = {
  ACTIVE: "bg-success/10 text-success border-success/20",
  SUSPENDED: "bg-destructive/10 text-destructive border-destructive/20",
  CLOSED: "bg-muted text-muted-foreground border-border",
};

// Coerce possibly-undefined/null/NaN numeric values to a finite number (0 fallback).
const num = (v: unknown): number => {
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
};

const OverdraftManagementPage = () => {
  const queryClient = useQueryClient();

  const { data: summary, isLoading: summaryLoading } = useQuery<OverdraftSummary>({
    queryKey: ["overdraft-summary"],
    queryFn: () => overdraftService.getSummary(),
  });

  const { data: wallets = [], isLoading: walletsLoading } = useQuery({
    queryKey: ["wallets"],
    queryFn: () => overdraftService.listWallets(),
  });

  // Fetch facilities for all wallets (parallel)
  const facilitiesQuery = useQuery<OverdraftFacility[]>({
    queryKey: ["overdraft-facilities", wallets.map((w) => w.id)],
    queryFn: async () => {
      const results = await Promise.allSettled(
        wallets.map((w) => overdraftService.getFacility(w.id))
      );
      return results
        .filter((r): r is PromiseFulfilledResult<OverdraftFacility> => r.status === "fulfilled")
        .map((r) => r.value);
    },
    enabled: wallets.length > 0,
  });

  const suspendMutation = useMutation({
    mutationFn: (walletId: string) => overdraftService.suspendFacility(walletId),
    onSuccess: () => {
      toast.success("Facility suspended");
      queryClient.invalidateQueries({ queryKey: ["overdraft-facilities"] });
      queryClient.invalidateQueries({ queryKey: ["overdraft-summary"] });
    },
    onError: (err: Error) => toast.error(`Failed: ${err.message}`),
  });

  const facilities = facilitiesQuery.data ?? [];
  const isLoading = summaryLoading || walletsLoading;

  return (
    <DashboardLayout
      title="Overdraft Management"
      subtitle="AI credit-scored customer overdraft facilities"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Float & Wallet" }, { label: "Overdraft Management" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Summary KPIs */}
        {isLoading ? (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <Loader2 className="h-5 w-5 animate-spin mr-2" />
            <span className="text-sm">Loading summary…</span>
          </div>
        ) : summary ? (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground">Total Facilities</span>
                  <Users className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="text-2xl font-heading">{summary.totalFacilities}</p>
                <p className="text-xs text-muted-foreground mt-1">{summary.activeFacilities} active</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground">Total Approved Limit</span>
                  <CreditCard className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="text-2xl font-heading">KES {num(summary.totalApprovedLimit).toLocaleString()}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground">Total Drawn</span>
                  <TrendingDown className="h-4 w-4 text-destructive" />
                </div>
                <p className="text-2xl font-heading text-destructive">
                  KES {num(summary.totalDrawnAmount).toLocaleString()}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground">Available Headroom</span>
                  <AlertCircle className="h-4 w-4 text-success" />
                </div>
                <p className="text-2xl font-heading text-success">
                  KES {num(summary.totalAvailableOverdraft).toLocaleString()}
                </p>
              </CardContent>
            </Card>
          </div>
        ) : null}

        {/* Band breakdown */}
        {summary && (
          <div className="grid grid-cols-4 gap-3">
            {["A", "B", "C", "D"].map((band) => (
              <Card key={band}>
                <CardContent className="p-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs text-muted-foreground">Band {band}</span>
                    <Badge variant="outline" className={`text-[10px] ${bandBadgeStyle[band]}`}>
                      {band}
                    </Badge>
                  </div>
                  <p className="text-xl font-heading">{summary.facilitiesByBand?.[band] ?? 0}</p>
                  <p className="text-xs text-muted-foreground mt-1">
                    KES {num(summary.drawnByBand?.[band]).toLocaleString()} drawn
                  </p>
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        {/* Facilities Table */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">Overdraft Facilities</CardTitle>
          </CardHeader>
          <CardContent>
            {facilitiesQuery.isLoading ? (
              <div className="flex items-center justify-center py-8 text-muted-foreground">
                <Loader2 className="h-5 w-5 animate-spin mr-2" />
                <span className="text-sm">Loading facilities…</span>
              </div>
            ) : facilities.length === 0 ? (
              <p className="text-center text-sm text-muted-foreground py-8">
                No overdraft facilities found. Apply from the Wallets page.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Customer</TableHead>
                    <TableHead className="text-xs">Band</TableHead>
                    <TableHead className="text-xs text-right">Limit (KES)</TableHead>
                    <TableHead className="text-xs text-right">Drawn (KES)</TableHead>
                    <TableHead className="text-xs text-right">Available (KES)</TableHead>
                    <TableHead className="text-xs text-right">Rate (p.a.)</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {facilities.map((f) => (
                    <TableRow key={f.id} className="table-row-hover">
                      <TableCell className="text-xs font-medium">{f.customerId}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`text-[10px] ${bandBadgeStyle[f.creditBand] || ""}`}>
                          {f.creditBand} ({f.creditScore})
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs text-right font-mono">
                        {num(f.approvedLimit).toLocaleString()}
                      </TableCell>
                      <TableCell className={`text-xs text-right font-mono ${num(f.drawnAmount) > 0 ? "text-destructive" : ""}`}>
                        {num(f.drawnAmount).toLocaleString()}
                      </TableCell>
                      <TableCell className="text-xs text-right font-mono text-success">
                        {num(f.availableOverdraft).toLocaleString()}
                      </TableCell>
                      <TableCell className="text-xs text-right">
                        {(num(f.interestRate) * 100).toFixed(0)}%
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`text-[10px] ${statusStyle[f.status] || ""}`}>
                          {f.status}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {f.status === "ACTIVE" && (
                          <Button
                            size="sm"
                            variant="outline"
                            className="text-xs h-7 px-2 text-destructive border-destructive/30 hover:bg-destructive/10"
                            disabled={suspendMutation.isPending}
                            onClick={() => suspendMutation.mutate(f.walletId)}
                          >
                            Suspend
                          </Button>
                        )}
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

export default OverdraftManagementPage;
