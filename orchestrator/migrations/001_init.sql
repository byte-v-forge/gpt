CREATE TABLE IF NOT EXISTS jobs (
  id text PRIMARY KEY,
  account_id text NOT NULL DEFAULT '',
  action text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT '',
  recoverable boolean NOT NULL DEFAULT false,
  retryable boolean NOT NULL DEFAULT false,
  last_step text NOT NULL DEFAULT '',
  error_message text NOT NULL DEFAULT '',
  result_json text NOT NULL DEFAULT '',
  claim_owner text NOT NULL DEFAULT '',
  claim_until bigint NOT NULL DEFAULT 0,
  attempt_count integer NOT NULL DEFAULT 0,
  n8n_execution_id text NOT NULL DEFAULT '',
  created_at bigint NOT NULL DEFAULT 0,
  updated_at bigint NOT NULL DEFAULT 0
);
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS claim_owner text NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS claim_until bigint NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS attempt_count integer NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS n8n_execution_id text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_jobs_account_id ON jobs(account_id);
CREATE INDEX IF NOT EXISTS idx_jobs_action ON jobs(action);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_claim_owner ON jobs(claim_owner);
CREATE INDEX IF NOT EXISTS idx_jobs_claim_until ON jobs(claim_until);
CREATE INDEX IF NOT EXISTS idx_jobs_n8n_execution_id ON jobs(n8n_execution_id);

CREATE TABLE IF NOT EXISTS job_params (job_id text NOT NULL, key text NOT NULL, value text NOT NULL DEFAULT '', created_at bigint NOT NULL DEFAULT 0, updated_at bigint NOT NULL DEFAULT 0, PRIMARY KEY(job_id, key));
CREATE TABLE IF NOT EXISTS job_steps (job_id text NOT NULL, step_name text NOT NULL, status text NOT NULL DEFAULT '', recoverable boolean NOT NULL DEFAULT false, retryable boolean NOT NULL DEFAULT false, error_message text NOT NULL DEFAULT '', result_json text NOT NULL DEFAULT '', started_at bigint NOT NULL DEFAULT 0, completed_at bigint NOT NULL DEFAULT 0, created_at bigint NOT NULL DEFAULT 0, updated_at bigint NOT NULL DEFAULT 0, PRIMARY KEY(job_id, step_name));
CREATE INDEX IF NOT EXISTS idx_job_steps_status ON job_steps(status);
CREATE TABLE IF NOT EXISTS job_events (event_id bigserial PRIMARY KEY, job_id text NOT NULL DEFAULT '', event_type text NOT NULL DEFAULT '', snapshot_json text NOT NULL DEFAULT '', created_at bigint NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS idx_job_events_job_id ON job_events(job_id);
CREATE INDEX IF NOT EXISTS idx_job_events_event_type ON job_events(event_type);
CREATE TABLE IF NOT EXISTS gpt_runtime_settings (settings_key text PRIMARY KEY, value_json text NOT NULL DEFAULT '', created_at bigint NOT NULL DEFAULT 0, updated_at bigint NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS codex_oauth_phone_leases (activation_id text PRIMARY KEY, phone_e164 text NOT NULL DEFAULT '', phone_national text NOT NULL DEFAULT '', country_iso2 text NOT NULL DEFAULT '', country_calling_code text NOT NULL DEFAULT '', profile_key text NOT NULL DEFAULT '', status text NOT NULL DEFAULT '', label text NOT NULL DEFAULT '', use_count integer NOT NULL DEFAULT 0, max_use_count integer NOT NULL DEFAULT 0, expires_at bigint NOT NULL DEFAULT 0, last_failure_kind text NOT NULL DEFAULT '', last_job_id text NOT NULL DEFAULT '', last_account_id text NOT NULL DEFAULT '', last_error text NOT NULL DEFAULT '', created_at bigint NOT NULL DEFAULT 0, updated_at bigint NOT NULL DEFAULT 0);
ALTER TABLE codex_oauth_phone_leases ADD COLUMN IF NOT EXISTS expires_at bigint NOT NULL DEFAULT 0;
ALTER TABLE codex_oauth_phone_leases ADD COLUMN IF NOT EXISTS last_failure_kind text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_phone_e164 ON codex_oauth_phone_leases(phone_e164);
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_country_iso2 ON codex_oauth_phone_leases(country_iso2);
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_profile_key ON codex_oauth_phone_leases(profile_key);
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_status ON codex_oauth_phone_leases(status);
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_label ON codex_oauth_phone_leases(label);
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_expires_at ON codex_oauth_phone_leases(expires_at);
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_last_failure_kind ON codex_oauth_phone_leases(last_failure_kind);
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_last_job_id ON codex_oauth_phone_leases(last_job_id);
CREATE INDEX IF NOT EXISTS idx_codex_oauth_phone_leases_last_account_id ON codex_oauth_phone_leases(last_account_id);

CREATE TABLE IF NOT EXISTS account_browser_fingerprints (
  account_id text PRIMARY KEY,
  country_code text NOT NULL DEFAULT '',
  region text NOT NULL DEFAULT '',
  browser_profile_template text NOT NULL DEFAULT '',
  browser_family text NOT NULL DEFAULT '',
  browser_major_version text NOT NULL DEFAULT '',
  os_family text NOT NULL DEFAULT '',
  tls_profile_family text NOT NULL DEFAULT '',
  tls_fingerprint_variant text NOT NULL DEFAULT '',
  locale text NOT NULL DEFAULT '',
  timezone text NOT NULL DEFAULT '',
  user_agent text NOT NULL DEFAULT '',
  accept_language text NOT NULL DEFAULT '',
  language text NOT NULL DEFAULT '',
  device_id text NOT NULL DEFAULT '',
  created_at bigint NOT NULL DEFAULT 0,
  updated_at bigint NOT NULL DEFAULT 0
);
ALTER TABLE account_browser_fingerprints ADD COLUMN IF NOT EXISTS country_code text NOT NULL DEFAULT '';
ALTER TABLE account_browser_fingerprints ADD COLUMN IF NOT EXISTS region text NOT NULL DEFAULT '';
ALTER TABLE account_browser_fingerprints ADD COLUMN IF NOT EXISTS browser_profile_template text NOT NULL DEFAULT '';
ALTER TABLE account_browser_fingerprints ADD COLUMN IF NOT EXISTS browser_family text NOT NULL DEFAULT '';
ALTER TABLE account_browser_fingerprints ADD COLUMN IF NOT EXISTS browser_major_version text NOT NULL DEFAULT '';
ALTER TABLE account_browser_fingerprints ADD COLUMN IF NOT EXISTS os_family text NOT NULL DEFAULT '';
ALTER TABLE account_browser_fingerprints ADD COLUMN IF NOT EXISTS tls_profile_family text NOT NULL DEFAULT '';
ALTER TABLE account_browser_fingerprints ADD COLUMN IF NOT EXISTS tls_fingerprint_variant text NOT NULL DEFAULT '';
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'account_browser_fingerprints' AND column_name = 'fingerprint_selector') THEN
    EXECUTE $sql$UPDATE account_browser_fingerprints SET browser_profile_template = COALESCE(NULLIF(browser_profile_template, ''), NULLIF(fingerprint_selector, ''), '') WHERE browser_profile_template = ''$sql$;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'account_browser_fingerprints' AND column_name = 'tls_profile_name') THEN
    EXECUTE $sql$UPDATE account_browser_fingerprints SET tls_profile_family = COALESCE(NULLIF(tls_profile_family, ''), NULLIF(tls_profile_name, ''), '') WHERE tls_profile_family = ''$sql$;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'account_browser_fingerprints' AND column_name = 'os_alias') THEN
    EXECUTE $sql$UPDATE account_browser_fingerprints SET os_family = COALESCE(NULLIF(os_family, ''), NULLIF(os_alias, ''), NULLIF(platform, ''), '') WHERE os_family = ''$sql$;
  END IF;
