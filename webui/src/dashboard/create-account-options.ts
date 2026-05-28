import { faker } from '@faker-js/faker';
import { AccountEmailStrategy } from '../proto/orchestrator_account';
import type { SelectFieldOption } from '@byte-v-forge/common-ui';
import type { MailboxDomain, MailboxProviderCapability } from './types';

export type MailboxChoice = 'domain' | 'independent' | 'alias' | 'manual';
export type CreateAccountValues = {
  provider_key: string;
  mailbox_choice: MailboxChoice;
  email: string;
  password: string;
  local: string;
  domain: string;
  country_code: string;
  region: string;
};

export function mailboxProviderOptions(capabilities: MailboxProviderCapability[], domains: MailboxDomain[]): SelectFieldOption[] {
  const providers = new Map<string, string>();
  for (const provider of capabilities) {
    const key = normalizeProviderKey(provider.key);
    if (!providerEnabled(key, domains)) continue;
    providers.set(key, provider.display_name || provider.key);
  }
  for (const domain of domains) {
    const key = normalizeProviderKey(domain.provider_key);
    if (!key || domain.enabled === false) continue;
    providers.set(key, providers.get(key) || labelForProvider(key));
  }
  providers.set('manual', '手动输入');
  return Array.from(providers)
    .sort(([left], [right]) => providerRank(left) - providerRank(right) || left.localeCompare(right))
    .map(([value, label]) => ({ value, label }));
}

export function mailboxChoiceOptions(providerKey: string, domains: MailboxDomain[]): SelectFieldOption[] {
  const key = normalizeProviderKey(providerKey);
  if (key === 'cloudflare') {
    return [{ value: 'domain', label: '域名邮箱', disabled: domainsForProvider(domains, key).length === 0 }];
  }
  if (key === 'outlook') {
    return [
      { value: 'independent', label: '独立邮箱' },
      { value: 'alias', label: '别名邮箱' },
    ];
  }
  return [{ value: 'manual', label: '手动邮箱' }];
}

export function domainsForProvider(domains: MailboxDomain[], providerKey: string) {
  const key = normalizeProviderKey(providerKey);
  return Array.from(new Set(domains
    .filter((domain) => domain.enabled !== false)
    .filter((domain) => normalizeProviderKey(domain.provider_key) === key)
    .map((domain) => normalizeDomain(domain.domain))
    .filter(Boolean)));
}

export function accountEmail(values: CreateAccountValues, activeDomain: string) {
  if (values.mailbox_choice === 'manual') return values.email.trim();
  if (values.mailbox_choice !== 'domain') return '';
  const domain = normalizeDomain(activeDomain);
  if (!domain) return '';
  return `${localPart(values.local)}@${domain}`;
}

export function emailStrategyForValues(values: CreateAccountValues) {
  if (values.mailbox_choice === 'independent') return AccountEmailStrategy.ACCOUNT_EMAIL_STRATEGY_POOLED_PRIMARY;
  if (values.mailbox_choice === 'alias') return AccountEmailStrategy.ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS;
  return AccountEmailStrategy.ACCOUNT_EMAIL_STRATEGY_EXPLICIT;
}

export function defaultMailboxChoice(providerKey: string, domains: MailboxDomain[]): MailboxChoice {
  const first = mailboxChoiceOptions(providerKey, domains).find((option) => !option.disabled)?.value;
  return (first || 'manual') as MailboxChoice;
}

function providerEnabled(providerKey: string, domains: MailboxDomain[]) {
  if (providerKey === 'cloudflare') return domainsForProvider(domains, providerKey).length > 0;
  return providerKey === 'outlook';
}

function localPart(value: string) {
  return (value.trim().split('@')[0] || randomLocalPart()).toLowerCase();
}

function randomLocalPart() {
  const firstName = localPartToken(faker.person.firstName());
  const lastName = localPartToken(faker.person.lastName());
  const suffix = faker.number.int({ min: 100, max: 9999 });
  return compactLocalPart([firstName, lastName, String(suffix)]);
}

function compactLocalPart(parts: string[]) {
  const value = parts.filter(Boolean).join('').replace(/[^a-z0-9]/g, '');
  return value || faker.string.alphanumeric(10).toLowerCase();
}

function localPartToken(value: string) {
  return value
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-zA-Z0-9]/g, '')
    .toLowerCase();
}

function normalizeProviderKey(value: string) {
  const key = value.trim().toLowerCase();
  if (key === 'cf' || key === 'cloudflare-email-relay') return 'cloudflare';
  return key;
}

function normalizeDomain(value: string) {
  return value.trim().toLowerCase().replace(/^@+/, '').replace(/^\.+|\.+$/g, '');
}

function labelForProvider(providerKey: string) {
  if (providerKey === 'cloudflare') return 'Cloudflare';
  if (providerKey === 'outlook') return 'Outlook';
  return providerKey;
}

function providerRank(providerKey: string) {
  if (providerKey === 'cloudflare') return 0;
  if (providerKey === 'outlook') return 1;
  if (providerKey === 'manual') return 99;
  return 50;
}
