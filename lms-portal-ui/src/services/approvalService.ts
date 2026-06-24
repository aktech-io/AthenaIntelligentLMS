import { apiGet, apiPost, apiPut } from "@/lib/api";

export interface ControlConfig {
  tenantId: string;
  operation: string;
  enabled: boolean;
  thresholdAmount: number;
}

export interface PendingApproval {
  id: string;
  operation: string;
  entityType?: string;
  entityId?: string;
  amount?: number;
  description?: string;
  status: string;
  makerId?: string;
  makerRole?: string;
  checkerId?: string;
  checkerRole?: string;
  reason?: string;
  createdAt: string;
  decidedAt?: string;
}

const BASE = "/proxy/auth/api/v1";
// Loan maker-checker config is served by the origination service, proxied at
// /proxy/loan-applications (not /proxy/loans, which routes to loan-management).
const LOAN_BASE = "/proxy/loan-applications/api/v1";

export const approvalService = {
  async listControlConfig(): Promise<ControlConfig[]> {
    const result = await apiGet<ControlConfig[]>(`${BASE}/control-config`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to load control config");
    }
    return result.data;
  },

  async updateControlConfig(
    operation: string,
    enabled: boolean,
    threshold: number
  ): Promise<ControlConfig[]> {
    const result = await apiPut<ControlConfig[]>(`${BASE}/control-config`, {
      operation,
      enabled,
      threshold,
    });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to update control config");
    }
    return result.data;
  },

  async listLoanControlConfig(): Promise<ControlConfig[]> {
    const result = await apiGet<ControlConfig[]>(`${LOAN_BASE}/control-config`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to load loan control config");
    }
    return result.data;
  },

  async updateLoanControlConfig(
    operation: string,
    enabled: boolean,
    threshold: number
  ): Promise<ControlConfig[]> {
    const result = await apiPut<ControlConfig[]>(`${LOAN_BASE}/control-config`, {
      operation,
      enabled,
      threshold,
    });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to update loan control config");
    }
    return result.data;
  },

  async listPendingApprovals(status?: string): Promise<PendingApproval[]> {
    const url = status
      ? `${BASE}/pending-approvals?status=${encodeURIComponent(status)}`
      : `${BASE}/pending-approvals`;
    const result = await apiGet<PendingApproval[]>(url);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to load pending approvals");
    }
    return result.data;
  },

  async approvePending(id: string): Promise<PendingApproval> {
    const result = await apiPost<PendingApproval>(
      `${BASE}/pending-approvals/${id}/approve`,
      {}
    );
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to approve");
    }
    return result.data;
  },

  async rejectPending(id: string, reason: string): Promise<void> {
    const result = await apiPost<PendingApproval>(
      `${BASE}/pending-approvals/${id}/reject`,
      { reason }
    );
    if (result.error) {
      throw new Error(result.error);
    }
  },
};
