import { api } from '@byte-v-forge/common-ui';

export type AccountAuthTokens = {
  account_id: string;
  session_token: string;
  session_token_expires_at_unix: number;
  access_token: string;
  access_token_expires_at_unix: number;
  session_token_present: boolean;
  access_token_present: boolean;
};

export const accountAuthQueryPrefix = ['gpt', 'account-auth'] as const;
export const accountAuthQueryKey = (accountID: string) => [...accountAuthQueryPrefix, accountID] as const;

export function loadAccountAuthTokens(accountID: string) {
  return api<AccountAuthTokens>(`/api/gpt/accounts/${encodeURIComponent(accountID)}/auth`);
}
