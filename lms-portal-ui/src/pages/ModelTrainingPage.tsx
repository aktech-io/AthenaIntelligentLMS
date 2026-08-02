import { Fragment, useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  AlertTriangle, Check, ChevronDown, ChevronRight, FlaskConical, ShieldCheck, X,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import {
  CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from "recharts";
import { mlService, type MLRun } from "@/services/mlService";

// ─── formatting & lookups ────────────────────────────────────────────────────

const RUN_STATUS: Record<string, { label: string; color: string }> = {
  FINISHED: { label: "Finished", color: "bg-success/15 text-success border-success/30" },
  RUNNING: { label: "Running", color: "bg-primary/10 text-primary border-primary/30" },
  SCHEDULED: { label: "Scheduled", color: "bg-muted/50 text-muted-foreground" },
  FAILED: { label: "Failed", color: "bg-destructive/15 text-destructive border-destructive/30" },
  KILLED: { label: "Killed", color: "bg-destructive/15 text-destructive border-destructive/30" },
};

// Metric curves offered on the expandable training row (per-epoch logging
// from the liveness-training pipeline).
const TRAINING_CHART_METRICS = ["loss", "val_apcer_max", "val_bpcer"];

const fmtMetric = (v: number | undefined): string => {
  if (v == null || Number.isNaN(v)) return "—";
  if (Math.abs(v) >= 100) return v.toFixed(0);
  if (Math.abs(v) >= 1) return v.toFixed(2);
  return v.toPrecision(3);
};

const fmtWhen = (ms?: number): string => (ms ? new Date(ms).toLocaleString() : "—");

// Red-team metric keys are owned by the liveness-redteam rig; resolve
// tolerantly — explicit "worst" keys first, else max over apcer_<species>.
const worstApcer = (m: Record<string, number>): number | undefined => {
  for (const k of ["apcer_worst", "worst_apcer", "worst_species_apcer", "apcer_max"]) {
    if (m[k] != null) return m[k];
  }
  const perSpecies = Object.entries(m)
    .filter(([k]) => k.startsWith("apcer_"))
    .map(([, v]) => v);
  return perSpecies.length ? Math.max(...perSpecies) : undefined;
};

const bpcerOf = (m: Record<string, number>): number | undefined => m["bpcer"] ?? m["val_bpcer"];

// ─── small pieces ────────────────────────────────────────────────────────────

const RunStatusChip = ({ status }: { status: string }) => {
  const cfg = RUN_STATUS[status] ?? { label: status, color: "bg-muted/50 text-muted-foreground" };
  return <Badge variant="outline" className={`text-[10px] ${cfg.color}`}>{cfg.label}</Badge>;
};

// Gate verdicts carry an icon + label, never color alone.
const GateChip = ({ verdict }: { verdict?: string }) => {
  if (!verdict) return <span className="text-xs text-muted-foreground">—</span>;
  const pass = verdict.toLowerCase() === "pass";
  return (
    <Badge
      variant="outline"
      className={`text-[10px] gap-0.5 ${
        pass
          ? "bg-success/15 text-success border-success/30"
          : "bg-destructive/15 text-destructive border-destructive/30"
      }`}
    >
      {pass ? <Check className="h-2.5 w-2.5" /> : <X className="h-2.5 w-2.5" />}
      {pass ? "Pass" : "Fail"}
    </Badge>
  );
};

const EmptyRuns = ({ subtitle }: { subtitle: string }) => (
  <div className="flex flex-col items-center justify-center h-40 text-muted-foreground">
    <FlaskConical className="h-8 w-8 mb-2 text-muted-foreground/50" />
    <p className="text-sm font-medium">No runs yet</p>
    <p className="text-xs mt-1 text-center max-w-md">{subtitle}</p>
  </div>
);

const UpstreamError = ({ message }: { message: string }) => (
  <div className="flex items-center gap-2 p-4 text-warning">
    <AlertTriangle className="h-4 w-4 shrink-0" />
    <p className="text-xs">{message}</p>
  </div>
);

// Single-series metric curve (x = training step/epoch). One series → no
// legend; the metric selector above the chart names it.
const MetricHistoryChart = ({ runId, metric }: { runId: string; metric: string }) => {
  const { data, isLoading, error } = useQuery({
    queryKey: ["ml", "metric-history", runId, metric],
    queryFn: () => mlService.getMetricHistory(runId, metric),
    staleTime: 30_000,
    retry: false,
  });

  if (isLoading) return <Skeleton className="h-52 w-full" />;
  if (error) return <UpstreamError message={(error as Error).message} />;
  if (!data || data.points.length === 0) {
    return <p className="text-xs text-muted-foreground py-8 text-center">No datapoints logged for “{metric}”.</p>;
  }

  return (
    <ResponsiveContainer width="100%" height={220}>
      <LineChart data={data.points} margin={{ top: 8, right: 16, bottom: 4, left: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
        <XAxis
          dataKey="step"
          tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
          tickLine={false}
          axisLine={{ stroke: "hsl(var(--border))" }}
          label={{ value: "step", position: "insideBottomRight", offset: -2, fontSize: 10, fill: "hsl(var(--muted-foreground))" }}
        />
        <YAxis
          width={56}
          tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
          tickFormatter={(v: number) => fmtMetric(v)}
          tickLine={false}
          axisLine={false}
          domain={["auto", "auto"]}
        />
        <Tooltip
          cursor={{ stroke: "hsl(var(--muted-foreground))", strokeDasharray: "3 3" }}
          formatter={(value: number) => [fmtMetric(value), metric]}
          labelFormatter={(step) => `step ${step}`}
          contentStyle={{
            backgroundColor: "hsl(var(--popover))",
            border: "1px solid hsl(var(--border))",
            borderRadius: 6,
            fontSize: 12,
            color: "hsl(var(--popover-foreground))",
          }}
        />
        <Line
          type="monotone"
          dataKey="value"
          stroke="hsl(var(--primary))"
          strokeWidth={2}
          dot={false}
          activeDot={{ r: 4 }}
          isAnimationActive={false}
        />
      </LineChart>
    </ResponsiveContainer>
  );
};

// ─── page ────────────────────────────────────────────────────────────────────

const NO_RUNS_HINT =
  "Training publishes here when MLFLOW_TRACKING_URI is set on the training pipeline.";

const ModelTrainingPage = () => {
  const [expandedRun, setExpandedRun] = useState<string | null>(null);
  const [chartMetric, setChartMetric] = useState<string>("val_apcer_max");

  const deployedQuery = useQuery({
    queryKey: ["ml", "deployed-model"],
    queryFn: () => mlService.getDeployedModel(),
    staleTime: 30_000,
    retry: false,
  });

  const trainingQuery = useQuery({
    queryKey: ["ml", "runs", "liveness-training"],
    queryFn: () => mlService.listRuns("liveness-training", 25),
    staleTime: 30_000,
    retry: false,
  });

  const redteamQuery = useQuery({
    queryKey: ["ml", "runs", "liveness-redteam"],
    queryFn: () => mlService.listRuns("liveness-redteam", 25),
    staleTime: 30_000,
    retry: false,
  });

  const deployed = deployedQuery.data;
  const engineUnreachable = deployedQuery.isError || (deployed != null && !deployed.reachable);

  const toggleRun = (run: MLRun) => {
    if (expandedRun === run.runId) {
      setExpandedRun(null);
      return;
    }
    setExpandedRun(run.runId);
    // Default the chart to a metric this run actually logged.
    const available = TRAINING_CHART_METRICS.filter((m) => run.latestMetrics[m] != null);
    if (available.length > 0 && !available.includes(chartMetric)) {
      setChartMetric(available[0]);
    }
  };

  return (
    <DashboardLayout
      title="Model Training"
      subtitle="Liveness model training runs, red-team certification and the deployed engine"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "Model Training" }]}
    >
      <div className="space-y-6 animate-fade-in">
        {/* Deployed model */}
        <Card className={engineUnreachable ? "border-warning/50" : ""}>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <ShieldCheck className="h-4 w-4 text-muted-foreground" />
              Deployed Liveness Model
              {deployed?.reachable && deployed.status && (
                <Badge
                  variant="outline"
                  className={`text-[10px] ${
                    deployed.status === "ok"
                      ? "bg-success/15 text-success border-success/30"
                      : "bg-warning/15 text-warning border-warning/30"
                  }`}
                >
                  {deployed.status === "ok" ? "Healthy" : "Degraded"}
                </Badge>
              )}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {deployedQuery.isLoading ? (
              <Skeleton className="h-16 w-full" />
            ) : engineUnreachable ? (
              <div className="flex items-start gap-3 rounded-md bg-warning/10 border border-warning/30 p-3">
                <AlertTriangle className="h-4 w-4 text-warning shrink-0 mt-0.5" />
                <div>
                  <p className="text-sm font-medium text-warning">eKYC engine unreachable</p>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    {deployed?.message ?? (deployedQuery.error as Error | null)?.message ??
                      "Deployed model identity is unavailable until the engine answers /health."}
                  </p>
                </div>
              </div>
            ) : (
              <div className="grid grid-cols-2 md:grid-cols-4 gap-x-6 gap-y-3">
                <div>
                  <p className="text-xs text-muted-foreground">Model</p>
                  <p className="text-sm font-medium">{deployed?.modelName || deployed?.livenessEngine || "—"}</p>
                </div>
                <div className="md:col-span-2">
                  <p className="text-xs text-muted-foreground">Checksum</p>
                  <p className="text-xs font-mono break-all mt-0.5">{deployed?.modelChecksum || "—"}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Face Engine</p>
                  <p className="text-sm">{deployed?.faceEngine || "—"}</p>
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        {/* Training runs */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">
              Training Runs <span className="text-muted-foreground font-normal">— liveness-training</span>
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {trainingQuery.isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : trainingQuery.isError ? (
              <UpstreamError message={(trainingQuery.error as Error).message} />
            ) : (trainingQuery.data ?? []).length === 0 ? (
              <EmptyRuns subtitle={NO_RUNS_HINT} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-8" />
                    <TableHead className="text-xs">Run</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Started</TableHead>
                    <TableHead className="text-xs text-right">val_apcer_max</TableHead>
                    <TableHead className="text-xs text-right">val_bpcer</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(trainingQuery.data ?? []).map((run) => (
                    <Fragment key={run.runId}>
                      <TableRow
                        className="table-row-hover cursor-pointer"
                        onClick={() => toggleRun(run)}
                      >
                        <TableCell className="pr-0">
                          <Button variant="ghost" size="sm" className="h-6 w-6 p-0"
                            aria-label={expandedRun === run.runId ? "Collapse run" : "Expand run"}>
                            {expandedRun === run.runId
                              ? <ChevronDown className="h-3.5 w-3.5" />
                              : <ChevronRight className="h-3.5 w-3.5" />}
                          </Button>
                        </TableCell>
                        <TableCell className="text-xs font-medium">
                          {run.name}
                          <span className="ml-2 font-mono text-muted-foreground">{run.runId.slice(0, 8)}</span>
                        </TableCell>
                        <TableCell><RunStatusChip status={run.status} /></TableCell>
                        <TableCell className="text-xs whitespace-nowrap">{fmtWhen(run.startTime)}</TableCell>
                        <TableCell className="text-xs text-right font-mono">
                          {fmtMetric(run.latestMetrics["val_apcer_max"])}
                        </TableCell>
                        <TableCell className="text-xs text-right font-mono">
                          {fmtMetric(bpcerOf(run.latestMetrics))}
                        </TableCell>
                      </TableRow>
                      {expandedRun === run.runId && (
                        <TableRow>
                          <TableCell colSpan={6} className="bg-muted/30">
                            <div className="py-2 space-y-3">
                              <div className="flex items-center gap-1">
                                {TRAINING_CHART_METRICS.map((m) => (
                                  <Button
                                    key={m}
                                    variant={chartMetric === m ? "secondary" : "ghost"}
                                    size="sm"
                                    className="h-6 text-[11px] font-mono"
                                    onClick={(e) => { e.stopPropagation(); setChartMetric(m); }}
                                  >
                                    {m}
                                  </Button>
                                ))}
                              </div>
                              <MetricHistoryChart runId={run.runId} metric={chartMetric} />
                              {Object.keys(run.params).length > 0 && (
                                <p className="text-[11px] text-muted-foreground font-mono">
                                  {Object.entries(run.params).slice(0, 8)
                                    .map(([k, v]) => `${k}=${v}`).join("  ·  ")}
                                </p>
                              )}
                            </div>
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Red-team runs */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">
              Red-Team Runs <span className="text-muted-foreground font-normal">— liveness-redteam</span>
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {redteamQuery.isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : redteamQuery.isError ? (
              <UpstreamError message={(redteamQuery.error as Error).message} />
            ) : (redteamQuery.data ?? []).length === 0 ? (
              <EmptyRuns subtitle={`Red-team certification results land here after each rig run. ${NO_RUNS_HINT}`} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Run</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Started</TableHead>
                    <TableHead className="text-xs text-right">Worst-species APCER</TableHead>
                    <TableHead className="text-xs text-right">BPCER</TableHead>
                    <TableHead className="text-xs">L1 Gate</TableHead>
                    <TableHead className="text-xs">L2 Gate</TableHead>
                    <TableHead className="text-xs">Report</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(redteamQuery.data ?? []).map((run) => (
                    <TableRow key={run.runId} className="table-row-hover">
                      <TableCell className="text-xs font-medium">
                        {run.name}
                        <span className="ml-2 font-mono text-muted-foreground">{run.runId.slice(0, 8)}</span>
                      </TableCell>
                      <TableCell><RunStatusChip status={run.status} /></TableCell>
                      <TableCell className="text-xs whitespace-nowrap">{fmtWhen(run.startTime)}</TableCell>
                      <TableCell className="text-xs text-right font-mono">
                        {fmtMetric(worstApcer(run.latestMetrics))}
                      </TableCell>
                      <TableCell className="text-xs text-right font-mono">
                        {fmtMetric(bpcerOf(run.latestMetrics))}
                      </TableCell>
                      <TableCell><GateChip verdict={run.tags["l1_gate"]} /></TableCell>
                      <TableCell><GateChip verdict={run.tags["l2_gate"]} /></TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {run.tags["report"] ?? run.tags["report_path"] ?? "in run artifacts"}
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

export default ModelTrainingPage;
