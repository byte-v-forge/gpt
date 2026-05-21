import { canonicalUiEmail, maskEmail, normalizeUiEmail } from '@/dashboard/modules/mailbox/sdk';
import type { Account, AccountMailboxContext, GPTEmailAllocation, LatestOtp, Mailbox } from './types';

export function mailboxContextForEmail(mailboxes: Mailbox[], allocations: GPTEmailAllocation[], account: Account): AccountMailboxContext {
  const accountEmail = normalizeUiEmail(account.email);
  const mailbox = mailboxes.find((item) => normalizeUiEmail(item.email_address) === accountEmail);
  const allocation = allocationForEmail(allocations, accountEmail);
  const primaryEmail = normalizeUiEmail(account.primary_mailbox_email || allocation?.primary_email || canonicalUiEmail(accountEmail));
  return {
    account_email: accountEmail,
    primary_email: primaryEmail,
    is_split: !!accountEmail && !!primaryEmail && accountEmail !== primaryEmail,
    known: !!mailbox || !!allocation
  };
}

export function accountInboxHint(email: string, context: AccountMailboxContext | null, showSecrets: boolean) {
  const accountEmail = showSecrets ? email : maskEmail(email);
  if (!context?.is_split) return `拉取当前账号邮箱 ${accountEmail} 的最新 OTP`;
  const primaryEmail = showSecrets ? context.primary_email : maskEmail(context.primary_email);
  return `用邮箱账号 ${primaryEmail} 拉取收件箱，按收件地址 ${accountEmail} 匹配 OTP`;
}

export function latestOtpFromAccount(account: Account): LatestOtp | null {
  if (!account.mailbox_latest_otp) return null;
  return {
    otp: account.mailbox_latest_otp,
    subject: account.mailbox_latest_otp_subject || '',
    received_at_unix: account.mailbox_latest_otp_received_at_unix || 0
  };
}

export function allocationForEmail(allocations: GPTEmailAllocation[], email: string) {
  const target = normalizeUiEmail(email);
  if (!target) return undefined;
  return allocations.find((allocation) => normalizeUiEmail(allocation.email) === target);
}

export function countAllocatableEmailAllocations(allocations: GPTEmailAllocation[]) {
  return allocations.filter((allocation) => (
    allocation.status === 'AVAILABLE' ||
    (allocation.is_primary && allocation.status === 'REGISTERED' && allocation.splittable)
  )).length;
}
