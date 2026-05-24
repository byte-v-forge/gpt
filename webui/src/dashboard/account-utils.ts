import { numberValue, objectValue, stringValue } from '@/dashboard/module-kit';
import { goPayPaymentChannelLabel, paymentChannelValue } from './gopay-utils';
import { statusText } from './labels';
import type { Account, Job } from './types';

export function canRegister(account: Account) {
  return !isUserAlreadyExistsAccount(account) && !hasRegisteredSession(account);
}

export function canGoPayPayment(account: Account) {
  const tier = normalizeTier(account.tier);
  return !isUserAlreadyExistsAccount(account) &&
    account.status !== 'ACTIVATED' &&
    !account.plus_active &&
    account.plus_trial_eligible !== false &&
    (tier === '' || tier === 'free') &&
    (!!account.session_token || !!account.access_token);
}

export function accountActivationChannel(account: Account, jobs: Job[]) {
  const direct = goPayPaymentChannelLabel(paymentChannelValue(account.activation_channel || ''));
  if (direct !== '-') return direct;
  const latestPaymentJob = jobs
    .filter((job) => job.account_id === account.account_id && ['GOPAY_PAYMENT', 'GOPAY_QRIS_PAYMENT_ACTIVATE', 'GOPAY_WA_PAYMENT', 'ACTIVATE', 'AUTOPAY', 'REGISTER_AND_ACTIVATE'].includes(job.action))
    .sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0))[0];
  if (!latestPaymentJob) return '-';
  if (latestPaymentJob.action === 'GOPAY_QRIS_PAYMENT_ACTIVATE') return 'QRIS';
  if (latestPaymentJob.action === 'GOPAY_WA_PAYMENT') return '纯Gopay-WA';
  return goPayPaymentChannelLabel(paymentChannelValue(stringValue(objectValue(latestPaymentJob.result).otp_channel)));
}


export type AccountCodexPhoneState = {
  confirmed: boolean;
  label: string;
  title: string;
  tone: 'good' | 'neutral' | 'bad';
};

export function accountCodexPhoneState(account: Account, jobs: Job[]): AccountCodexPhoneState {
  const accountState = account as Account & { codex_phone_confirmed?: boolean; codex_phone_label?: string; codex_phone_status?: string };
  const status = normalizeCodexPhoneStatus(accountState.codex_phone_status);
  if (status === 'CONFIRMED' || accountState.codex_phone_confirmed === true) return codexPhoneState(true, stringValue(accountState.codex_phone_label));
  const protocol = latestProtocolPhoneState(account, jobs);
  if (protocol) return protocol;
  if (status === 'OAUTH_NEED_PHONE') return oauthNeedPhoneState(stringValue(accountState.codex_phone_label));
  if (accountState.codex_phone_confirmed === false) return codexPhoneState(false, stringValue(accountState.codex_phone_label));
  const latest = jobs
    .filter((job) => job.account_id === account.account_id && job.action === 'CODEX_OAUTH_ADD_PHONE')
    .sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0));
  for (const job of latest) {
    const result = objectValue(job.result);
    const confirmed = boolResult(result.add_phone_confirmed);
    if (confirmed === true) {
      return codexPhoneState(true, stringValue(result.phone_label) || stringValue(result.label), numberValue(result.phone_reuse_count), numberValue(result.phone_reuse_limit));
    }
    if (job.status === 'SUCCEEDED' && confirmed === false) {
      return { confirmed: false, label: '未加手机', title: 'OAuth 已完成，但该账号未出现 add phone', tone: 'neutral' };
    }
  }
  return { confirmed: false, label: '未加手机', title: '未确认 add phone', tone: 'neutral' };
}


function latestProtocolPhoneState(account: Account, jobs: Job[]): AccountCodexPhoneState | null {
  const latest = jobs
    .filter((job) => job.account_id === account.account_id && ['CODEX_OAUTH_PROTOCOL', 'CODEX_OAUTH'].includes(job.action))
    .sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0))[0];
  const result = objectValue(latest?.result);
  if (boolResult(result.client_auth_phone_present) === true) {
    return codexPhoneState(true, stringValue(result.client_auth_phone_verification_channel) || 'dump');
  }
  if (boolResult(result.add_phone_required) === true || stringValue(result.login_stage) === 'add_phone') return oauthNeedPhoneState(stringValue(result.phone_label) || stringValue(result.label));
  return null;
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

