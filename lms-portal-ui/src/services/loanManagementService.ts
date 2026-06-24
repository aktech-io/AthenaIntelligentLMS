import { apiGet, apiPost, type PageResponse } from "@/lib/api";

export interface Loan {
  id: string;
  customerId: string;
  applicationId?: string;
  productId?: string;
  status: string;
  currency: string;
  disbursedAmount: number;
  outstandingPrincipal: number;
  outstandingInterest?: number;
  totalOutstanding?: number;
  interestRate?: number;
  tenorMonths?: number;
  disbursedAt: string;
  maturityDate?: string;
  dpd: number;
  nextDueDate?: string;
  createdAt?: string;
}

export interface Installment {
  installmentNo: number;
  dueDate: string;
  principalDue: number;
  interestDue: number;
  totalDue: number;
  principalPaid?: number;
  interestPaid?: number;
  totalPaid?: number;
  status: string;
  paidDate?: string;
}

export interface Repayment {
  id: string;
  loanId: string;
  amount: number;
  paymentDate: string;
  paymentMethod?: string;
  reference?: string;
  status?: string;
  createdAt?: string;
}

export interface RepaymentRequest {
  amount: number;
  paymentDate?: string;
  paymentMethod?: string;
  reference?: string;
}

export interface DpdInfo {
  loanId: string;
  dpd: number;
  stage: string;
  lastPaymentDate?: string;
  nextDueDate?: string;
}

const BASE = "/proxy/loans/api/v1/loans";
const REPAYMENTS_BASE = "/proxy/loans/api/v1/repayments";

export const loanManagementService = {
  async listLoans(
    page = 0,
    size = 20,
    status?: string
  ): Promise<PageResponse<Loan>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    if (status) params.set("status", status);
    const result = await apiGet<PageResponse<Loan>>(`${BASE}?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to list loans");
    }
    return result.data;
  },

  async getLoan(id: string): Promise<Loan> {
    const result = await apiGet<Loan>(`${BASE}/${id}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch loan");
    }
    return result.data;
  },

  async getLoanSchedule(id: string): Promise<Installment[]> {
    const result = await apiGet<Installment[]>(`${BASE}/${id}/schedule`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch schedule");
    }
    return result.data;
  },

  async getLoanRepayments(id: string): Promise<Repayment[]> {
    const result = await apiGet<Repayment[]>(`${BASE}/${id}/repayments`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch repayments");
    }
    return result.data;
  },

  async applyRepayment(id: string, req: RepaymentRequest): Promise<Repayment> {
    // Repayments post to the top-level /repayments endpoint with loanId in the
    // body. POST /loans/{id}/repayments is GET-only on the backend (405).
    const result = await apiPost<Repayment>(REPAYMENTS_BASE, { loanId: id, ...req });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to apply repayment");
    }
    return result.data;
  },

  async getLoansByCustomer(customerId: string): Promise<Loan[]> {
    const result = await apiGet<Loan[]>(`${BASE}?customerId=${customerId}&size=100`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch customer loans");
    }
    // Handle both array and page response
    const data = result.data as unknown;
    if (Array.isArray(data)) return data as Loan[];
    const page = data as PageResponse<Loan>;
    return page.content ?? [];
  },

  async getDpd(id: string): Promise<DpdInfo> {
    const result = await apiGet<DpdInfo>(`${BASE}/${id}/dpd`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch DPD info");
    }
    return result.data;
  },
};
