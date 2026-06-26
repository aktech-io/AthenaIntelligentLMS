import { apiGet } from "@/lib/api";

/**
 * Tamper-evident audit-chain verification.
 * The account-service and loan-management services each expose
 * GET /api/v1/audit-log/verify returning the chain integrity result.
 */
export interface AuditChainResult {
  intact: boolean;
  brokenSeq?: number;
  total: number;
}

// Match the proxy prefixes used by the existing service modules:
//  - account-service  -> /proxy/auth   (see accountService.ts)
//  - loan-management   -> /proxy/loans  (see loanManagementService.ts)
const ACCOUNT_VERIFY = "/proxy/auth/api/v1/audit-log/verify";
const LOANS_VERIFY = "/proxy/loans/api/v1/audit-log/verify";

async function verify(url: string): Promise<AuditChainResult> {
  const result = await apiGet<AuditChainResult>(url);
  if (result.error || !result.data) {
    throw new Error(result.error ?? "Failed to verify audit chain");
  }
  return result.data;
}

export const auditService = {
  verifyAccountChain(): Promise<AuditChainResult> {
    return verify(ACCOUNT_VERIFY);
  },
  verifyLoanChain(): Promise<AuditChainResult> {
    return verify(LOANS_VERIFY);
  },
};
