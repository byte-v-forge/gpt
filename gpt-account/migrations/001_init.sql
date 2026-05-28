CREATE TABLE IF NOT EXISTS accounts (
  id text PRIMARY KEY,
  email text NOT NULL UNIQUE,
  password text NOT NULL DEFAULT '',
  created_at bigint NOT NULL DEFAULT 0,
  updated_at bigint NOT NULL DEFAULT 0
);

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS password text NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS created_at bigint NOT NULL DEFAULT 0;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS updated_at bigint NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS gpt_email_allocations (
  email text PRIMARY KEY,
  primary_email text NOT NULL DEFAULT '',
  is_primary boolean NOT NULL DEFAULT false,
  status text NOT NULL DEFAULT '',
  splittable boolean NOT NULL DEFAULT false,
  assigned_account_id text NOT NULL DEFAULT '',
  last_error text NOT NULL DEFAULT '',
  created_at bigint NOT NULL DEFAULT 0,
  updated_at bigint NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_gpt_email_allocations_primary_email ON gpt_email_allocations(primary_email);
CREATE INDEX IF NOT EXISTS idx_gpt_email_allocations_is_primary ON gpt_email_allocations(is_primary);
CREATE INDEX IF NOT EXISTS idx_gpt_email_allocations_status ON gpt_email_allocations(status);
CREATE INDEX IF NOT EXISTS idx_gpt_email_allocations_splittable ON gpt_email_allocations(splittable);
CREATE INDEX IF NOT EXISTS idx_gpt_email_allocations_assigned_account_id ON gpt_email_allocations(assigned_account_id);

ALTER TABLE accounts DROP COLUMN IF EXISTS session_token;
ALTER TABLE accounts DROP COLUMN IF EXISTS access_token;
ALTER TABLE accounts DROP COLUMN IF EXISTS mailbox_latest_otp;
ALTER TABLE accounts DROP COLUMN IF EXISTS mailbox_latest_otp_subject;
ALTER TABLE accounts DROP COLUMN IF EXISTS mailbox_latest_otp_received_at_unix;
ALTER TABLE accounts DROP COLUMN IF EXISTS codex_auth_json;
ALTER TABLE accounts DROP COLUMN IF EXISTS codex_auth_updated_at_unix;
ALTER TABLE accounts DROP COLUMN IF EXISTS status;
ALTER TABLE accounts DROP COLUMN IF EXISTS error_message;
ALTER TABLE accounts DROP COLUMN IF EXISTS charge_ref;
ALTER TABLE accounts DROP COLUMN IF EXISTS first_name;
ALTER TABLE accounts DROP COLUMN IF EXISTS last_name;
ALTER TABLE accounts DROP COLUMN IF EXISTS dob;
ALTER TABLE accounts DROP COLUMN IF EXISTS plus_trial_eligible;
ALTER TABLE accounts DROP COLUMN IF EXISTS plus_active;
ALTER TABLE accounts DROP COLUMN IF EXISTS tier;
ALTER TABLE accounts DROP COLUMN IF EXISTS activation_channel;
ALTER TABLE accounts DROP COLUMN IF EXISTS primary_mailbox_email;
ALTER TABLE accounts DROP COLUMN IF EXISTS mailbox_last_fetched_at_unix;
ALTER TABLE accounts DROP COLUMN IF EXISTS mailbox_last_message_at_unix;
ALTER TABLE accounts DROP COLUMN IF EXISTS codex_phone_confirmed;
ALTER TABLE accounts DROP COLUMN IF EXISTS codex_phone_label;
ALTER TABLE accounts DROP COLUMN IF EXISTS codex_phone_updated_at_unix;
ALTER TABLE accounts DROP COLUMN IF EXISTS codex_phone_status;
DROP INDEX IF EXISTS idx_accounts_primary_mailbox_email;
