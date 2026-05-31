import { AccountActionRow, AccountActionRows, AccountOTPActionRow, accountDeleteButtonAction, accountMailboxInboxHintForAccount } from '@byte-v-forge/common-ui';
import type { ActionButtonDescriptor } from '@byte-v-forge/common-ui';
import type { AccountMailboxContext } from '@byte-v-forge/common-ui';
import type { Account, LatestOtp } from './types';

export function AccountDetailActions({ account, showSecrets, busy, inboxLoading, mailboxContext, latestOtp, canFetchOTP, onCopy, onFetchInbox }: {
  account: Account;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  mailboxContext: AccountMailboxContext | null;
  latestOtp: LatestOtp | null;
  canFetchOTP: boolean;
  onCopy: (label: string, value: string) => void;
  onFetchInbox: (account: Account) => Promise<void>;
}) {
  return (
    <AccountActionRows>
      <AccountOTPActionRow
        latestOtp={latestOtp}
        showSecrets={showSecrets}
        canRefresh={canFetchOTP}
        refreshDisabled={busy || inboxLoading}
        refreshHint={accountMailboxInboxHintForAccount(account, mailboxContext, showSecrets)}
        onCopy={onCopy}
        onRefresh={() => void onFetchInbox(account)}
      />
    </AccountActionRows>
  );
}

export function AccountDangerActions({ account, busy, onDelete }: { account: Account; busy: boolean; onDelete: (account: Account) => Promise<void>; }) {
  return <AccountActionRows className="bottomActionRows"><AccountActionRow label="危险" actions={dangerActions(account, busy, onDelete)} /></AccountActionRows>;
}

function dangerActions(account: Account, busy: boolean, onDelete: (account: Account) => Promise<void>): ActionButtonDescriptor[] {
  return [accountDeleteButtonAction(() => onDelete(account), busy)];
}
