import { apiGet } from "@/lib/api";

/**
 * ML observability service for the Model Training page. Talks to the
 * compliance-service JWT-gated proxy (/api/v1/ml/*) which fronts the MLflow
 * tracking server — the portal never reaches MLflow directly.
 */

export type MLExperimentName = "liveness-training" | "liveness-redteam";

export interface MLExperiment {
  experimentId: string;
  name: string;
  lifecycleStage?: string;
  lastUpdateTime?: number; // epoch ms
}

export interface MLRun {
  runId: string;
  name: string;
  status: string; // RUNNING | FINISHED | FAILED | KILLED | SCHEDULED
  startTime?: number; // epoch ms
  endTime?: number; // epoch ms, absent while running
  params: Record<string, string>;
  latestMetrics: Record<string, number>;
  tags: Record<string, string>; // l1_gate / l2_gate = pass|fail ride here
}

export interface MLMetricPoint {
  step: number;
  timestamp: number;
  value: number;
}

export interface MLMetricHistory {
  runId: string;
  metric: string;
  points: MLMetricPoint[];
}

export interface MLDeployedModel {
  reachable: boolean;
  status?: string; // ok | degraded
  livenessEngine?: string;
  faceEngine?: string;
  modelName?: string;
  modelChecksum?: string;
  message?: string;
  engine?: Record<string, unknown>; // raw /health payload
}

const BASE = "/proxy/compliance/api/v1/ml";

export const mlService = {
  async listExperiments(): Promise<MLExperiment[]> {
    const result = await apiGet<{ experiments: MLExperiment[] }>(`${BASE}/experiments`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to list ML experiments");
    }
    return result.data.experiments ?? [];
  },

  async listRuns(experiment: MLExperimentName, limit = 25): Promise<MLRun[]> {
    const params = new URLSearchParams({ experiment, limit: String(limit) });
    const result = await apiGet<{ runs: MLRun[] }>(`${BASE}/runs?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? `Failed to list ${experiment} runs`);
    }
    return result.data.runs ?? [];
  },

  async getMetricHistory(runId: string, metric: string): Promise<MLMetricHistory> {
    const params = new URLSearchParams({ metric });
    const result = await apiGet<MLMetricHistory>(
      `${BASE}/runs/${encodeURIComponent(runId)}/metric-history?${params}`,
    );
    if (result.error || !result.data) {
      throw new Error(result.error ?? `Failed to load ${metric} history`);
    }
    return result.data;
  },

  async getDeployedModel(): Promise<MLDeployedModel> {
    const result = await apiGet<MLDeployedModel>(`${BASE}/deployed-model`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to load deployed model");
    }
    return result.data;
  },
};