export function canProbeAccount(account: Account) {
  return !isUserAlreadyExistsAccount(account) && !!account.session_token;
}

export function probeAccountHint(account: Account) {
  if (normalizeTier(account.tier) === 'plus' || account.plus_active) return '已是 Plus，直接探测 Tier';
  if (account.plus_trial_eligible !== undefined && account.plus_trial_eligible !== null) return '资格已探测，直接探测 Tier';
  return '先探测 Plus 资格，再探测 Tier';
}

export function canRefreshAccessToken(account: Account) {
  return !isUserAlreadyExistsAccount(account) && !!account.session_token && !account.access_token;
}

export function canLoginSession(account: Account) {
  return !isUserAlreadyExistsAccount(account) && !!account.email && !!account.password;
}

export function loginActionLabel(account: Account) {
  if (!account.session_token) return '登录获取 Session';
  if (!account.access_token) return '登录刷新 Access Token';
  return '登录刷新 Token';
}

export function loginActionHint(account: Account) {
  if (!account.session_token) return '通过账号密码登录并获取 Session Token';
  if (!account.access_token) return '重新登录并刷新 Access Token';
  return '重新登录并刷新 Session / Access Token';
}

export function accountSignalText(account: Account) {
  if (account.status === 'DEACTIVATED') return '失效';
  if (account.status.includes('FAILED') || account.status.includes('EXISTS')) return statusText(account.status);
  if (accountIsActivated(account)) {
    const plan = accountPlanText(account);
    return plan === '未知' || plan === 'Free' ? 'Plus' : plan;
  }
  if (account.plus_trial_eligible === true) return '可试用';
  if (account.plus_trial_eligible === false) return '不可试用';
  return '未探测';
}

export function accountSignalTone(account: Account) {
  const signal = accountSignalText(account);
  if (['Plus', 'Pro', 'Team', 'Enterprise', '可试用'].includes(signal)) return 'good';
  if (signal === '不可试用' || signal === '失效' || account.status.includes('FAILED') || account.status.includes('EXISTS')) return 'bad';
  return 'mid';
}

export function accountIsActivated(account: Account) {
  if (account.status === 'DEACTIVATED') return false;
  const tier = normalizeTier(account.tier);
  return account.status === 'ACTIVATED' || account.plus_active === true || (!!tier && tier !== 'free' && tier !== 'unknown');
}

export function accountActivationText(account: Account) {
  if (account.status === 'DEACTIVATED') return statusText(account.status);
  if (accountIsActivated(account)) return '已激活';
  return statusText(account.status);
}

export function accountPlanText(account: Account) {
  const tier = normalizeTier(account.tier);
  if (!tier || tier === 'unknown') return account.plus_active ? 'Plus' : '未知';
  if (tier === 'free') return 'Free';
  if (tier === 'plus') return 'Plus';
  if (tier === 'pro') return 'Pro';
  if (tier === 'team') return 'Team';
  if (tier === 'enterprise') return 'Enterprise';
  return tier;
}

export function accountEligibilityText(account: Account) {
  return accountIsActivated(account) ? '已激活' : trialText(account.plus_trial_eligible);
}

export function tierEligibilityText(account: Account) {
  return accountSignalText(account);
}

export function tierText(tier: string) {
  return normalizeTier(tier) || '未知';
}

export function isUserAlreadyExistsAccount(account: Account) {
  return account.status === 'USER_ALREADY_EXISTS' || account.status === 'EMAIL_ALREADY_EXISTS';
}

export function hasRegisteredSession(account: Account) {
  return account.status === 'REGISTERED' || account.status === 'ACTIVATED' || !!account.session_token || !!account.access_token;
}

export function normalizeTier(tier: string) {
  return String(tier || '').trim().toLowerCase();
}

function trialText(value?: boolean) {
  if (value === true) return '可试用';
  if (value === false) return '不可试用';
  return '未探测';
}
