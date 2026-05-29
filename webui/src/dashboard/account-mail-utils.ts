import {
  canonicalUiEmail,
  maskEmail,
  normalizeUiEmail,
} from "@byte-v-forge/common-ui";
import type {
  Account,
  AccountMailboxContext,
  GPTEmailAllocation,
  Mailbox,
} from "./types";

export function mailboxContextForEmail(
  mailboxes: Mailbox[],
  allocations: GPTEmailAllocation[],
  account: Account,
): AccountMailboxContext {
  const accountEmail = normalizeUiEmail(account.email);
  const allocation = allocationForEmail(allocations, accountEmail);
  const primaryEmail = normalizeUiEmail(
    account.primary_mailbox_email ||
      allocation?.primary_email ||
      canonicalUiEmail(accountEmail),
  );
  const mailbox = mailboxes.find((item) =>
    [accountEmail, primaryEmail].includes(normalizeUiEmail(item.email_address)),
  );
  return {
    account_email: accountEmail,
    primary_email: primaryEmail,
    provider_key: mailbox?.provider_key || "",
    is_split: !!accountEmail && !!primaryEmail && accountEmail !== primaryEmail,
    known: !!mailbox || !!allocation,
  };
}

export function accountInboxHint(
  email: string,
  context: AccountMailboxContext | null,
  showSecrets: boolean,
) {
  const accountEmail = showSecrets ? email : maskEmail(email);
  if (!context?.is_split)
    return `重新读取当前账号邮箱 ${accountEmail} 的 OTP 缓存`;
  const primaryEmail = showSecrets
    ? context.primary_email
    : maskEmail(context.primary_email);
  return `重新读取邮箱账号 ${primaryEmail} 的 OTP 缓存，按收件地址 ${accountEmail} 匹配`;
}

export function canFetchAccountInbox(
  account: Account,
  context: AccountMailboxContext | null,
) {
  return !!normalizeUiEmail(account.email) && !!context?.known;
}

export function allocationForEmail(
  allocations: GPTEmailAllocation[],
  email: string,
) {
  const target = normalizeUiEmail(email);
  if (!target) return undefined;
  return allocations.find(
    (allocation) => normalizeUiEmail(allocation.email) === target,
  );
}

export function countAllocatableEmailAllocations(
  allocations: GPTEmailAllocation[],
) {
  return allocations.filter(
    (allocation) =>
      allocation.status === "AVAILABLE" ||
      (allocation.is_primary &&
        allocation.status === "REGISTERED" &&
        allocation.splittable),
  ).length;
}
