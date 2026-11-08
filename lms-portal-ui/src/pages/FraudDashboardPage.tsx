import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  ShieldAlert, AlertTriangle, Users, TrendingUp, Eye, Shield,
  Activity, ArrowUpRight, Gauge, BarChart3, Brain, RefreshCw,
} from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fraudService, type FraudAlert, type FraudAnalytics, type FraudEvent } from "@/services/fraudService";
import { fraudMLService } from "@/services/fraudMLService";
import { useNavigate } from "react-router-dom";
import { useToast } from "@/hooks/use-toast";
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer,
  PieChart, Pie, Cell, Legend, AreaChart, Area, CartesianGrid,
} from "recharts";

const COLORS = {
  CRITICAL: "#dc2626",
  HIGH: "#ef4444",
  MEDIUM: "#f97316",
  LOW: "#3b82f6",
};

const RULE_LABELS: Record<string, string> = {
  LARGE_SINGLE_TXN: "Large Transaction",
  STRUCTURING: "Structuring",
  HIGH_VELOCITY_1H: "High Velocity (1h)",
  HIGH_VELOCITY_24H: "High Velocity (24h)",
  APPLICATION_STACKING: "App Stacking",
  RAPID_FUND_MOVEMENT: "Rapid Movement",
  LOAN_CYCLING: "Loan Cycling",
  OVERPAYMENT: "Overpayment",
  WATCHLIST_MATCH: "Watchlist Match",
  DORMANT_REACTIVATION: "Dormant Reactivation",
  ROUND_AMOUNT_PATTERN: "Round Amounts",
  PAYMENT_REVERSAL: "Payment Reversal",
  BNPL_ABUSE: "BNPL Abuse",
  OVERDRAFT_RAPID_DRAW: "Overdraft Drawdown",
  KYC_BYPASS_ATTEMPT: "KYC Bypass",
  SUSPICIOUS_WRITEOFF: "Suspicious Write-off",
};

