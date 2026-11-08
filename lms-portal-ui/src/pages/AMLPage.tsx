import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  Shield, AlertTriangle, Activity, ArrowUpRight,
  TrendingUp, Users, Eye,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { fraudService, type FraudAlert } from "@/services/fraudService";
import { complianceService } from "@/services/complianceService";
import { formatKES } from "@/lib/format";
import { useNavigate } from "react-router-dom";
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer,
  PieChart, Pie, Cell, Legend,
} from "recharts";

const AML_TYPES = [
  "STRUCTURING", "RAPID_FUND_MOVEMENT", "LOAN_CYCLING",
  "LARGE_TRANSACTION", "OVERPAYMENT", "WATCHLIST_MATCH",
];

const PIE_COLORS = ["#ef4444", "#f97316", "#eab308", "#22c55e", "#3b82f6", "#8b5cf6"];

const AMLPage = () => {
  const navigate = useNavigate();
  const [severityFilter, setSeverityFilter] = useState("all");

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ["fraud", "summary"],
    queryFn: () => fraudService.getSummary(),
    staleTime: 30_000,
    retry: false,
  });

  const { data: alertsPage, isLoading: alertsLoading } = useQuery({
    queryKey: ["fraud", "aml-alerts"],
    queryFn: () => fraudService.listAlerts(0, 100),
    staleTime: 30_000,
    retry: false,
  });

  const { data: compliancePage } = useQuery({
    queryKey: ["compliance", "alerts-count"],
    queryFn: () => complianceService.listAlerts(0, 1),
    staleTime: 60_000,
    retry: false,
  });

  const allAlerts = alertsPage?.content ?? [];

  // Filter to AML-relevant alert types
  const amlAlerts = allAlerts.filter((a) => AML_TYPES.includes(a.alertType));
  const displayAlerts = amlAlerts.length > 0 ? amlAlerts : allAlerts;

  const filteredAlerts = severityFilter === "all"
    ? displayAlerts
    : displayAlerts.filter((a) => a.severity === severityFilter);

  // Chart data: alerts by type
  const alertsByType = AML_TYPES.map((type) => ({
    name: type.replace(/_/g, " "),
    count: amlAlerts.filter((a) => a.alertType === type).length,
  })).filter((d) => d.count > 0);

  // Chart data: alerts by severity
  const alertsBySeverity = ["CRITICAL", "HIGH", "MEDIUM", "LOW"].map((sev) => ({
    name: sev,
    value: amlAlerts.filter((a) => a.severity === sev).length,
  })).filter((d) => d.value > 0);

  // Escalated to compliance
  const escalatedCount = amlAlerts.filter((a) => a.escalatedToCompliance).length;

  return (
    <DashboardLayout
      title="AML Monitoring"
      subtitle="Anti-money laundering detection, investigation, and reporting"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "AML Monitoring" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Status bar */}
        <div className="flex items-center gap-3">
          <Badge className="bg-success/10 text-success border-success/20 gap-1.5 px-3 py-1">
            <span className="h-1.5 w-1.5 rounded-full bg-success inline-block animate-pulse" />
            AML Engine Active
          </Badge>
          <span className="text-xs text-muted-foreground">
            Real-time screening: structuring, rapid fund movement, loan cycling, watchlist matching
          </span>
        </div>

        {/* KPI Cards */}
        {summaryLoading ? (
          <div className="grid grid-cols-2 sm:grid-cols-5 gap-4">
            {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-24 w-full" />)}
          </div>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-5 gap-4">
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">AML Alerts</span>
                  <Shield className="h-4 w-4 text-warning" />
                </div>
                <p className="text-2xl font-heading">{amlAlerts.length}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Open</span>
                  <AlertTriangle className="h-4 w-4 text-destructive" />
                </div>
                <p className="text-2xl font-heading">
                  {amlAlerts.filter((a) => a.status === "OPEN").length}
                </p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Escalated</span>
                  <ArrowUpRight className="h-4 w-4 text-red-500" />
                </div>
                <p className="text-2xl font-heading">{escalatedCount}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">High Risk Customers</span>
                  <Users className="h-4 w-4 text-info" />
                </div>
                <p className="text-2xl font-heading">{summary?.highRiskCustomers ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground font-sans">Compliance Events</span>
                  <Activity className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="text-2xl font-heading">{compliancePage?.totalElements ?? 0}</p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Charts Row */}
        {amlAlerts.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <TrendingUp className="h-4 w-4" /> Alerts by Type
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                <ResponsiveContainer width="100%" height={220}>
                  <BarChart data={alertsByType} layout="vertical" margin={{ left: 10 }}>
                    <XAxis type="number" tick={{ fontSize: 10 }} />
                    <YAxis type="category" dataKey="name" tick={{ fontSize: 10 }} width={120} />
                    <Tooltip contentStyle={{ fontSize: 12 }} />
                    <Bar dataKey="count" fill="hsl(var(--warning))" radius={[0, 4, 4, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <Shield className="h-4 w-4" /> Severity Distribution
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-0 flex items-center justify-center">
                <ResponsiveContainer width="100%" height={220}>
                  <PieChart>
                    <Pie
                      data={alertsBySeverity}
                      cx="50%" cy="50%"
                      innerRadius={50} outerRadius={80}
                      dataKey="value"
                      label={({ name, value }) => `${name}: ${value}`}
                      labelLine={false}
                    >
                      {alertsBySeverity.map((_, i) => (
                        <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                      ))}
                    </Pie>
                    <Legend verticalAlign="bottom" iconSize={8} wrapperStyle={{ fontSize: 10 }} />
                    <Tooltip contentStyle={{ fontSize: 12 }} />
                  </PieChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Alerts Table */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium">AML Alerts</CardTitle>
              <div className="flex gap-2">
                <Select value={severityFilter} onValueChange={setSeverityFilter}>
                  <SelectTrigger className="h-8 w-[130px] text-xs">
                    <SelectValue placeholder="All Severity" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Severity</SelectItem>
                    <SelectItem value="CRITICAL">Critical</SelectItem>
                    <SelectItem value="HIGH">High</SelectItem>
                    <SelectItem value="MEDIUM">Medium</SelectItem>
                    <SelectItem value="LOW">Low</SelectItem>
                  </SelectContent>
                </Select>
                <Button variant="outline" size="sm" className="h-8 text-xs" onClick={() => navigate("/fraud")}>
                  <Eye className="h-3.5 w-3.5 mr-1" /> All Alerts
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            {alertsLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : filteredAlerts.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-40 text-muted-foreground">
                <Shield className="h-10 w-10 mb-3 opacity-30" />
                <p className="text-sm font-medium">No AML alerts detected</p>
                <p className="text-xs mt-1">
                  Monitoring structuring, rapid fund movement, loan cycling, and watchlist matches
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">ID</TableHead>
                    <TableHead className="text-xs">Type</TableHead>
                    <TableHead className="text-xs">Customer</TableHead>
                    <TableHead className="text-xs">Severity</TableHead>
                    <TableHead className="text-xs">Amount</TableHead>
                    <TableHead className="text-xs max-w-[300px]">Description</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Date</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredAlerts.slice(0, 50).map((a) => (
                    <TableRow key={a.id} className="table-row-hover cursor-pointer" onClick={() => navigate("/fraud")}>
                      <TableCell className="text-xs font-mono">{a.id?.slice(0, 8)}</TableCell>
                      <TableCell className="text-xs">
                        {a.alertType?.replace(/_/g, " ")}
                        {a.escalatedToCompliance && (
                          <ArrowUpRight className="h-3 w-3 text-red-500 inline ml-1" />
                        )}
                      </TableCell>
                      <TableCell className="text-xs font-mono">{a.customerId ?? "—"}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`text-[11px] ${
                          a.severity === "CRITICAL" ? "bg-red-500/15 text-red-600 border-red-500/30" :
                          a.severity === "HIGH" ? "bg-destructive/15 text-destructive border-destructive/30" :
                          a.severity === "MEDIUM" ? "bg-warning/15 text-warning border-warning/30" :
                          "bg-info/15 text-info border-info/30"
                        }`}>
                          {a.severity}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs">
                        {a.triggerAmount ? formatKES(a.triggerAmount) : "—"}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground max-w-[300px] truncate">
                        {a.description}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-[11px]">{a.status?.replace(/_/g, " ")}</Badge>
                      </TableCell>
                      <TableCell className="text-xs whitespace-nowrap">{a.createdAt?.split("T")[0] ?? "—"}</TableCell>
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

export default AMLPage;
