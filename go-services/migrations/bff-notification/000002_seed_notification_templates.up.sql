-- notification-service V2 -- seed notification templates for all tenants

-- Templates for 'default' tenant
INSERT INTO notification_templates (id, tenant_id, template_code, channel, title_template, body_template, category, active) VALUES
    (uuid_generate_v4(), 'default', 'WELCOME', 'IN_APP', 'Welcome to Athena!', 'Welcome {{name}}! Your Athena Mobile Wallet is ready.', 'SYSTEM', true),
    (uuid_generate_v4(), 'default', 'WELCOME', 'SMS', NULL, 'Welcome to Athena Mobile Wallet, {{name}}! Your account is ready.', 'SYSTEM', true),
    (uuid_generate_v4(), 'default', 'WELCOME', 'PUSH', 'Welcome to Athena!', 'Your mobile wallet is ready. Start transacting today!', 'SYSTEM', true),
    (uuid_generate_v4(), 'default', 'TRANSACTION_SUCCESS', 'IN_APP', 'Transaction Successful', 'Your transaction of {{amount}} was successful. Ref: {{reference}}', 'TRANSACTION', true),
    (uuid_generate_v4(), 'default', 'TRANSACTION_SUCCESS', 'SMS', NULL, 'Athena: {{amount}} sent successfully. Ref: {{reference}}. Balance: {{balance}}.', 'TRANSACTION', true),
    (uuid_generate_v4(), 'default', 'TRANSACTION_SUCCESS', 'PUSH', 'Payment Successful', '{{amount}} sent to {{recipient}}', 'TRANSACTION', true),
    (uuid_generate_v4(), 'default', 'LOAN_APPROVED', 'IN_APP', 'Loan Approved', 'Your loan of {{amount}} has been approved! Funds will be disbursed shortly.', 'LOAN', true),
    (uuid_generate_v4(), 'default', 'LOAN_APPROVED', 'PUSH', 'Loan Approved!', 'Great news! Your loan of {{amount}} has been approved.', 'LOAN', true),
    (uuid_generate_v4(), 'default', 'LOAN_APPROVED', 'SMS', NULL, 'Athena: Your loan of {{amount}} has been approved. Ref: {{reference}}.', 'LOAN', true),
    (uuid_generate_v4(), 'default', 'LOAN_REPAYMENT', 'IN_APP', 'Repayment Received', 'Your repayment of {{amount}} has been received. Remaining: {{balance}}.', 'LOAN', true),
    (uuid_generate_v4(), 'default', 'LOAN_DISBURSED', 'IN_APP', 'Loan Disbursed', 'Your loan of {{amount}} has been disbursed to your account.', 'LOAN', true),
    (uuid_generate_v4(), 'default', 'BILL_PAYMENT', 'IN_APP', 'Bill Payment Successful', 'Your {{biller}} payment of {{amount}} was successful.', 'TRANSACTION', true),
    (uuid_generate_v4(), 'default', 'SAVINGS_DEPOSIT', 'IN_APP', 'Savings Deposit', '{{amount}} deposited to your "{{goalName}}" savings goal.', 'TRANSACTION', true),
    (uuid_generate_v4(), 'default', 'SAVINGS_WITHDRAWAL', 'IN_APP', 'Savings Withdrawal', '{{amount}} withdrawn from your "{{goalName}}" savings goal.', 'TRANSACTION', true),
    (uuid_generate_v4(), 'default', 'ORDER_PLACED', 'IN_APP', 'Order Placed', 'Your order #{{orderId}} has been placed successfully.', 'TRANSACTION', true),
    (uuid_generate_v4(), 'default', 'ORDER_SHIPPED', 'IN_APP', 'Order Shipped', 'Your order #{{orderId}} has been shipped!', 'TRANSACTION', true),
    (uuid_generate_v4(), 'default', 'SECURITY_ALERT', 'IN_APP', 'Security Alert', '{{message}}', 'SECURITY', true),
    (uuid_generate_v4(), 'default', 'SECURITY_ALERT', 'SMS', NULL, 'Athena Security: {{message}}. If not you, contact support.', 'SECURITY', true),
    (uuid_generate_v4(), 'default', 'PROMOTION', 'IN_APP', '{{title}}', '{{message}}', 'PROMOTION', true),
    (uuid_generate_v4(), 'default', 'PROMOTION', 'PUSH', '{{title}}', '{{message}}', 'PROMOTION', true);

-- Duplicate templates for 'admin' tenant
INSERT INTO notification_templates (id, tenant_id, template_code, channel, title_template, body_template, category, active)
SELECT uuid_generate_v4(), 'admin', template_code, channel, title_template, body_template, category, active
FROM notification_templates
WHERE tenant_id = 'default';
