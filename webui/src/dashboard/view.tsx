import type { ReactNode } from 'react';
import { Eye, EyeOff, Trash2 } from 'lucide-react';
import { AccountManagementDrawerView, ToolbarIconButton, accountCarrierID, type AccountListPagination, type AccountRecord } from '@byte-v-forge/common-ui';
import type { Account, Job } from './types';
import { CreateAccountForm } from './accounts';
import type { AccountWorkflowRunner } from './account-action-specs';
import { ACCOUNT_BULK_WORKFLOW_ACTIONS, accountBulkToolbarAction, type AccountBulkWorkflowRunner } from './account-bulk-action-specs';
import type { GptActionCatalog } from './action-catalog';
import { AccountChannelTag, AccountCodexPhoneTag, AccountSignalBadge } from './account-badges';
import { invalidAccountsForCleanup } from './account-cleanup-actions';
import { accountActivationChannel, accountCodexPhoneState } from './account-job-semantics';
import { AccountRowActions } from './account-table';
import { gptAccountRecord } from './account-record';
import { OpenAIIcon } from './brand-icons';

export type GptAccountsViewProps = {
  accounts: Account[];
  jobs: Job[];
  selectedAccountId?: string;
  actionCatalog?: GptActionCatalog;
  showSecrets: boolean;
  busy: boolean;
  cleaningInvalidAccounts: boolean;
  runningAccountIds: Set<string>;
  runningWorkflowByAccountID: Map<string, Job>;
  accountsPagination?: AccountListPagination;
  selectedAccount?: Account | null;
  detail?: ReactNode;
  onCreateDone: (message: string) => Promise<void>;
  onError: (message: string) => void;
  onToggleSecrets: () => void;
  onCleanInvalidAccounts: () => void | Promise<void>;
  onSelectAccount: (account: Account) => void;
  onCloseDetails: () => void;
  runWorkflow: AccountWorkflowRunner;
  runBulkWorkflow: AccountBulkWorkflowRunner;
  onDeleteAccount: (account: Account) => void | Promise<void>;
};

export function GptAccountsView(props: GptAccountsViewProps) {
  const bulkActions = ACCOUNT_BULK_WORKFLOW_ACTIONS.map((spec) => accountBulkToolbarAction(spec, props.accounts, props.jobs, props.actionCatalog, props.busy, props.runBulkWorkflow));
  const invalidAccounts = invalidAccountsForCleanup(props.accounts);
  return (
    <AccountManagementDrawerView
      title="GPT账号"
      icon={<OpenAIIcon size={16} />}
      actions={
        <>
          <CreateAccountForm compact onDone={props.onCreateDone} onError={props.onError} />
          {bulkActions.filter((action) => action.visible).map((action) => (
            <ToolbarIconButton
              key={action.id}
              label={action.label}
              icon={action.icon}
              disabled={action.disabled}
              onClick={action.onClick}
            />
          ))}
          {invalidAccounts.length > 0 && (
            <ToolbarIconButton label={props.cleaningInvalidAccounts ? '清理中' : `清理失效账号 · ${invalidAccounts.length}`} icon={<Trash2 size={15} />} disabled={props.busy || props.cleaningInvalidAccounts} onClick={() => void props.onCleanInvalidAccounts()} />
          )}
          <ToolbarIconButton label={props.showSecrets ? '隐藏敏感信息' : '显示敏感信息'} icon={props.showSecrets ? <EyeOff size={15} /> : <Eye size={15} />} onClick={props.onToggleSecrets} />
        </>
      }
      carriers={props.accounts}
      selectedCarrier={props.selectedAccount}
      selectedID={props.selectedAccountId}
      emptyText="暂无账号。可以先创建账号，或切换为全部状态查看。"
      pagination={props.accountsPagination}
      onSelectCarrier={props.onSelectAccount}
      recordOf={(account) => gptAccountRecord(account, props.showSecrets)}
      config={gptAccountRenderConfig(props.accounts, props.jobs, props.actionCatalog)}
      renderChildren={(account) => {
        const accountID = accountCarrierID(account);
        return <AccountRowActions account={account} actionCatalog={props.actionCatalog} accountBusy={props.runningAccountIds.has(accountID)} currentWorkflow={props.runningWorkflowByAccountID.get(accountID)} busy={props.busy} runWorkflow={props.runWorkflow} onDelete={props.onDeleteAccount} />;
      }}
      drawerTitle="GPT账号详情"
      detail={() => props.detail}
      onCloseDetails={props.onCloseDetails}
    />
  );
}

function gptAccountRenderConfig(accounts: Account[], jobs: Job[], actionCatalog?: GptActionCatalog) {
  const byID = new Map(accounts.map((account) => [accountCarrierID(account), account] as const));
  return {
    icon: () => <OpenAIIcon size={15} />,
    title: (record: AccountRecord) => <span className="accountCardEmail" title={record.display_name}>{record.display_name}</span>,
    subtitle: () => '',
    meta: (record: AccountRecord) => {
      const account = byID.get(accountCarrierID(record));
      if (!account) return null;
      return <div className="accountCardTags"><AccountSignalBadge account={account} compact /><AccountCodexPhoneTag state={accountCodexPhoneState(account, jobs, actionCatalog)} /><AccountChannelTag channel={accountActivationChannel(account, jobs, actionCatalog)} /></div>;
    }
  };
}
