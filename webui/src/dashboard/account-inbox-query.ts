import { accountQueryKey, accountQueryPrefix, api, normalizeUiEmail } from '@byte-v-forge/common-ui';
import type { FetchAccountMailboxResponse, InboxResponse } from './types';

export const accountInboxQueryPrefix = accountQueryPrefix('gpt', 'inbox');

export const accountInboxQueryKey = (accountID: string, email: string, version = 0) => accountQueryKey(accountInboxQueryPrefix, accountID, normalizeUiEmail(email), version);

export async function loadAccountMailboxProjection(accountID: string): Promise<InboxResponse> {
  const resp = await api<FetchAccountMailboxResponse>(`/api/gpt/accounts/${encodeURIComponent(accountID)}/mailbox/inbox`, { method: 'POST', body: JSON.stringify({}) });
  return resp.inbox || { results: [], mailbox_count: 0, fetched_count: 0, failed_count: 0, message_count: 0 };
}