END $$;
ALTER TABLE account_browser_fingerprints DROP COLUMN IF EXISTS proxy_ref;
ALTER TABLE account_browser_fingerprints DROP COLUMN IF EXISTS fingerprint_selector;
ALTER TABLE account_browser_fingerprints DROP COLUMN IF EXISTS tls_profile_name;
ALTER TABLE account_browser_fingerprints DROP COLUMN IF EXISTS platform;
ALTER TABLE account_browser_fingerprints DROP COLUMN IF EXISTS os_alias;
ALTER TABLE account_browser_fingerprints DROP COLUMN IF EXISTS sec_ch_ua;
ALTER TABLE account_browser_fingerprints DROP COLUMN IF EXISTS sec_ch_platform;
DROP INDEX IF EXISTS idx_account_browser_fingerprints_proxy_ref;
DROP INDEX IF EXISTS idx_account_browser_fingerprints_selector;
CREATE INDEX IF NOT EXISTS idx_account_browser_fingerprints_template ON account_browser_fingerprints(browser_profile_template);
CREATE INDEX IF NOT EXISTS idx_account_browser_fingerprints_tls_family ON account_browser_fingerprints(tls_profile_family);
CREATE INDEX IF NOT EXISTS idx_account_browser_fingerprints_tls_variant ON account_browser_fingerprints(tls_fingerprint_variant);
CREATE INDEX IF NOT EXISTS idx_account_browser_fingerprints_country_code ON account_browser_fingerprints(country_code);

