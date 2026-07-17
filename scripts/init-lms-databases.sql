-- AthenaLMS — Create all service databases
-- Run as postgres superuser before starting services
-- Usage: psql -U postgres -f init-lms-databases.sql

\set ON_ERROR_STOP on

-- ─── Account Service ──────────────────────────────────────────────────────────
SELECT 'Creating athena_accounts...' AS step;
CREATE DATABASE athena_accounts
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_accounts
GRANT ALL PRIVILEGES ON DATABASE athena_accounts TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;

-- ─── Product Service ──────────────────────────────────────────────────────────
SELECT 'Creating athena_products...' AS step;
CREATE DATABASE athena_products
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_products
GRANT ALL PRIVILEGES ON DATABASE athena_products TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ─── Loan Services (shared DB) ────────────────────────────────────────────────
SELECT 'Creating athena_loans...' AS step;
CREATE DATABASE athena_loans
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_loans
GRANT ALL PRIVILEGES ON DATABASE athena_loans TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ─── Payment Service ──────────────────────────────────────────────────────────
SELECT 'Creating athena_payments...' AS step;
CREATE DATABASE athena_payments
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_payments
GRANT ALL PRIVILEGES ON DATABASE athena_payments TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Accounting Service ───────────────────────────────────────────────────────
SELECT 'Creating athena_accounting...' AS step;
CREATE DATABASE athena_accounting
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_accounting
GRANT ALL PRIVILEGES ON DATABASE athena_accounting TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Float Service ────────────────────────────────────────────────────────────
SELECT 'Creating athena_float...' AS step;
CREATE DATABASE athena_float
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_float
GRANT ALL PRIVILEGES ON DATABASE athena_float TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Collections Service ──────────────────────────────────────────────────────
SELECT 'Creating athena_collections...' AS step;
CREATE DATABASE athena_collections
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_collections
GRANT ALL PRIVILEGES ON DATABASE athena_collections TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Compliance Service ───────────────────────────────────────────────────────
SELECT 'Creating athena_compliance...' AS step;
CREATE DATABASE athena_compliance
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_compliance
GRANT ALL PRIVILEGES ON DATABASE athena_compliance TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Reporting Service ────────────────────────────────────────────────────────
SELECT 'Creating athena_reporting...' AS step;
CREATE DATABASE athena_reporting
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_reporting
GRANT ALL PRIVILEGES ON DATABASE athena_reporting TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS dblink;

-- ─── AI Scoring Service ───────────────────────────────────────────────────────
SELECT 'Creating athena_scoring...' AS step;
CREATE DATABASE athena_scoring
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_scoring
GRANT ALL PRIVILEGES ON DATABASE athena_scoring TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Overdraft Service ────────────────────────────────────────────────────────
SELECT 'Creating athena_overdraft...' AS step;
CREATE DATABASE athena_overdraft
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_overdraft
GRANT ALL PRIVILEGES ON DATABASE athena_overdraft TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Media Service ────────────────────────────────────────────────────────────
SELECT 'Creating athena_media...' AS step;
CREATE DATABASE athena_media
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_media
GRANT ALL PRIVILEGES ON DATABASE athena_media TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Fraud Detection Service ─────────────────────────────────────────────────
SELECT 'Creating athena_fraud...' AS step;
CREATE DATABASE athena_fraud
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_fraud
GRANT ALL PRIVILEGES ON DATABASE athena_fraud TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Notification Service ─────────────────────────────────────────────────────
SELECT 'Creating athena_notifications...' AS step;
CREATE DATABASE athena_notifications
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_notifications
GRANT ALL PRIVILEGES ON DATABASE athena_notifications TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Decision Service (Nemo E1 decision spine) ───────────────────────────────
SELECT 'Creating athena_decision...' AS step;
CREATE DATABASE athena_decision
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_decision
GRANT ALL PRIVILEGES ON DATABASE athena_decision TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─── Card Service (Nemo B1 card issuing) ─────────────────────────────────────
SELECT 'Creating athena_cards...' AS step;
CREATE DATABASE athena_cards
    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;

\c athena_cards
GRANT ALL PRIVILEGES ON DATABASE athena_cards TO athena;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

SELECT 'All LMS databases created successfully.' AS result;
