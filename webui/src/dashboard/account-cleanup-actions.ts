import { api } from '@/dashboard/module-kit';
import { isInvalidGptAccount } from './account-utils';
import type { Account } from './types';

const CLEANUP_BATCH_LIMIT = 500;
const CLEANUP_MAX_BATCHES = 20;

export function invalidAccountsForCleanup(accounts: Account[]) {
  return accounts.filter(isInvalidAccountForCleanup);
}

export function isInvalidAccountForCleanup(account: Account) {
  return isInvalidGptAccount(account);
}

export async function deleteGptAccount(accountID: string) {
  return api(`/api/accounts/${accountID}`, { method: 'DELETE' });
}

export async function cleanInvalidGptAccounts() {
  const deleted: Account[] = [];
  for (let batch = 0; batch < CLEANUP_MAX_BATCHES; batch += 1) {
    const accounts = await loadInvalidAccountsBatch();
    if (!accounts.length) break;
    for (const account of accounts) {
      await deleteGptAccount(account.account_id);
      deleted.push(account);
    }
    if (accounts.length < CLEANUP_BATCH_LIMIT) break;
  }
  return deleted;
}

function loadInvalidAccountsBatch() {
  return api<Account[]>(`/api/accounts?status=DEACTIVATED&limit=${CLEANUP_BATCH_LIMIT}`);
}