UPDATE account_browser_fingerprints
SET country_code = CASE timezone
  WHEN 'Asia/Tokyo' THEN 'JP'
  WHEN 'Asia/Jakarta' THEN 'ID'
  WHEN 'Asia/Bangkok' THEN 'TH'
  WHEN 'Asia/Singapore' THEN 'SG'
  WHEN 'America/Los_Angeles' THEN 'US'
  WHEN 'America/Chicago' THEN 'US'
  WHEN 'America/New_York' THEN 'US'
  ELSE country_code
END,
region = CASE timezone
  WHEN 'Asia/Tokyo' THEN 'JP-13'
  WHEN 'Asia/Jakarta' THEN 'ID-JK'
  WHEN 'Asia/Bangkok' THEN 'TH-10'
  WHEN 'Asia/Singapore' THEN 'SG-01'
  WHEN 'America/Los_Angeles' THEN 'US-CA'
  WHEN 'America/Chicago' THEN 'US-TX'
  WHEN 'America/New_York' THEN 'US-NY'
  ELSE region
END
WHERE country_code = '' OR region = '';

UPDATE account_browser_fingerprints
SET locale = 'en-US',
    accept_language = 'en-US,en;q=0.9',
    language = 'en-US',
    updated_at = EXTRACT(EPOCH FROM now())::bigint
WHERE locale <> 'en-US'
   OR accept_language <> 'en-US,en;q=0.9'
   OR language <> 'en-US';
DROP TABLE IF EXISTS gpt_otp_events;
DROP TABLE IF EXISTS gpt_mailbox_messages;
DROP TABLE IF EXISTS gpt_mailbox_email_events;
CREATE TABLE IF NOT EXISTS gpt_platform_event_outbox (event_id TEXT PRIMARY KEY, subject TEXT NOT NULL, event_name TEXT NOT NULL, idempotency_key TEXT NOT NULL DEFAULT '', envelope BYTEA NOT NULL, status TEXT NOT NULL DEFAULT 'PENDING', attempt_count INT NOT NULL DEFAULT 0, next_attempt_at BIGINT NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', published_at BIGINT NOT NULL DEFAULT 0, created_at BIGINT NOT NULL, updated_at BIGINT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_gpt_platform_event_outbox_pending ON gpt_platform_event_outbox(status, next_attempt_at, created_at);

CREATE TABLE IF NOT EXISTS account_proxy_usages (
  id text PRIMARY KEY,
  account_id text NOT NULL DEFAULT '',
  job_id text NOT NULL DEFAULT '',
  n8n_execution_id text NOT NULL DEFAULT '',
  purpose text NOT NULL DEFAULT '',
  proxy_url_hash text NOT NULL DEFAULT '',
  session_id_hash text NOT NULL DEFAULT '',
  exit_ip text NOT NULL DEFAULT '',
  country_code text NOT NULL DEFAULT '',
  region text NOT NULL DEFAULT '',
  city text NOT NULL DEFAULT '',
  attempt_index integer NOT NULL DEFAULT 0,
  accepted boolean NOT NULL DEFAULT false,
  error_message text NOT NULL DEFAULT '',
  raw_json text NOT NULL DEFAULT '',
  created_at bigint NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_account_id ON account_proxy_usages(account_id);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_job_id ON account_proxy_usages(job_id);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_n8n_execution_id ON account_proxy_usages(n8n_execution_id);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_purpose ON account_proxy_usages(purpose);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_proxy_url_hash ON account_proxy_usages(proxy_url_hash);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_session_id_hash ON account_proxy_usages(session_id_hash);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_exit_ip ON account_proxy_usages(exit_ip);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_country_code ON account_proxy_usages(country_code);
CREATE INDEX IF NOT EXISTS idx_account_proxy_usages_accepted ON account_proxy_usages(accepted);
DROP INDEX IF EXISTS idx_account_proxy_usages_network_kind;
DROP INDEX IF EXISTS idx_account_proxy_usages_anonymizer_kind;
DROP INDEX IF EXISTS idx_account_proxy_usages_fraud_risk_level;
DROP INDEX IF EXISTS idx_account_proxy_usages_edge_risk_level;
ALTER TABLE account_proxy_usages DROP COLUMN IF EXISTS network_kind, DROP COLUMN IF EXISTS anonymizer_kind, DROP COLUMN IF EXISTS fraud_risk_level, DROP COLUMN IF EXISTS fraud_risk_score, DROP COLUMN IF EXISTS edge_risk_level, DROP COLUMN IF EXISTS edge_risk_score, DROP COLUMN IF EXISTS proxy_protocol, DROP COLUMN IF EXISTS proxy_host, DROP COLUMN IF EXISTS proxy_port;
