import { useMemo } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Brain, BarChart3, TrendingUp, Target, Shield, Activity, Zap, RefreshCw, CheckCircle, XCircle, AlertTriangle } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { scoringService, type ScoringRequest } from "@/services/scoringService";
import { fraudService } from "@/services/fraudService";
import { useToast } from "@/hooks/use-toast";
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Cell,
} from "recharts";

const bandColor: Record<string, string> = {
  A: "bg-success/15 text-success border-success/30",
  B: "bg-info/15 text-info border-info/30",
  C: "bg-warning/15 text-warning border-warning/30",
  D: "bg-destructive/15 text-destructive border-destructive/30",
};

const riskBarColors: Record<string, string> = {
  LOW: "#22c55e",
  MEDIUM: "#f59e0b",
  HIGH: "#f97316",
  CRITICAL: "#ef4444",
};

const AIPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  // Credit scoring data
  const { data: apiPage, isLoading } = useQuery({
    queryKey: ["scoring-requests"],
    queryFn: () => scoringService.listRequests(0, 100),
    staleTime: 60_000,
    retry: false,
  });

  // Fraud ML data
  const { data: scoringStats, isLoading: statsLoading } = useQuery({
    queryKey: ["fraud-scoring-stats"],
    queryFn: () => fraudService.getScoringStats(),
    staleTime: 60_000,
    retry: false,
  });

  const { data: mlHealth } = useQuery({
    queryKey: ["fraud-ml-health"],
    queryFn: () => fraudService.getMLHealth(),
    staleTime: 30_000,
    retry: false,
  });

  const { data: trainingStatus } = useQuery({
    queryKey: ["fraud-training-status"],
    queryFn: () => fraudService.getTrainingStatus(),
    staleTime: 30_000,
    retry: false,
  });

  const { data: fraudAnalytics } = useQuery({
    queryKey: ["fraud-analytics-ml", 30],
    queryFn: () => fraudService.getAnalytics(30),
    staleTime: 60_000,
    retry: false,
  });

  const trainMutation = useMutation({
    mutationFn: (modelType: string) => fraudService.triggerTraining(modelType),
    onSuccess: (data) => {
      toast({ title: "Training triggered", description: data.message });
      queryClient.invalidateQueries({ queryKey: ["fraud-training-status"] });
    },
    onError: (err: Error) => {
      toast({ title: "Training failed", description: err.message, variant: "destructive" });
    },
  });

  const requests: ScoringRequest[] = apiPage?.content ?? [];

  const creditStats = useMemo(() => {
    if (requests.length === 0) return { total: 0, completed: 0, failed: 0, avgScore: 0, bands: {} as Record<string, number> };
    const completed = requests.filter(r => r.status === "COMPLETED").length;
    const failed = requests.filter(r => r.status === "FAILED" || r.errorMessage).length;
    const scored = requests.filter(r => r.score != null);
    const avgScore = scored.length > 0 ? Math.round(scored.reduce((s, r) => s + (r.score ?? 0), 0) / scored.length) : 0;
    const bands: Record<string, number> = {};
    scored.forEach(r => {
      const b = r.scoreBand ?? "?";
      bands[b] = (bands[b] || 0) + 1;
    });
    return { total: requests.length, completed, failed, avgScore, bands };
  }, [requests]);

  const riskDistribution = scoringStats ? [
    { level: "LOW", count: scoringStats.lowCount },
    { level: "MEDIUM", count: scoringStats.mediumCount },
    { level: "HIGH", count: scoringStats.highCount },
    { level: "CRITICAL", count: scoringStats.criticalCount },
  ] : [];

  return (
    <DashboardLayout
      title="AI Intelligence"
      subtitle="ML model performance, fraud scoring, and credit scoring"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "AI Intelligence" }]}
    >
      <div className="space-y-4 animate-fade-in">
        <Tabs defaultValue="fraud-ml" className="w-full">
          <TabsList>
            <TabsTrigger value="fraud-ml" className="text-xs">Fraud ML Scoring</TabsTrigger>
            <TabsTrigger value="credit" className="text-xs">Credit Scoring</TabsTrigger>
          </TabsList>

          {/* Fraud ML Tab */}
          <TabsContent value="fraud-ml">
            <div className="space-y-4">
              {/* ML Health & Model Status */}
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <Card>
                  <CardContent className="p-5">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs text-muted-foreground font-sans">ML Service</span>
                      <Activity className="h-4 w-4 text-accent" />
                    </div>
                    <div className="flex items-center gap-2">
                      {mlHealth?.healthy ? (
                        <CheckCircle className="h-5 w-5 text-success" />
                      ) : (
                        <XCircle className="h-5 w-5 text-destructive" />
                      )}
                      <p className="text-lg font-heading">{mlHealth?.healthy ? "Online" : "Offline"}</p>
                    </div>
                    {mlHealth?.models && (
                      <div className="mt-2 space-y-0.5">
                        {Object.entries(mlHealth.models).map(([k, v]) => (
                          <p key={k} className="text-[11px] text-muted-foreground">
                            {k}: <span className={v === "loaded" ? "text-success" : "text-destructive"}>{v}</span>
                          </p>
                        ))}
                      </div>
                    )}
                  </CardContent>
                </Card>

                <Card>
                  <CardContent className="p-5">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs text-muted-foreground font-sans">Total Scored</span>
                      <Zap className="h-4 w-4 text-info" />
                    </div>
                    <p className="text-2xl font-heading">{scoringStats?.totalScored ?? 0}</p>
                    <p className="text-[11px] text-muted-foreground mt-1">
                      Avg latency: {scoringStats?.avgLatencyMs?.toFixed(0) ?? "—"}ms
                    </p>
                  </CardContent>
                </Card>

                <Card>
                  <CardContent className="p-5">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs text-muted-foreground font-sans">Avg ML Score</span>
                      <TrendingUp className="h-4 w-4 text-warning" />
                    </div>
                    <p className="text-2xl font-heading">{scoringStats?.avgScore?.toFixed(2) ?? "—"}</p>
                    <p className="text-[11px] text-muted-foreground mt-1">
                      0 = clean, 1 = high fraud risk
                    </p>
                  </CardContent>
                </Card>

                <Card>
                  <CardContent className="p-5">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs text-muted-foreground font-sans">High / Critical</span>
                      <AlertTriangle className="h-4 w-4 text-destructive" />
                    </div>
                    <p className="text-2xl font-heading text-destructive">
                      {(scoringStats?.highCount ?? 0) + (scoringStats?.criticalCount ?? 0)}
                    </p>
                    <p className="text-[11px] text-muted-foreground mt-1">
                      {scoringStats?.highCount ?? 0} high, {scoringStats?.criticalCount ?? 0} critical
                    </p>
                  </CardContent>
                </Card>
              </div>

              {/* Risk Distribution Chart + Rule Effectiveness */}
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                <Card>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-xs font-semibold uppercase tracking-wider">Risk Level Distribution</CardTitle>
                  </CardHeader>
                  <CardContent>
                    {riskDistribution.length === 0 || statsLoading ? (
                      <div className="h-48 flex items-center justify-center text-xs text-muted-foreground">
                        No scoring data yet
                      </div>
                    ) : (
                      <ResponsiveContainer width="100%" height={200}>
                        <BarChart data={riskDistribution} layout="vertical">
                          <CartesianGrid strokeDasharray="3 3" horizontal={false} />
                          <XAxis type="number" tick={{ fontSize: 10 }} />
                          <YAxis type="category" dataKey="level" tick={{ fontSize: 10 }} width={60} />
                          <Tooltip contentStyle={{ fontSize: 12 }} />
                          <Bar dataKey="count" radius={[0, 4, 4, 0]}>
                            {riskDistribution.map((entry) => (
                              <Cell key={entry.level} fill={riskBarColors[entry.level] ?? "#888"} />
                            ))}
                          </Bar>
                        </BarChart>
                      </ResponsiveContainer>
                    )}
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-xs font-semibold uppercase tracking-wider">Rule Effectiveness (Precision)</CardTitle>
                  </CardHeader>
                  <CardContent>
                    {!fraudAnalytics?.ruleEffectiveness?.length ? (
                      <div className="h-48 flex items-center justify-center text-xs text-muted-foreground">
                        No rule data yet
                      </div>
                    ) : (
                      <ResponsiveContainer width="100%" height={200}>
                        <BarChart data={fraudAnalytics.ruleEffectiveness.slice(0, 8)} layout="vertical">
                          <CartesianGrid strokeDasharray="3 3" horizontal={false} />
                          <XAxis type="number" domain={[0, 1]} tick={{ fontSize: 10 }} tickFormatter={(v: number) => `${(v * 100).toFixed(0)}%`} />
                          <YAxis type="category" dataKey="ruleCode" tick={{ fontSize: 9 }} width={120} />
                          <Tooltip contentStyle={{ fontSize: 12 }} formatter={(v: number) => `${(v * 100).toFixed(1)}%`} />
                          <Bar dataKey="precisionRate" fill="#6366f1" radius={[0, 4, 4, 0]} />
                        </BarChart>
                      </ResponsiveContainer>
                    )}
                  </CardContent>
                </Card>
              </div>

              {/* Daily Scoring Volume */}
              {scoringStats?.dailyVolume && scoringStats.dailyVolume.length > 0 && (
                <Card>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-xs font-semibold uppercase tracking-wider">Daily Scoring Volume</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <ResponsiveContainer width="100%" height={180}>
                      <BarChart data={scoringStats.dailyVolume}>
                        <CartesianGrid strokeDasharray="3 3" />
                        <XAxis dataKey="date" tick={{ fontSize: 9 }} />
                        <YAxis tick={{ fontSize: 10 }} />
                        <Tooltip contentStyle={{ fontSize: 12 }} />
                        <Bar dataKey="count" fill="#8b5cf6" radius={[4, 4, 0, 0]} />
                      </BarChart>
                    </ResponsiveContainer>
                  </CardContent>
                </Card>
              )}

              {/* Model Training Controls */}
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-xs font-semibold uppercase tracking-wider flex items-center gap-1.5">
                    <RefreshCw className="h-3.5 w-3.5" /> Model Training
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div className="border rounded-lg p-4 space-y-3">
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="text-sm font-medium">Anomaly Detector</p>
                          <p className="text-[11px] text-muted-foreground">Isolation Forest — unsupervised</p>
                        </div>
                        <Badge variant="outline" className="text-[11px]">
                          {(trainingStatus?.anomaly as Record<string, unknown>)?.status as string ?? "idle"}
                        </Badge>
                      </div>
                      <Button size="sm" variant="outline" className="text-xs w-full"
                        disabled={trainMutation.isPending}
                        onClick={() => trainMutation.mutate("anomaly")}>
                        <RefreshCw className="h-3 w-3 mr-1.5" /> Retrain Anomaly Model
                      </Button>
                    </div>

                    <div className="border rounded-lg p-4 space-y-3">
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="text-sm font-medium">Fraud Scorer</p>
                          <p className="text-[11px] text-muted-foreground">LightGBM — supervised</p>
                        </div>
                        <Badge variant="outline" className="text-[11px]">
                          {(trainingStatus?.lgbm as Record<string, unknown>)?.status as string ?? "idle"}
                        </Badge>
                      </div>
                      <Button size="sm" variant="outline" className="text-xs w-full"
                        disabled={trainMutation.isPending}
                        onClick={() => trainMutation.mutate("fraud-scorer")}>
                        <RefreshCw className="h-3 w-3 mr-1.5" /> Retrain Fraud Scorer
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>
          </TabsContent>

          {/* Credit Scoring Tab */}
          <TabsContent value="credit">
            <div className="space-y-4">
              {isLoading ? (
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                  {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-24 w-full" />)}
                </div>
              ) : (
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                  <Card>
                    <CardContent className="p-5">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-xs text-muted-foreground font-sans">Total Requests</span>
                        <Brain className="h-4 w-4 text-accent" />
                      </div>
                      <p className="text-2xl font-heading">{creditStats.total}</p>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardContent className="p-5">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-xs text-muted-foreground font-sans">Completed</span>
                        <BarChart3 className="h-4 w-4 text-success" />
                      </div>
                      <p className="text-2xl font-heading">{creditStats.completed}</p>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardContent className="p-5">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-xs text-muted-foreground font-sans">Failed</span>
                        <Target className="h-4 w-4 text-destructive" />
                      </div>
                      <p className="text-2xl font-heading">{creditStats.failed}</p>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardContent className="p-5">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-xs text-muted-foreground font-sans">Success Rate</span>
                        <TrendingUp className="h-4 w-4 text-info" />
                      </div>
                      <p className="text-2xl font-heading">{creditStats.total > 0 ? Math.round((creditStats.completed / creditStats.total) * 100) : 0}%</p>
                    </CardContent>
                  </Card>
                </div>
              )}

              {!isLoading && Object.keys(creditStats.bands).filter(k => k !== "?").length > 0 && (
                <div className="flex items-center gap-3">
                  <span className="text-xs text-muted-foreground font-sans">Band Distribution:</span>
                  {Object.entries(creditStats.bands).filter(([k]) => k !== "?").sort(([a], [b]) => a.localeCompare(b)).map(([band, count]) => (
                    <Badge key={band} variant="outline" className={`text-xs font-mono ${bandColor[band] ?? ""}`}>
                      {band}: {count}
                    </Badge>
                  ))}
                </div>
              )}

              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium flex items-center gap-2">
                    <Brain className="h-4 w-4" /> Credit Scoring Requests
                  </CardTitle>
                </CardHeader>
                <CardContent className="p-0">
                  {isLoading ? (
                    <div className="p-4 space-y-2">
                      {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
                    </div>
                  ) : requests.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                      <Brain className="h-8 w-8 mb-2 text-muted-foreground/50" />
                      <p className="text-sm font-medium">No scoring requests</p>
                      <p className="text-xs mt-1">Credit scoring requests will appear here when loans are processed.</p>
                    </div>
                  ) : (
                    <Table>
                      <TableHeader>
                        <TableRow className="hover:bg-transparent">
                          <TableHead className="text-[11px] uppercase tracking-wider font-sans">Customer ID</TableHead>
                          <TableHead className="text-[11px] uppercase tracking-wider font-sans">Application</TableHead>
                          <TableHead className="text-[11px] uppercase tracking-wider font-sans">Trigger</TableHead>
                          <TableHead className="text-[11px] uppercase tracking-wider font-sans">Status</TableHead>
                          <TableHead className="text-[11px] uppercase tracking-wider font-sans">Requested</TableHead>
                          <TableHead className="text-[11px] uppercase tracking-wider font-sans">Completed</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {requests.map((req) => (
                          <TableRow key={req.id} className="table-row-hover">
                            <TableCell className="text-xs font-mono font-medium">{req.customerId}</TableCell>
                            <TableCell className="text-xs font-mono text-muted-foreground">{req.loanApplicationId?.slice(0, 8) ?? "—"}</TableCell>
                            <TableCell className="text-xs font-sans text-muted-foreground">{req.triggerEvent ?? "—"}</TableCell>
                            <TableCell>
                              <Badge variant="outline" className={`text-[11px] font-sans ${
                                req.status === "COMPLETED" ? "bg-success/15 text-success border-success/30" :
                                req.status === "FAILED" ? "bg-destructive/15 text-destructive border-destructive/30" :
                                "bg-warning/15 text-warning border-warning/30"
                              }`}>
                                {req.status ?? "PENDING"}
                              </Badge>
                            </TableCell>
                            <TableCell className="text-xs font-sans">{req.requestedAt?.split("T")[0] ?? "—"}</TableCell>
                            <TableCell className="text-xs font-sans">{req.completedAt?.replace("T", " ").slice(0, 19) ?? "—"}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                </CardContent>
              </Card>
            </div>
          </TabsContent>
        </Tabs>
      </div>
    </DashboardLayout>
  );
};

export default AIPage;
