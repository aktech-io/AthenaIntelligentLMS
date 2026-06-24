import { apiGet, apiPost, apiPut, type PageResponse } from "@/lib/api";

export interface Account {
  id: string;
  accountNumber: string;
  accountType: string;
  customerId: string;
  status: string;
  currency: string;
  balance?: number | { availableBalance: number; currentBalance: number; ledgerBalance: number };
  branchCode?: string;
  createdAt?: string;
  // Deposit product fields
  depositProductId?: string;
  branchId?: string;
  openedBy?: string;
  closedAt?: string;
  closureReason?: string;
  lastTransactionDate?: string;
  dormantSince?: string;
  // Fixed deposit
  maturityDate?: string;
  termDays?: number;
  lockedAmount?: number;
  autoRenew?: boolean;
  // Interest
  accruedInterest?: number;
  lastInterestAccrualDate?: string;
  lastInterestPostingDate?: string;
  interestRateOverride?: number;
  accountName?: string;
  kycTier?: number;
}

export interface CreateAccountRequest {
  customerId: string;
  accountType: string;
  currency: string;
  branchCode?: string;
}

export interface OpenAccountRequest {
  customerId: string;
  depositProductId?: string;
  accountType: string;
  currency: string;
  kycTier: number;
  accountName: string;
  branchId?: string;
  initialDeposit?: number;
  termDays?: number;
  autoRenew?: boolean;
  interestRateOverride?: number;
}

export interface BalanceResponse {
  id?: string;
  accountId: string;
  accountNumber?: string;
  currency?: string;
  currentBalance: number;
  availableBalance: number;
  ledgerBalance?: number;
  updatedAt?: string;
}

export interface Transaction {
  id: string;
  accountId: string;
  transactionType: string;
  amount: number;
  currency: string;
  description?: string;
  reference?: string;
  valueDate: string;
  createdAt?: string;
  runningBalance?: number;
  balanceAfter?: number;
}

export interface PendingApprovalResult {
  status: string;
  pendingId: string;
  operation: string;
  message: string;
}

export interface TransferRequest {
  sourceAccountId: string;
  destinationAccountId?: string;
  destinationAccountNumber?: string;
  amount: number;
  transferType: string;
  narration?: string;
  idempotencyKey?: string;
}

export interface TransferResponse {
  id: string;
  sourceAccountId: string;
  sourceAccountNumber?: string;
  destinationAccountId: string;
  destinationAccountNumber?: string;
  amount: number;
  currency: string;
  transferType: string;
  status: string;
  reference: string;
  narration?: string;
  chargeAmount: number;
  initiatedAt: string;
  completedAt?: string;
  failedReason?: string;
}

export interface StatementResponse {
  accountNumber: string;
  customerName: string;
  currency: string;
  openingBalance: number;
  closingBalance: number;
  periodFrom: string;
  periodTo: string;
  transactions: PageResponse<Transaction>;
}

export interface InterestAccrual {
  id: string;
  accountId: string;
  accrualDate: string;
  balanceUsed: number;
  rate: number;
  dailyAmount: number;
  posted: boolean;
}

export interface InterestPosting {
  id: string;
  accountId: string;
  periodStart: string;
  periodEnd: string;
  grossInterest: number;
  withholdingTax: number;
  netInterest: number;
  postedAt: string;
  postedBy?: string;
}

export interface InterestSummary {
  accountId: string;
  unpostedTotal: number;
  recentAccruals: InterestAccrual[];
  postingHistory: InterestPosting[];
}

export interface EODResult {
  date: string;
  accountsAccrued: number;
  dormantDetected: number;
  maturedProcessed: number;
  status: string;
}

const BASE = "/proxy/auth/api/v1/accounts";
const TRANSFER_BASE = "/proxy/auth/api/v1/transfers";
const EOD_BASE = "/proxy/auth/api/v1/eod";

