ALTER TABLE users RENAME COLUMN balance_usd TO balance_rub;
ALTER TABLE billing_transactions RENAME COLUMN amount_usd TO amount_rub;
ALTER TABLE yookassa_payments DROP COLUMN IF EXISTS amount_usd;
