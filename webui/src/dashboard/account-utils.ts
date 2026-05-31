import { statusText } from './labels';
import type { Account } from './types';
import { accountCarrierEmail, accountCarrierStatusValue } from '@byte-v-forge/common-ui';

const INVALID_GPT_ACCOUNT_STATUS = 'DEACTIVATED';

export function isInvalidGptAccount(account: Account) { return accountCarrierStatusValue(account) === INVALID_GPT_ACCOUNT_STATUS; }

export function canRegister(account: Account) {
  return !isInvalidGptAccount(account) && !isUserAlreadyExistsAccount(account) && !hasRegisteredSession(account);
}

export function canProbeAccount(account: Account) {
  return !isInvalidGptAccount(account) && !isUserAlreadyExistsAccount(account) && hasRegisteredSession(account);
}

export function probeAccountHint(account: Account) {
  if (normalizeTier(account.tier) === 'plus' || account.plus_active) return '已是 Plus，直接探测 Tier';
  if (account.plus_trial_eligible !== undefined && account.plus_trial_eligible !== null) return '资格已探测，直接探测 Tier';
  return '先探测 Plus 资格，再探测 Tier';
}

export function canUpdateWebAccessToken(account: Account) {
  return !isInvalidGptAccount(account) && !isUserAlreadyExistsAccount(account) && hasRegisteredSession(account);
}

export function canLoginSession(account: Account) {
  return !isInvalidGptAccount(account) && !isUserAlreadyExistsAccount(account) && !!accountCarrierEmail(account);
}

export function loginActionLabel(account: Account) {
  void account;
  return '登录更新认证';
}

export function loginActionHint(account: Account) {
  void account;
  return '通过账号密码登录并更新 Redis 中的 Session Token / Web AT';
}

export function accountSignalText(account: Account) {
  if (isInvalidGptAccount(account)) return '失效';
  if (accountCarrierStatusValue(account).includes('FAILED') || accountCarrierStatusValue(account).includes('EXISTS')) return statusText(accountCarrierStatusValue(account));
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
  if (signal === '不可试用' || signal === '失效' || accountCarrierStatusValue(account).includes('FAILED') || accountCarrierStatusValue(account).includes('EXISTS')) return 'bad';
  return 'mid';
}

export function accountIsActivated(account: Account) {
  if (isInvalidGptAccount(account)) return false;
  const tier = normalizeTier(account.tier);
  return accountCarrierStatusValue(account) === 'ACTIVATED' || account.plus_active === true || (!!tier && tier !== 'free' && tier !== 'unknown');
}

export function accountActivationText(account: Account) {
  if (isInvalidGptAccount(account)) return statusText(accountCarrierStatusValue(account));
  if (accountIsActivated(account)) return '已激活';
  return statusText(accountCarrierStatusValue(account));
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
  return accountCarrierStatusValue(account) === 'USER_ALREADY_EXISTS' || accountCarrierStatusValue(account) === 'EMAIL_ALREADY_EXISTS';
}

export function hasRegisteredSession(account: Account) {
  return accountCarrierStatusValue(account) === 'REGISTERED' || accountCarrierStatusValue(account) === 'ACTIVATED';
}

export function normalizeTier(tier: string) {
  return String(tier || '').trim().toLowerCase();
}

function trialText(value?: boolean) {
  if (value === true) return '可试用';
  if (value === false) return '不可试用';
  return '未探测';
}
