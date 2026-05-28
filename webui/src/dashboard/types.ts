import type { Account, GPTEmailAllocation } from '../proto/gpt_account';
import type { FetchAccountMailboxResponse as ProtoFetchAccountMailboxResponse } from '../proto/orchestrator_account';
import type { InboxMessage, InboxResponse, InboxResult, LatestOtp, Mailbox, MailboxDomain, MailboxProviderCapability } from '@byte-v-forge/common-ui';
import type { Job, JobSnapshot, WorkflowProgress } from '../proto/orchestrator_job';
import type { GoPayUserStatusResponse } from '../proto/orchestrator_gopay_app';

export type FetchAccountMailboxResponse = Omit<ProtoFetchAccountMailboxResponse, 'inbox'> & { inbox?: InboxResponse };

export type { Account, GoPayUserStatusResponse, GPTEmailAllocation, InboxMessage, InboxResponse, InboxResult, Job, JobSnapshot, LatestOtp, Mailbox, MailboxDomain, MailboxProviderCapability, WorkflowProgress };

export type GoPayDashboardStateResponse = GoPayUserStatusResponse & {
  user_id: string;
  wa_phone: string;
  wa_phone_error_message?: string;
};

export type AccountMailboxContext = {
  account_email: string;
  primary_email: string;
  provider_key: string;
  is_split: boolean;
  known: boolean;
};

export type GoPayOTPChannel = '' | 'sms' | 'wa';
export type GoPayPaymentChannel = GoPayOTPChannel | 'app_wa';
export type ConcreteGoPayPaymentChannel = Exclude<GoPayPaymentChannel, ''>;
export type GoPayAddBalanceMethod = '' | 'manual_transfer' | 'envelope' | 'rekberinaja';
export type ConcreteGoPayAddBalanceMethod = Exclude<GoPayAddBalanceMethod, ''>;
export type DisplayLabelMap = Record<string, string>;

export type AccountBrowserFingerprint = {
  account_id: string;
  browser_profile_template: string;
  browser_family: string;
  browser_major_version: string;
  os_family: string;
  tls_profile_family: string;
  tls_fingerprint_variant: string;
  locale: string;
  timezone: string;
  user_agent: string;
  accept_language: string;
  language: string;
  device_id: string;
  created_at?: number;
  updated_at?: number;
};


export type AccountProxyChain = {
  chain_id: string;
  line_source_id: string;
  line_node_id: string;
  line_display_name: string;
  dynamic_provider_account_id: string;
  dynamic_provider_id: string;
  dynamic_gateway_id: string;
  dynamic_gateway_name: string;
};

export type AccountProxyUsage = {
  id: string;
  job_id: string;
  n8n_execution_id: string;
  purpose: string;
  proxy_url_hash: string;
  proxy_protocol: string;
  proxy_host: string;
  proxy_port: number;
  session_id_hash: string;
  exit_ip: string;
  country_code: string;
  region: string;
  city: string;
  network_kind: string;
  anonymizer_kind: string;
  fraud_risk_level: string;
  fraud_risk_score: number;
  edge_risk_level: string;
  edge_risk_score: number;
  attempt_index: number;
  accepted: boolean;
  error_message: string;
  created_at: number;
  chain?: AccountProxyChain;
};
