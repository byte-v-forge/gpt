import { AccountActionRow, AccountActionRows, AccountOTPActionRow, accountDeleteButtonAction, accountMailboxInboxHintForAccount } from '@byte-v-forge/common-ui';
import type { ActionButtonDescriptor } from '@byte-v-forge/common-ui';
import type { AccountMailboxContext } from '@byte-v-forge/common-ui';
import { AccountDetailExtensionRows } from './account-extension-registry';
import type { AccountWorkflowRunner } from './account-action-specs';
import type { GptActionCatalog } from './action-catalog';
import type { Account, LatestOtp } from './types';

export function AccountDetailActions({ account, actionCatalog, showSecrets, busy, inboxLoading, mailboxContext, latestOtp, canFetchOTP, runWorkflow, onCopy, onFetchInbox }: {
  account: Account;
  actionCatalog?: GptActionCatalog;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  mailboxContext: AccountMailboxContext | null;
  latestOtp: LatestOtp | null;
  canFetchOTP: boolean;
  runWorkflow: AccountWorkflowRunner;
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
      <AccountDetailExtensionRows account={account} actionCatalog={actionCatalog} busy={busy} runWorkflow={runWorkflow} />
    </AccountActionRows>
  );
}

export function AccountDangerActions({ account, busy, onDelete }: { account: Account; busy: boolean; onDelete: (account: Account) => Promise<void>; }) {
  return <AccountActionRows className="bottomActionRows"><AccountActionRow label="危险" actions={dangerActions(account, busy, onDelete)} /></AccountActionRows>;
}

function dangerActions(account: Account, busy: boolean, onDelete: (account: Account) => Promise<void>): ActionButtonDescriptor[] {
  return [accountDeleteButtonAction(() => onDelete(account), busy)];
}
