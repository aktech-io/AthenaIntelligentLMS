-- Contabo shared PG: create the Nemo service databases missing from the
-- original 16-service deployment. Owner matches the existing athena_* DBs.
-- Errors on already-existing databases are harmless (run without ON_ERROR_STOP).
CREATE DATABASE athena_decision          WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;
CREATE DATABASE athena_cards             WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;
CREATE DATABASE athena_mobile_gateway    WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;
CREATE DATABASE athena_bff_notifications WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;
CREATE DATABASE athena_billpay_savings   WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;
CREATE DATABASE athena_shop              WITH OWNER = athena ENCODING = 'UTF8' TEMPLATE = template0;
\c athena_decision
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
\c athena_cards
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
\c athena_mobile_gateway
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
\c athena_bff_notifications
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
\c athena_billpay_savings
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
\c athena_shop
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
