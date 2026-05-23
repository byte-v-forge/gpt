import { api } from '@/dashboard/module-kit';
import { mergeInboxMessage, normalizeUiEmail } from '@/dashboard/modules/mailbox/sdk';
import type { InboxMessage, InboxResponse, InboxResult } from './types';

export const accountInboxQueryKey = (accountID: string, email: string) => ['gpt', 'inbox', accountID, normalizeUiEmail(email)] as const;

export type MailboxEmailEvent = {
  email_address: string;
  message?: InboxMessage;
};

export async function loadStoredInbox(email: string): Promise<InboxResponse> {
  const result = await api<InboxResult>(`/api/mailboxes/${encodeURIComponent(email)}/inbox?limit=25`);
  const messageCount = result.messages?.length || 0;
  return {
    results: [result],
    mailbox_count: 1,
    fetched_count: 1,
    failed_count: result.error_message ? 1 : 0,
    message_count: messageCount
  };
}

export function mailboxEventURL(email: string) {
  const params = new URLSearchParams({ email_address: email, signal_kind: 'otp' });
  return `/api/mailboxes/events?${params.toString()}`;
}

export function mergeInboxResponse(prev: InboxResponse | null | undefined, email: string, message: InboxMessage): InboxResponse {
  const current = prev || { results: [], mailbox_count: 1, fetched_count: 1, failed_count: 0, message_count: 0 };
  const result = mergeInboxMessage(current.results?.[0] || null, email, message);
  return { ...current, results: [result], message_count: result.messages?.length || 0 };
}
