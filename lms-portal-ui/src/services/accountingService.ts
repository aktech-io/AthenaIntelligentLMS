import { apiGet, apiPost, type PageResponse } from "@/lib/api";

export interface JournalEntryLine {
  id: string;
  accountId: string;
  accountCode?: string;
  accountName?: string;
  lineNo?: number;
  description?: string;
  debitAmount: number;
  creditAmount: number;
  currency?: string;
}

export interface JournalEntry {
  id: string;
  tenantId?: string;
  reference: string;
  description: string;
  entryDate: string;
  status: string;
  sourceEvent?: string;
  sourceId?: string;
  totalDebit: number;
  totalCredit: number;
  postedBy?: string;
  createdAt?: string;
  lines?: JournalEntryLine[];
}

export interface TrialBalanceAccount {
  accountId: string;
  accountCode: string;
  accountName: string;
  accountType: string;
  balanceType: string;
  balance: number;
  currency?: string;
  periodYear?: number;
  periodMonth?: number;
}

export interface TrialBalanceResponse {
  periodYear: number;
  periodMonth: number;
  accounts: TrialBalanceAccount[];
  totalDebits: number;
  totalCredits: number;
  balanced: boolean;
}

export interface GLAccount {
  id: string;
  accountCode: string;
  accountName: string;
  accountType: string;
  parentId?: string;
  currency?: string;
  status?: string;
  balance?: number;
  createdAt?: string;
}

export interface FiscalPeriod {
  id: string;
  tenantId: string;
  periodYear: number;
  periodMonth: number;
  status: "OPEN" | "SOFT_CLOSED" | "CLOSED";
  closedBy?: string;
  closedAt?: string;
  reopenedBy?: string;
  reopenReason?: string;
}

export interface AuditLogEntry {
  id: string;
  tenantId: string;
  action: string;
  entityType: string;
  entityId: string;
  userId?: string;
  userRole?: string;
  details?: any;
  ipAddress?: string;
  createdAt: string;
}

export interface CashFlowItem {
  description: string;
  amount: number;
}

export interface CashFlowResponse {
  periodYear: number;
  periodMonth: number;
  operatingItems: CashFlowItem[];
  investingItems: CashFlowItem[];
  financingItems: CashFlowItem[];
  totalOperating: number;
  totalInvesting: number;
  totalFinancing: number;
  netCashFlow: number;
  openingCash: number;
  closingCash: number;
}

const BASE = "/proxy/accounting/api/v1/accounting";

export const accountingService = {
  async listGLAccounts(page = 0, size = 50): Promise<PageResponse<GLAccount>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    const result = await apiGet<PageResponse<GLAccount>>(`${BASE}/accounts?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to list GL accounts");
    }
    return result.data;
  },

  async listJournalEntries(page = 0, size = 20, from?: string, to?: string, status?: string): Promise<PageResponse<JournalEntry>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    if (status) params.set("status", status);
    const result = await apiGet<PageResponse<JournalEntry>>(`${BASE}/journal-entries?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to list journal entries");
    }
    return result.data;
  },

  async getTrialBalance(year?: number, month?: number): Promise<TrialBalanceResponse> {
    const params = new URLSearchParams();
    if (year) params.set("year", String(year));
    if (month) params.set("month", String(month));
    const qs = params.toString();
    const result = await apiGet<TrialBalanceResponse>(`${BASE}/trial-balance${qs ? `?${qs}` : ""}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch trial balance");
    }
    return result.data;
  },

  async submitEntry(id: string): Promise<JournalEntry> {
    const result = await apiPost<JournalEntry>(`${BASE}/journal-entries/${id}/submit`, {});
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to submit entry");
    return result.data;
  },

  async approveEntry(id: string): Promise<JournalEntry> {
    const result = await apiPost<JournalEntry>(`${BASE}/journal-entries/${id}/approve`, {});
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to approve entry");
    return result.data;
  },

  async rejectEntry(id: string, reason: string): Promise<JournalEntry> {
    const result = await apiPost<JournalEntry>(`${BASE}/journal-entries/${id}/reject`, { reason });
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to reject entry");
    return result.data;
  },

  async reverseEntry(id: string, reason: string): Promise<JournalEntry> {
    const result = await apiPost<JournalEntry>(`${BASE}/journal-entries/${id}/reverse`, { reason });
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to reverse entry");
    return result.data;
  },

  async listPeriods(): Promise<FiscalPeriod[]> {
    const result = await apiGet<FiscalPeriod[]>(`${BASE}/periods`);
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to list periods");
    return result.data;
  },

  async closePeriod(year: number, month: number): Promise<FiscalPeriod> {
    const result = await apiPost<FiscalPeriod>(`${BASE}/periods/${year}/${month}/close`, {});
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to close period");
    return result.data;
  },

  async reopenPeriod(year: number, month: number, reason: string): Promise<FiscalPeriod> {
    const result = await apiPost<FiscalPeriod>(`${BASE}/periods/${year}/${month}/reopen`, { reason });
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to reopen period");
    return result.data;
  },

  async listAuditLogs(page = 0, size = 20, entityType?: string, from?: string, to?: string): Promise<PageResponse<AuditLogEntry>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    if (entityType) params.set("entityType", entityType);
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const result = await apiGet<PageResponse<AuditLogEntry>>(`${BASE}/audit-log?${params}`);
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to list audit logs");
    return result.data;
  },

  async getCashFlow(year?: number, month?: number): Promise<CashFlowResponse> {
    const params = new URLSearchParams();
    if (year) params.set("year", String(year));
    if (month) params.set("month", String(month));
    const qs = params.toString();
    const result = await apiGet<CashFlowResponse>(`${BASE}/cash-flow${qs ? `?${qs}` : ""}`);
    if (result.error || !result.data) throw new Error(result.error ?? "Failed to fetch cash flow");
    return result.data;
  },

  /** Builds the CSV export URL for a report by appending format=csv. */
  csvUrl(report: "trial-balance" | "cash-flow", year?: number, month?: number): string {
    const params = new URLSearchParams();
    if (year) params.set("year", String(year));
    if (month) params.set("month", String(month));
    params.set("format", "csv");
    return `${BASE}/${report}?${params.toString()}`;
  },
};
