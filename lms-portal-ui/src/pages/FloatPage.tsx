import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  CreditCard,
  TrendingUp,
  Users,
  AlertTriangle,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { floatService, type FloatAccount } from "@/services/floatService";

const statusColor: Record<string, string> = {
  Active: "bg-success/10 text-success border-success/20",
  ACTIVE: "bg-success/10 text-success border-success/20",
  Maxed: "bg-warning/10 text-warning border-warning/20",
  MAXED: "bg-warning/10 text-warning border-warning/20",
  Dormant: "bg-muted text-muted-foreground border-border",
  DORMANT: "bg-muted text-muted-foreground border-border",
  "Near Limit": "bg-warning/10 text-warning border-warning/20",
  Overdue: "bg-destructive/10 text-destructive border-destructive/20",
  OVERDUE: "bg-destructive/10 text-destructive border-destructive/20",
};

function utilisationPct(acc: FloatAccount): number {
  if (!acc.floatLimit || acc.floatLimit === 0) return 0;
  return Math.round((acc.drawnAmount / acc.floatLimit) * 100);
}

const FloatPage = () => {
  const { data: apiAccounts, isLoading } = useQuery({
    queryKey: ["float", "accounts"],
    queryFn: () => floatService.listFloatAccounts(),
    staleTime: 60_000,
    retry: false,
  });

  const floatAccounts: FloatAccount[] = apiAccounts ?? [];

  // Compute summary from real data
  const totalLimit = floatAccounts.reduce((s, a) => s + (a.floatLimit ?? 0), 0);
  const totalDrawn = floatAccounts.reduce((s, a) => s + (a.drawnAmount ?? 0), 0);
  const totalAvailable = floatAccounts.reduce(
    (s, a) => s + (a.available ?? 0),
    0
  );
  const activeCount = floatAccounts.filter(
    (a) => a.status === "ACTIVE" || a.status === "Active"
  ).length;

  const summaryCards = [
    {
      label: "Total Float Limit",
      value: totalLimit.toLocaleString(),
      icon: CreditCard,
    },
    {
      label: "Active Accounts",
      value: activeCount.toString(),
      icon: Users,
    },
    {
      label: "Total Drawn",
      value: totalDrawn.toLocaleString(),
      icon: TrendingUp,
    },
    {
      label: "Available",
      value: totalAvailable.toLocaleString(),
      icon: AlertTriangle,
    },
  ];

  return (
    <DashboardLayout
      title="AthenaFloat Overview"
      subtitle="Fuliza-style overdraft management"
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Float & Wallet" },
        { label: "AthenaFloat Overview" },
      ]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Summary Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {isLoading
            ? Array.from({ length: 4 }).map((_, i) => (
                <Card key={i}>
                  <CardContent className="p-5">
                    <Skeleton className="h-4 w-2/3 mb-3" />
                    <Skeleton className="h-7 w-1/2" />
                  </CardContent>
                </Card>
              ))
            : summaryCards.map((s) => (
                <Card key={s.label}>
                  <CardContent className="p-5">
                    <div className="flex items-center justify-between mb-3">
                      <span className="text-xs text-muted-foreground font-sans">
                        {s.label}
                      </span>
                      <s.icon className="h-4 w-4 text-muted-foreground" />
                    </div>
                    <p className="text-2xl font-heading">{s.value}</p>
                  </CardContent>
                </Card>
              ))}
        </div>

        {/* Float Accounts Table */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium">
                Float Accounts
              </CardTitle>
              <Button size="sm" variant="outline" className="text-xs">
                Export
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 5 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : floatAccounts.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">No float accounts</p>
                <p className="text-xs mt-1">No float account records returned from the backend.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Float ID</TableHead>
                    <TableHead className="text-xs">Account / Customer</TableHead>
                    <TableHead className="text-xs text-right">Limit</TableHead>
                    <TableHead className="text-xs text-right">Drawn</TableHead>
                    <TableHead className="text-xs">Utilisation</TableHead>
                    <TableHead className="text-xs">Available</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {floatAccounts.map((a) => {
                    const util = utilisationPct(a);
                    return (
                      <TableRow key={a.id} className="table-row-hover">
                        <TableCell className="text-xs font-mono">
                          {a.accountCode}
                        </TableCell>
                        <TableCell className="text-sm font-medium">
                          {a.accountName}
                        </TableCell>
                        <TableCell className="text-xs text-right font-mono">
                          {a.floatLimit.toLocaleString()}
                        </TableCell>
                        <TableCell className="text-xs text-right font-mono">
                          {a.drawnAmount.toLocaleString()}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <div className="w-16 h-1.5 rounded-full bg-muted overflow-hidden">
                              <div
                                className={`h-full rounded-full ${
                                  util > 90
                                    ? "bg-destructive"
                                    : util > 70
                                    ? "bg-warning"
                                    : "bg-success"
                                }`}
                                style={{ width: `${util}%` }}
                              />
                            </div>
                            <span className="text-xs text-muted-foreground font-mono">
                              {util}%
                            </span>
                          </div>
                        </TableCell>
                        <TableCell className="text-xs font-mono">
                          {a.available.toLocaleString()}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant="outline"
                            className={`text-[11px] ${
                              statusColor[a.status] ?? ""
                            }`}
                          >
                            {a.status}
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
      </div>
    </DashboardLayout>
  );
};

export default FloatPage;
