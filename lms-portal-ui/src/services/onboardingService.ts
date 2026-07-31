import { apiGet, apiPost, type PageResponse } from "@/lib/api";

export type OnboardingStatus = "REFERRED" | "APPROVED" | "REJECTED" | "AUTO_APPROVED";
export type OnboardingDocumentType = "NATIONAL_ID" | "PASSPORT";
export type OnboardingRiskTier = "LOW" | "MEDIUM" | "HIGH";

export interface OnboardingApplication {
  id: string;
  tenantId: string;
  phone: string;
  fullName: string;
  nationalId: string;
  documentType: OnboardingDocumentType;
  dateOfBirth?: string;
  documentRef?: string;
  selfieRef?: string;
  status: OnboardingStatus;
  riskTier?: OnboardingRiskTier;
  provider?: string;
  decisionReasons?: string;
  // Passive-PAD (liveness) observability — absent when no PAD ran;
  // livenessScore absent when the score was unavailable (shadow-error).
  livenessScore?: number;
  livenessMode?: string;
  livenessProvider?: string;
  customerId?: string;
  decidedBy?: string;
  decidedAt?: string;
  createdAt?: string;
}

const BASE = "/proxy/compliance/api/v1/onboarding";

export const onboardingService = {
  async listApplications(status?: OnboardingStatus, page = 0, size = 50): Promise<PageResponse<OnboardingApplication>> {
    const params = new URLSearchParams({ page: String(page), size: String(size), tenantId: "*" });
    if (status) params.set("status", status);
    const result = await apiGet<PageResponse<OnboardingApplication>>(`${BASE}/?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to list onboarding applications");
    }
    return result.data;
  },

  async approveApplication(id: string, tenantId: string, reason: string): Promise<OnboardingApplication> {
    const params = new URLSearchParams({ tenantId });
    const result = await apiPost<OnboardingApplication>(`${BASE}/${id}/approve?${params}`, { reason });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to approve application");
    }
    return result.data;
  },

  async rejectApplication(id: string, tenantId: string, reason: string): Promise<OnboardingApplication> {
    const params = new URLSearchParams({ tenantId });
    const result = await apiPost<OnboardingApplication>(`${BASE}/${id}/reject?${params}`, { reason });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to reject application");
    }
    return result.data;
  },
};