const FraudDashboardPage = () => {
  const navigate = useNavigate();
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ["fraud", "summary"],
    queryFn: () => fraudService.getSummary(),
    staleTime: 30_000,
    retry: false,
  });

  const { data: alertsPage, isLoading: alertsLoading } = useQuery({
    queryKey: ["fraud", "all-alerts"],
    queryFn: () => fraudService.listAlerts(0, 200),
    staleTime: 30_000,
    retry: false,
  });

  const { data: highRiskPage, isLoading: riskLoading } = useQuery({
    queryKey: ["fraud", "high-risk"],
    queryFn: () => fraudService.listHighRiskCustomers(0, 10),
    staleTime: 60_000,
    retry: false,
  });

  const { data: analytics } = useQuery({
    queryKey: ["fraud", "analytics"],
    queryFn: () => fraudService.getAnalytics(30),
    staleTime: 60_000,
    retry: false,
  });

  const { data: recentEventsPage } = useQuery({
    queryKey: ["fraud", "recent-events"],
    queryFn: () => fraudService.getRecentEvents(0, 20),
    staleTime: 15_000,
    retry: false,
    refetchInterval: 30_000,
  });

  const recentEvents: FraudEvent[] = recentEventsPage?.content ?? [];

  // ML Service queries
  const { data: mlHealth } = useQuery({
    queryKey: ["fraud-ml", "health"],
    queryFn: () => fraudMLService.health(),
    staleTime: 30_000,
    retry: false,
  });

  const { data: trainStatus } = useQuery({
    queryKey: ["fraud-ml", "train-status"],
    queryFn: () => fraudMLService.trainingStatus(),
    staleTime: 15_000,
    retry: false,
  });

  const trainAnomalyMutation = useMutation({
    mutationFn: () => fraudMLService.trainAnomaly(),
    onSuccess: () => {
      toast({ title: "Training started", description: "Anomaly detection model training initiated" });
      queryClient.invalidateQueries({ queryKey: ["fraud-ml", "train-status"] });
    },
    onError: (e: Error) => toast({ title: "Training failed", description: e.message, variant: "destructive" }),
  });

  const trainScorerMutation = useMutation({
    mutationFn: () => fraudMLService.trainFraudScorer(),
    onSuccess: () => {
      toast({ title: "Training started", description: "Fraud scorer model training initiated" });
      queryClient.invalidateQueries({ queryKey: ["fraud-ml", "train-status"] });
    },
    onError: (e: Error) => toast({ title: "Training failed", description: e.message, variant: "destructive" }),
  });

  const allAlerts: FraudAlert[] = alertsPage?.content ?? [];
  const highRiskCustomers = highRiskPage?.content ?? [];

  // ─── Chart: Alerts by Rule ─────────────────────────────────────────────────
  const ruleCounts: Record<string, number> = {};
  allAlerts.forEach((a) => {
    const key = a.ruleCode ?? a.alertType ?? "UNKNOWN";
    ruleCounts[key] = (ruleCounts[key] ?? 0) + 1;
  });
  const ruleChartData = Object.entries(ruleCounts)
    .sort(([, a], [, b]) => b - a)
    .slice(0, 10)
    .map(([rule, count]) => ({
      name: RULE_LABELS[rule] ?? rule.replace(/_/g, " "),
      count,
    }));

  // ─── Chart: Severity Pie ──────────────────────────────────────────────────
  const severityCounts = (["CRITICAL", "HIGH", "MEDIUM", "LOW"] as const).map((sev) => ({
    name: sev,
    value: allAlerts.filter((a) => a.severity === sev).length,
  })).filter((d) => d.value > 0);

  // ─── Chart: Alerts Over Time (group by date) ─────────────────────────────
  const dateCounts: Record<string, number> = {};
  allAlerts.forEach((a) => {
    const day = a.createdAt?.split("T")[0];
    if (day) dateCounts[day] = (dateCounts[day] ?? 0) + 1;
  });
  const timelineData = Object.entries(dateCounts)
    .sort(([a], [b]) => a.localeCompare(b))
    .slice(-30)
    .map(([date, count]) => ({ date: date.slice(5), alerts: count }));

  // ─── Chart: Status Distribution ───────────────────────────────────────────
  const statusCounts: Record<string, number> = {};
  allAlerts.forEach((a) => {
    statusCounts[a.status] = (statusCounts[a.status] ?? 0) + 1;
  });
  const statusData = Object.entries(statusCounts).map(([status, count]) => ({
    name: status.replace(/_/g, " "),
    value: count,
  }));

  const STATUS_COLORS = ["#ef4444", "#f97316", "#eab308", "#dc2626", "#22c55e", "#6b7280"];

  const isLoading = summaryLoading || alertsLoading;

  return (
    <DashboardLayout
      title="Fraud & AML Dashboard"
      subtitle="Real-time fraud detection analytics, risk monitoring, and investigation overview"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "Fraud Dashboard" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Status Bar */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Badge className="bg-success/10 text-success border-success/20 gap-1.5 px-3 py-1">
              <span className="h-1.5 w-1.5 rounded-full bg-success inline-block animate-pulse" />
              Detection Active
            </Badge>
            <span className="text-xs text-muted-foreground">
              20 rules | 25 event types | real-time velocity tracking
            </span>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" className="h-8 text-xs" onClick={() => navigate("/fraud")}>
              <Eye className="h-3.5 w-3.5 mr-1" /> View Alerts
            </Button>
            <Button variant="outline" size="sm" className="h-8 text-xs" onClick={() => navigate("/aml")}>
              <Shield className="h-3.5 w-3.5 mr-1" /> AML Monitor
            </Button>
          </div>
        </div>

        {/* KPI Cards */}
        {isLoading ? (
          <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-4">
            {Array.from({ length: 7 }).map((_, i) => <Skeleton key={i} className="h-24 w-full" />)}
          </div>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-3">
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[11px] text-muted-foreground">Total Alerts</span>
                  <Activity className="h-3.5 w-3.5 text-muted-foreground" />
                </div>
                <p className="text-xl font-heading">{allAlerts.length}</p>
              </CardContent>
            </Card>
            <Card className="border-destructive/30">
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[11px] text-muted-foreground">Open</span>
                  <ShieldAlert className="h-3.5 w-3.5 text-destructive" />
                </div>
                <p className="text-xl font-heading text-destructive">{summary?.openAlerts ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[11px] text-muted-foreground">Critical</span>
                  <AlertTriangle className="h-3.5 w-3.5 text-red-600" />
                </div>
                <p className="text-xl font-heading">{summary?.criticalAlerts ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[11px] text-muted-foreground">Reviewing</span>
                  <Gauge className="h-3.5 w-3.5 text-warning" />
                </div>
                <p className="text-xl font-heading">{summary?.underReviewAlerts ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[11px] text-muted-foreground">Escalated</span>
                  <ArrowUpRight className="h-3.5 w-3.5 text-red-500" />
                </div>
                <p className="text-xl font-heading">{summary?.escalatedAlerts ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[11px] text-muted-foreground">Confirmed</span>
                  <AlertTriangle className="h-3.5 w-3.5 text-red-700" />
                </div>
                <p className="text-xl font-heading">{summary?.confirmedFraud ?? 0}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1.5">
                  <span className="text-[11px] text-muted-foreground">High Risk</span>
                  <Users className="h-3.5 w-3.5 text-info" />
                </div>
                <p className="text-xl font-heading">{summary?.highRiskCustomers ?? 0}</p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* ML Models Status */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <Brain className="h-4 w-4" /> ML Models
              </CardTitle>
              <div className="flex gap-2">
                <Button
                  variant="outline" size="sm" className="h-7 text-xs"
                  onClick={() => trainAnomalyMutation.mutate()}
                  disabled={trainAnomalyMutation.isPending || trainStatus?.anomaly?.status === "running"}
                >
                  <RefreshCw className={`h-3 w-3 mr-1 ${trainAnomalyMutation.isPending ? "animate-spin" : ""}`} />
                  Train Anomaly
                </Button>
                <Button
                  variant="outline" size="sm" className="h-7 text-xs"
                  onClick={() => trainScorerMutation.mutate()}
                  disabled={trainScorerMutation.isPending || trainStatus?.lgbm?.status === "running"}
                >
                  <RefreshCw className={`h-3 w-3 mr-1 ${trainScorerMutation.isPending ? "animate-spin" : ""}`} />
                  Train Fraud Scorer
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
              <div className="space-y-1">
                <p className="text-[11px] text-muted-foreground uppercase tracking-wide">Anomaly Detector</p>
                <Badge variant="outline" className={`text-[11px] ${
                  mlHealth?.models?.anomaly_detector === "loaded"
                    ? "bg-success/15 text-success border-success/30"
                    : "bg-muted text-muted-foreground"
                }`}>
                  {mlHealth?.models?.anomaly_detector ?? "unknown"}
                </Badge>
              </div>
              <div className="space-y-1">
                <p className="text-[11px] text-muted-foreground uppercase tracking-wide">Fraud Scorer (LightGBM)</p>
                <Badge variant="outline" className={`text-[11px] ${
                  mlHealth?.models?.fraud_scorer === "loaded"
                    ? "bg-success/15 text-success border-success/30"
                    : "bg-muted text-muted-foreground"
                }`}>
                  {mlHealth?.models?.fraud_scorer ?? "unknown"}
                </Badge>
              </div>
              <div className="space-y-1">
                <p className="text-[11px] text-muted-foreground uppercase tracking-wide">Anomaly Training</p>
                <Badge variant="outline" className={`text-[11px] ${
                  trainStatus?.anomaly?.status === "completed" ? "bg-success/15 text-success border-success/30" :
                  trainStatus?.anomaly?.status === "running" ? "bg-info/15 text-info border-info/30" :
                  trainStatus?.anomaly?.status === "failed" ? "bg-destructive/15 text-destructive border-destructive/30" :
                  "bg-muted text-muted-foreground"
                }`}>
                  {trainStatus?.anomaly?.status ?? "idle"}
                </Badge>
              </div>
              <div className="space-y-1">
                <p className="text-[11px] text-muted-foreground uppercase tracking-wide">Scorer Training</p>
                <Badge variant="outline" className={`text-[11px] ${
                  trainStatus?.lgbm?.status === "completed" ? "bg-success/15 text-success border-success/30" :
                  trainStatus?.lgbm?.status === "running" ? "bg-info/15 text-info border-info/30" :
                  trainStatus?.lgbm?.status === "failed" ? "bg-destructive/15 text-destructive border-destructive/30" :
                  "bg-muted text-muted-foreground"
                }`}>
                  {trainStatus?.lgbm?.status ?? "idle"}
                </Badge>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Investigation & Detection KPIs */}
        {analytics && (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <Card>
              <CardContent className="p-4">
                <p className="text-[11px] text-muted-foreground mb-1">Resolution Rate</p>
                <p className="text-xl font-heading">{(analytics.resolutionRate * 100).toFixed(1)}%</p>
                <p className="text-[11px] text-muted-foreground">{analytics.resolvedAlerts} / {analytics.totalAlerts} resolved</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <p className="text-[11px] text-muted-foreground mb-1">Precision Rate</p>
                <p className="text-xl font-heading">{(analytics.precisionRate * 100).toFixed(1)}%</p>
                <p className="text-[11px] text-muted-foreground">{analytics.confirmedFraudCount} confirmed / {analytics.confirmedFraudCount + analytics.falsePositiveCount} reviewed</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <p className="text-[11px] text-muted-foreground mb-1">Active Cases</p>
                <p className="text-xl font-heading">{analytics.activeCases}</p>
                <p className="text-[11px] text-muted-foreground">Under investigation</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <p className="text-[11px] text-muted-foreground mb-1">False Positives</p>
                <p className="text-xl font-heading text-success">{analytics.falsePositiveCount}</p>
                <p className="text-[11px] text-muted-foreground">Dismissed alerts</p>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Rule Effectiveness */}
        {analytics && analytics.ruleEffectiveness.length > 0 && (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <BarChart3 className="h-4 w-4" /> Rule Effectiveness (Precision per Rule)
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-0">
              <ResponsiveContainer width="100%" height={Math.max(200, analytics.ruleEffectiveness.length * 35)}>
                <BarChart
                  data={analytics.ruleEffectiveness.map((r) => ({
                    name: RULE_LABELS[r.ruleCode] ?? r.ruleCode.replace(/_/g, " "),
                    precision: Math.round(r.precisionRate * 100),
                    triggers: r.totalTriggers,
                    confirmed: r.confirmedFraud,
                    falsePos: r.falsePositives,
                  }))}
                  layout="vertical"
                  margin={{ left: 10 }}
                >
                  <XAxis type="number" domain={[0, 100]} tick={{ fontSize: 10 }} unit="%" />
                  <YAxis type="category" dataKey="name" tick={{ fontSize: 10 }} width={130} />
                  <Tooltip
                    contentStyle={{ fontSize: 12 }}
                    formatter={(value: number, name: string) => {
                      if (name === "precision") return [`${value}%`, "Precision"];
                      return [value, name];
                    }}
                  />
                  <Bar dataKey="precision" fill="hsl(var(--chart-1))" radius={[0, 4, 4, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        )}

        {/* Charts Row 1: Timeline + Severity */}
        {allAlerts.length > 0 && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            <Card className="lg:col-span-2">
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <TrendingUp className="h-4 w-4" /> Alert Trend (Last 30 Days)
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                <ResponsiveContainer width="100%" height={200}>
                  <AreaChart data={timelineData}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                    <XAxis dataKey="date" tick={{ fontSize: 10 }} />
                    <YAxis tick={{ fontSize: 10 }} />
                    <Tooltip contentStyle={{ fontSize: 12 }} />
                    <Area type="monotone" dataKey="alerts" stroke="hsl(var(--destructive))" fill="hsl(var(--destructive)/0.15)" />
                  </AreaChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <Shield className="h-4 w-4" /> By Severity
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                <ResponsiveContainer width="100%" height={200}>
                  <PieChart>
                    <Pie data={severityCounts} cx="50%" cy="50%" innerRadius={40} outerRadius={70} dataKey="value">
                      {severityCounts.map((entry) => (
                        <Cell key={entry.name} fill={COLORS[entry.name as keyof typeof COLORS] ?? "#6b7280"} />
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

        {/* Charts Row 2: Rules + Status */}
        {allAlerts.length > 0 && (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <BarChart3 className="h-4 w-4" /> Top Rules Triggered
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                <ResponsiveContainer width="100%" height={250}>
                  <BarChart data={ruleChartData} layout="vertical" margin={{ left: 10 }}>
                    <XAxis type="number" tick={{ fontSize: 10 }} />
                    <YAxis type="category" dataKey="name" tick={{ fontSize: 10 }} width={130} />
                    <Tooltip contentStyle={{ fontSize: 12 }} />
                    <Bar dataKey="count" fill="hsl(var(--warning))" radius={[0, 4, 4, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <Gauge className="h-4 w-4" /> Alert Status Distribution
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                <ResponsiveContainer width="100%" height={250}>
                  <PieChart>
                    <Pie data={statusData} cx="50%" cy="50%" outerRadius={80} dataKey="value"
                      label={({ name, value }) => `${name}: ${value}`} labelLine={false}>
                      {statusData.map((_, i) => (
                        <Cell key={i} fill={STATUS_COLORS[i % STATUS_COLORS.length]} />
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

        {/* High Risk Customers */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <CardTitle className="text-sm font-medium">High Risk Customers</CardTitle>
              <Badge variant="outline" className="text-[11px]">
                {highRiskPage?.totalElements ?? 0} total
              </Badge>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            {riskLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : highRiskCustomers.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                <Users className="h-8 w-8 mb-2 opacity-30" />
                <p className="text-sm font-medium">No high-risk customers identified</p>
                <p className="text-xs mt-1">Risk profiles are built as fraud alerts are processed</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Customer ID</TableHead>
                    <TableHead className="text-xs">Risk Level</TableHead>
                    <TableHead className="text-xs">Risk Score</TableHead>
                    <TableHead className="text-xs">Total Alerts</TableHead>
                    <TableHead className="text-xs">Open</TableHead>
                    <TableHead className="text-xs">Confirmed Fraud</TableHead>
                    <TableHead className="text-xs">False Positives</TableHead>
                    <TableHead className="text-xs">Last Alert</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {highRiskCustomers.map((c) => (
                    <TableRow key={c.customerId} className="table-row-hover cursor-pointer"
                      onClick={() => navigate(`/borrowers/${c.customerId}`)}>
                      <TableCell className="text-xs font-mono">{c.customerId}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className={`text-[11px] ${
                          c.riskLevel === "CRITICAL" ? "bg-red-500/15 text-red-600 border-red-500/30" :
                          c.riskLevel === "HIGH" ? "bg-destructive/15 text-destructive border-destructive/30" :
                          "bg-warning/15 text-warning border-warning/30"
                        }`}>
                          {c.riskLevel}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-xs">
                        <div className="flex items-center gap-2">
                          <div className="w-16 h-1.5 bg-muted rounded-full overflow-hidden">
                            <div
                              className={`h-full rounded-full ${
                                c.riskScore >= 0.7 ? "bg-red-500" :
                                c.riskScore >= 0.5 ? "bg-orange-500" :
                                "bg-yellow-500"
                              }`}
                              style={{ width: `${Math.min(100, c.riskScore * 100)}%` }}
                            />
                          </div>
                          <span className="text-muted-foreground">{(c.riskScore * 100).toFixed(0)}%</span>
                        </div>
                      </TableCell>
                      <TableCell className="text-xs">{c.totalAlerts}</TableCell>
                      <TableCell className="text-xs">{c.openAlerts}</TableCell>
                      <TableCell className="text-xs font-medium text-destructive">{c.confirmedFraud}</TableCell>
                      <TableCell className="text-xs text-success">{c.falsePositives}</TableCell>
                      <TableCell className="text-xs whitespace-nowrap">
                        {c.lastAlertAt?.split("T")[0] ?? "—"}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Live Transaction Feed */}
        {recentEvents.length > 0 && (
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-medium flex items-center gap-2">
                  <Activity className="h-4 w-4" />
                  Live Transaction Feed
                  <span className="h-1.5 w-1.5 rounded-full bg-success animate-pulse inline-block" />
                </CardTitle>
                <Badge variant="outline" className="text-[11px]">
                  Last {recentEvents.length} events
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Time</TableHead>
                    <TableHead className="text-xs">Event</TableHead>
                    <TableHead className="text-xs">Customer</TableHead>
                    <TableHead className="text-xs text-right">Amount</TableHead>
                    <TableHead className="text-xs">Risk Score</TableHead>
                    <TableHead className="text-xs">Rules Triggered</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {recentEvents.slice(0, 15).map((ev) => (
                    <TableRow key={ev.id} className="table-row-hover">
                      <TableCell className="text-xs text-muted-foreground whitespace-nowrap">
                        {ev.createdAt ? new Date(ev.createdAt).toLocaleTimeString() : "—"}
                      </TableCell>
                      <TableCell className="text-xs font-mono">{ev.eventType?.replace(/\./g, " ")}</TableCell>
                      <TableCell className="text-xs font-mono">
                        {ev.customerId ? (
                          <span className="text-info cursor-pointer" onClick={() => navigate(`/customer/${ev.customerId}`)}>
                            {ev.customerId}
                          </span>
                        ) : "—"}
                      </TableCell>
                      <TableCell className="text-xs font-mono text-right">
                        {ev.amount != null ? Number(ev.amount).toLocaleString() : "—"}
                      </TableCell>
                      <TableCell className="text-xs">
                        {ev.riskScore != null ? (
                          <Badge variant="outline" className={`text-[11px] ${
                            ev.riskScore >= 0.7 ? "bg-destructive/15 text-destructive border-destructive/30" :
                            ev.riskScore >= 0.3 ? "bg-warning/15 text-warning border-warning/30" :
                            "bg-muted text-muted-foreground"
                          }`}>
                            {(ev.riskScore * 100).toFixed(0)}%
                          </Badge>
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="text-xs">
                        {ev.rulesTriggered ? (
                          <span className="text-destructive font-medium">{ev.rulesTriggered.split(",").length} rules</span>
                        ) : (
                          <span className="text-success">clean</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}

        {/* Recent Critical Alerts */}
        {allAlerts.filter((a) => a.severity === "CRITICAL" || a.severity === "HIGH").length > 0 && (
          <Card className="border-destructive/20">
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium flex items-center gap-2 text-destructive">
                <AlertTriangle className="h-4 w-4" /> Recent Critical & High Severity Alerts
              </CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">ID</TableHead>
                    <TableHead className="text-xs">Type</TableHead>
                    <TableHead className="text-xs">Customer</TableHead>
                    <TableHead className="text-xs">Severity</TableHead>
                    <TableHead className="text-xs">Description</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Date</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {allAlerts
                    .filter((a) => a.severity === "CRITICAL" || a.severity === "HIGH")
                    .slice(0, 10)
                    .map((a) => (
                      <TableRow key={a.id} className="table-row-hover cursor-pointer" onClick={() => navigate("/fraud")}>
                        <TableCell className="text-xs font-mono">{a.id?.slice(0, 8)}</TableCell>
                        <TableCell className="text-xs">{a.alertType?.replace(/_/g, " ")}</TableCell>
                        <TableCell className="text-xs font-mono">{a.customerId ?? "—"}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[11px] ${
                            a.severity === "CRITICAL" ? "bg-red-500/15 text-red-600 border-red-500/30" :
                            "bg-destructive/15 text-destructive border-destructive/30"
                          }`}>
                            {a.severity}
                          </Badge>
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
            </CardContent>
          </Card>
        )}
      </div>
    </DashboardLayout>
  );
};

export default FraudDashboardPage;
