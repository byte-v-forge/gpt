import { accountActionDefinition, accountActionHasCapability, accountCarrierCredentialState, numberValue, stringValue } from '@byte-v-forge/common-ui';
import { GPT_CAPABILITIES, type GptActionCatalog, type GptCapability } from './action-catalog';
import { jobDataObject } from './job-data';
import type { Account, Job } from './types';
import { accountCarrierID } from '@byte-v-forge/common-ui';

const CREDENTIAL_KIND_CODEX_PHONE = 'codex_phone';

export type AccountCodexPhoneState = {
  confirmed: boolean;
  label: string;
  title: string;
  tone: 'good' | 'neutral' | 'bad';
};

export function accountActivationChannel(account: Account, jobs: Job[], catalog?: GptActionCatalog) {
  const direct = String(account.activation_channel || '').trim();
  if (direct) return direct;
  const latest = jobs
    .filter((job) => job.account_id === accountCarrierID(account) && isActivationJob(job, catalog))
    .sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0))[0];
  return latest ? String(accountActionDefinition(catalog, latest.action)?.display_name || latest.action || '-') : '-';
}

export function accountCodexPhoneState(account: Account, jobs: Job[], catalog?: GptActionCatalog): AccountCodexPhoneState {
  const credential = accountCodexPhoneCredential(account);
  const status = normalizeCodexPhoneStatus(credential?.status);
  if (status === 'CONFIRMED' || credential?.present === true) return codexPhoneState(true);
  const protocol = latestProtocolPhoneState(account, jobs, catalog);
  if (protocol) return protocol;
  if (status === 'OAUTH_NEED_PHONE') return oauthNeedPhoneState();
  if (credential?.present === false) return codexPhoneState(false);
  return latestAddPhoneState(account, jobs, catalog) || { confirmed: false, label: '未加手机', title: '未确认 add phone', tone: 'neutral' };
}

function accountCodexPhoneCredential(account: Account) {
  return accountCarrierCredentialState(account, CREDENTIAL_KIND_CODEX_PHONE);
}

function latestAddPhoneState(account: Account, jobs: Job[], catalog?: GptActionCatalog): AccountCodexPhoneState | null {
  const latest = jobs
    .filter((job) => job.account_id === accountCarrierID(account) && jobHasCapability(job, catalog, GPT_CAPABILITIES.phoneBinding))
    .sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0));
  for (const job of latest) {
    const result = jobDataObject(job.result);
    const confirmed = boolResult(result.add_phone_confirmed);
    if (confirmed === true) return codexPhoneState(true, stringValue(result.phone_label) || stringValue(result.label), numberValue(result.phone_reuse_count), numberValue(result.phone_reuse_limit));
    if (job.status === 'SUCCEEDED' && confirmed === false) return { confirmed: false, label: '未加手机', title: 'OAuth 已完成，但该账号未出现 add phone', tone: 'neutral' };
  }
  return null;
}

function latestProtocolPhoneState(account: Account, jobs: Job[], catalog?: GptActionCatalog): AccountCodexPhoneState | null {
  const latest = jobs
    .filter((job) => job.account_id === accountCarrierID(account) && jobHasCapability(job, catalog, GPT_CAPABILITIES.codexOAuth) && !jobHasCapability(job, catalog, GPT_CAPABILITIES.phoneBinding))
    .sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0))[0];
  const result = jobDataObject(latest?.result);
  if (boolResult(result.client_auth_phone_present) === true) return codexPhoneState(true, stringValue(result.client_auth_phone_verification_channel) || 'dump');
  if (boolResult(result.add_phone_required) === true || stringValue(result.login_stage) === 'add_phone') return oauthNeedPhoneState(stringValue(result.phone_label) || stringValue(result.label));
  return null;
}

function isActivationJob(job: Job, catalog?: GptActionCatalog) {
  return jobHasCapability(job, catalog, GPT_CAPABILITIES.activation) || jobHasCapability(job, catalog, GPT_CAPABILITIES.payment);
}

function jobHasCapability(job: Job, catalog: GptActionCatalog | undefined, capability: GptCapability) {
  return accountActionHasCapability(catalog, job.action, capability);
}

function codexPhoneState(confirmed: boolean, label = '', reuseCount = 0, reuseLimit = 0): AccountCodexPhoneState {
  const suffix = label ? ` · ${label}` : '';
  const reuse = reuseLimit > 0 ? ` · ${reuseCount || 0}/${reuseLimit}` : '';
  return { confirmed, label: confirmed ? '已加手机' : '未加手机', title: `${confirmed ? '已完成 add phone' : '未确认 add phone'}${suffix}${reuse}`, tone: confirmed ? 'good' : 'neutral' };
}

function oauthNeedPhoneState(label = ''): AccountCodexPhoneState {
  const suffix = label ? ` · ${label}` : '';
  return { confirmed: false, label: 'OAuth Need Phone', title: `OAuth 需要加手机号${suffix}`, tone: 'bad' };
}

function normalizeCodexPhoneStatus(value: unknown) {
  return String(value ?? '').trim().toUpperCase().replace(/[\s-]+/g, '_');
}

function boolResult(value: unknown) {
  if (value === true || value === false) return value;
  const normalized = String(value ?? '').trim().toLowerCase();
  if (['true', '1', 'yes'].includes(normalized)) return true;
  if (['false', '0', 'no'].includes(normalized)) return false;
  return null;
}
