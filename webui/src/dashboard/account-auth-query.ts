import { accountQueryKey, accountQueryPrefix, api } from '@byte-v-forge/common-ui';
import type { AccountAuthResponse as AccountAuthTokens } from '../proto/orchestrator_account';

export type { AccountAuthTokens };

export const accountAuthQueryPrefix = accountQueryPrefix('gpt', 'account-auth');
export const accountAuthQueryKey = (accountID: string) => accountQueryKey(accountAuthQueryPrefix, accountID);

export function loadAccountAuthTokens(accountID: string) {
  return api<AccountAuthTokens>(`/api/gpt/accounts/${encodeURIComponent(accountID)}/auth`);
}
