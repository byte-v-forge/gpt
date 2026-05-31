import type { Account } from './types';

export function requireAccount(account?: Account) {
  if (!account) throw new Error('account response is empty');
  return account;
}
