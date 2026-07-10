-- Revenue & write-off GL accounts (week 2: HIGH-2, BLOCKER-2/3/4).
-- 1350: penalty accruals are receivables (accrual basis), distinct from 1300 Fee Receivable.
-- 4110: loan processing/product fee income, distinct from generic 4100 Fee Income.
-- 4500: transfer charges, so fee revenue by rail is reportable separately.
-- Write-offs use the existing 1410 Allowance for Credit Losses (seeded in migration 3).
INSERT INTO chart_of_accounts (tenant_id, code, name, account_type, balance_type, description) VALUES
    ('system', '1350', 'Penalty Receivable', 'ASSET', 'DEBIT', 'Accrued late-payment penalties due from borrowers'),
    ('system', '4110', 'Loan Fee Income', 'INCOME', 'CREDIT', 'Loan processing and product fees charged to borrowers'),
    ('system', '4500', 'Transfer Fee Income', 'INCOME', 'CREDIT', 'Charges on customer fund transfers')
ON CONFLICT (tenant_id, code) DO NOTHING;
