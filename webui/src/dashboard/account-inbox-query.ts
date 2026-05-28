import { api, normalizeUiEmail } from '@byte-v-forge/common-ui';
import type { FetchAccountMailboxResponse, InboxResponse } from './types';

export const accountInboxQueryPrefix = ['gpt', 'inbox'] as const;

export const accountInboxQueryKey = (accountID: string, email: string, version = 0) => [...accountInboxQueryPrefix, accountID, normalizeUiEmail(email), version] as const;

export async function loadAccountMailboxProjection(accountID: string): Promise<InboxResponse> {
  const resp = await api<FetchAccountMailboxResponse>(`/api/gpt/accounts/${encodeURIComponent(accountID)}/mailbox/inbox`, { method: 'POST', body: JSON.stringify({}) });
  return resp.inbox || { results: [], mailbox_count: 0, fetched_count: 0, message_count: 0 };
}
