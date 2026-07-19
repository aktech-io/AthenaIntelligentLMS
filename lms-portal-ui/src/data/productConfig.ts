// ─── GL Chart of Accounts ────────────────────────────
export const chartOfAccounts = [
  { code: "1001", name: "Cash & Bank Balances" },
  { code: "1100", name: "Loan Portfolio — Principal" },
  { code: "1101", name: "Loan Portfolio — Interest Receivable" },
  { code: "1200", name: "Provision for Loan Losses" },
  { code: "2001", name: "Customer Deposits" },
  { code: "2100", name: "Accrued Interest Payable" },
  { code: "3001", name: "Share Capital" },
  { code: "3100", name: "Retained Earnings" },
  { code: "4001", name: "Interest Income — Loans" },
  { code: "4002", name: "Fee Income — Origination" },
  { code: "4003", name: "Fee Income — Late Payment" },
  { code: "4004", name: "Float Fee Income" },
  { code: "5001", name: "Interest Expense — Deposits" },
  { code: "5100", name: "Provision Expense — Stage 1" },
  { code: "5101", name: "Provision Expense — Stage 2" },
  { code: "5102", name: "Provision Expense — Stage 3" },
  { code: "5200", name: "Operating Expenses" },
  { code: "6001", name: "Write-Off Expense" },
  { code: "6100", name: "Recovery Income" },
];

// ─── GL Event Mapping Defaults ──────────────────────
export interface GLMapping {
  event: string;
  debitAccount: string;
  creditAccount: string;
}

export const defaultGLMappings: GLMapping[] = [
  { event: "Disbursement", debitAccount: "1100", creditAccount: "1001" },
  { event: "Interest Accrual", debitAccount: "1101", creditAccount: "4001" },
  { event: "Fee Collection", debitAccount: "1001", creditAccount: "4002" },
  { event: "Repayment — Principal", debitAccount: "1001", creditAccount: "1100" },
  { event: "Repayment — Interest", debitAccount: "1001", creditAccount: "1101" },
  { event: "Late Payment Fee", debitAccount: "1001", creditAccount: "4003" },
  { event: "Write-Off", debitAccount: "6001", creditAccount: "1100" },
  { event: "Recovery", debitAccount: "1001", creditAccount: "6100" },
];

// ─── Product Templates ──────────────────────────────
export const productTemplates = [
  { id: "tpl-1", icon: "🚀", name: "Digital Nano-Loan (Fuliza-style)", description: "Instant micro-loans with daily interest, auto-debit from wallet. High-frequency, low-value.", keyParams: "KES 500–10K · 1–30 days · 1.5% daily · Auto-collect" },
  { id: "tpl-2", icon: "🏦", name: "Consumer Personal Loan (Standard Bank)", description: "Standard EMI-based personal loan with credit scoring and KYC requirements.", keyParams: "KES 10K–5M · 3–60 months · 24.8% p.a. · Monthly EMI" },
  { id: "tpl-3", icon: "🛒", name: "BNPL 3-Month Interest-Free", description: "Buy now pay later product for merchant partnerships. Zero interest, merchant-funded.", keyParams: "KES 1K–200K · 3 months · 0% interest · Merchant 4% fee" },
  { id: "tpl-4", icon: "🏢", name: "SME Business Term Loan", description: "Medium-term business financing with flexible collateral and graduated repayment.", keyParams: "KES 100K–20M · 6–60 months · 22.4% p.a. · Monthly EMI" },
  { id: "tpl-5", icon: "👥", name: "Group Solidarity Loan (MFI)", description: "Group-guaranteed micro-loans for informal sector and agriculture.", keyParams: "KES 5K–500K · 3–24 months · 18% p.a. · Weekly" },
];
