import { accountCarrierID, api, fetchAccountList, forEachCursorPageItem } from '@byte-v-forge/common-ui';
import { isInvalidGptAccount } from './account-utils';
import type { ListAccountsResponse } from '../proto/gpt_account';
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
  return api(`/api/gpt/accounts/${accountID}`, { method: 'DELETE' });
}

export async function cleanInvalidGptAccounts() {
  const deleted: Account[] = [];
  await forEachCursorPageItem<Account, ListAccountsResponse, 'accounts'>({
    field: 'accounts',
    maxPages: CLEANUP_MAX_BATCHES,
    queryFn: loadInvalidAccountsBatch,
    onItem: async (account) => {
      await deleteGptAccount(accountCarrierID(account));
      deleted.push(account);
    }
  });
  return deleted;
}

async function loadInvalidAccountsBatch(cursor: string) {
  return fetchAccountList<Account, ListAccountsResponse>({ path: '/api/gpt/accounts', cursor, limit: CLEANUP_BATCH_LIMIT, params: { status: 'DEACTIVATED' } });
}