export const accountService = {
  async listAccounts(
    page = 0,
    size = 20
  ): Promise<PageResponse<Account>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    const result = await apiGet<PageResponse<Account>>(`${BASE}?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to list accounts");
    }
    return result.data;
  },

  async getAccount(id: string): Promise<Account> {
    const result = await apiGet<Account>(`${BASE}/${id}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch account");
    }
    return result.data;
  },

  async createAccount(req: CreateAccountRequest): Promise<Account> {
    const result = await apiPost<Account>(BASE, req);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to create account");
    }
    return result.data;
  },

  async openAccount(req: OpenAccountRequest): Promise<Account> {
    const result = await apiPost<Account>(`${BASE}/open`, req);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to open account");
    }
    return result.data;
  },

  async approveAccount(id: string): Promise<Account> {
    const result = await apiPost<Account>(`${BASE}/${id}/approve`, {});
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to approve account");
    }
    return result.data;
  },

  async closeAccount(id: string, reason: string): Promise<Account> {
    const result = await apiPost<Account>(`${BASE}/${id}/close`, { reason });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to close account");
    }
    return result.data;
  },

  async deposit(
    id: string,
    amount: number,
    description?: string
  ): Promise<Transaction | PendingApprovalResult> {
    const result = await apiPost<Transaction | PendingApprovalResult>(`${BASE}/${id}/credit`, {
      amount,
      description: description || "Cash deposit",
      channel: "TELLER",
    });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to deposit");
    }
    return result.data;
  },

  async withdraw(
    id: string,
    amount: number,
    description?: string
  ): Promise<Transaction | PendingApprovalResult> {
    const result = await apiPost<Transaction | PendingApprovalResult>(`${BASE}/${id}/debit`, {
      amount,
      description: description || "Cash withdrawal",
      channel: "TELLER",
    });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to withdraw");
    }
    return result.data;
  },

  async getBalance(id: string): Promise<BalanceResponse> {
    const result = await apiGet<BalanceResponse>(`${BASE}/${id}/balance`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch balance");
    }
    return result.data;
  },

  async getTransactions(
    id: string,
    page = 0,
    size = 20
  ): Promise<PageResponse<Transaction>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    const result = await apiGet<PageResponse<Transaction>>(
      `${BASE}/${id}/transactions?${params}`
    );
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch transactions");
    }
    return result.data;
  },

  async getCustomerAccounts(customerId: string): Promise<Account[]> {
    const result = await apiGet<Account[]>(`${BASE}/customer/${encodeURIComponent(customerId)}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch customer accounts");
    }
    return result.data;
  },

  async getStatement(
    accountId: string,
    from: string,
    to: string,
    page = 0,
    size = 50
  ): Promise<StatementResponse> {
    const params = new URLSearchParams({
      startDate: from, endDate: to, page: String(page), size: String(size),
    });
    const result = await apiGet<StatementResponse>(`${BASE}/${accountId}/statement?${params}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch statement");
    }
    return result.data;
  },

  async getInterestSummary(id: string): Promise<InterestSummary> {
    const result = await apiGet<InterestSummary>(`${BASE}/${id}/interest-summary`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch interest summary");
    }
    return result.data;
  },

  async postInterest(id: string): Promise<InterestPosting> {
    const result = await apiPost<InterestPosting>(`${BASE}/${id}/post-interest`, {});
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to post interest");
    }
    return result.data;
  },

  async updateStatus(id: string, status: string): Promise<Account> {
    const result = await apiPut<Account>(`${BASE}/${id}/status`, { status });
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to update status");
    }
    return result.data;
  },

  async runEOD(): Promise<EODResult> {
    const result = await apiPost<EODResult>(`${EOD_BASE}/run`, {});
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to run EOD");
    }
    return result.data;
  },

  async initiateTransfer(
    req: TransferRequest
  ): Promise<TransferResponse | PendingApprovalResult> {
    const result = await apiPost<TransferResponse | PendingApprovalResult>(TRANSFER_BASE, req);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to initiate transfer");
    }
    return result.data;
  },

  async getTransfer(id: string): Promise<TransferResponse> {
    const result = await apiGet<TransferResponse>(`${TRANSFER_BASE}/${id}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch transfer");
    }
    return result.data;
  },

  async getTransfersByAccount(
    accountId: string,
    page = 0,
    size = 20
  ): Promise<PageResponse<TransferResponse>> {
    const params = new URLSearchParams({ page: String(page), size: String(size) });
    const result = await apiGet<PageResponse<TransferResponse>>(
      `${TRANSFER_BASE}/account/${accountId}?${params}`
    );
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to fetch transfers");
    }
    return result.data;
  },

  async searchAccounts(q: string): Promise<Account[]> {
    const result = await apiGet<Account[]>(`${BASE}/search?q=${encodeURIComponent(q)}`);
    if (result.error || !result.data) {
      throw new Error(result.error ?? "Failed to search accounts");
    }
    return result.data;
  },
};
