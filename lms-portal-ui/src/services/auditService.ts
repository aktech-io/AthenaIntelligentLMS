import { apiGet } from "@/lib/api";

/**
 * Tamper-evident audit-chain verification.
 * Each tamper-evident service exposes a verify endpoint returning the
 * hash-linked chain integrity result. The /proxy/<svc> prefixes below
 * match those used by the corresponding per-service modules in src/services/.
 */
export interface AuditChainResult {
  intact: boolean;
  brokenSeq?: number;
  total: number;
}

export type AuditDomainKey =
  | "account"
  | "loans"
  | "accounting"
  | "overdraft"
  | "fraud"
  | "payment"
  | "collections"
  | "float"
  | "product";

export interface AuditDomainDef {
  key: AuditDomainKey;
  label: string;
  url: string;
}

// Verify endpoints — prefixes match the existing per-service modules:
//  account-service    -> /proxy/auth        (accountService.ts)
//  loan-management     -> /proxy/loans       (loanManagementService.ts)
//  accounting-service  -> /proxy/accounting  (accountingService.ts)
//  overdraft-service   -> /proxy/overdraft   (overdraftService.ts)
//  fraud-service       -> /proxy/fraud       (fraudService.ts)
//  payment-service     -> /proxy/payments    (nginx.conf / vite.config.ts)
//  collections-service -> /proxy/collections (collectionsService.ts)
//  float-service       -> /proxy/float       (floatService.ts)
//  product-service     -> /proxy/products    (productService.ts)
export const AUDIT_DOMAINS: AuditDomainDef[] = [
  { key: "account", label: "account-service", url: "/proxy/auth/api/v1/audit-log/verify" },
  { key: "loans", label: "loan-management", url: "/proxy/loans/api/v1/audit-log/verify" },
  { key: "accounting", label: "accounting-service", url: "/proxy/accounting/api/v1/accounting/audit-log/verify" },
  { key: "overdraft", label: "overdraft-service", url: "/proxy/overdraft/api/v1/overdraft/audit/verify" },
  { key: "fraud", label: "fraud-service", url: "/proxy/fraud/api/v1/fraud/audit/verify" },
  { key: "payment", label: "payment-service", url: "/proxy/payments/api/v1/audit-log/verify" },
  { key: "collections", label: "collections-service", url: "/proxy/collections/api/v1/audit-log/verify" },
  { key: "float", label: "float-service", url: "/proxy/float/api/v1/audit-log/verify" },
  { key: "product", label: "product-service", url: "/proxy/products/api/v1/audit-log/verify" },
];

async function verify(url: string): Promise<AuditChainResult> {
  const result = await apiGet<AuditChainResult>(url);
  if (result.error || !result.data) {
    throw new Error(result.error ?? "Failed to verify audit chain");
  }
  return result.data;
}

export const auditService = {
  /** Verify a single domain's tamper-evident chain. */
  verifyChain(domain: AuditDomainDef): Promise<AuditChainResult> {
    return verify(domain.url);
  },
  // Back-compat convenience helpers.
  verifyAccountChain(): Promise<AuditChainResult> {
    return verify(AUDIT_DOMAINS[0].url);
  },
  verifyLoanChain(): Promise<AuditChainResult> {
    return verify(AUDIT_DOMAINS[1].url);
  },
};
