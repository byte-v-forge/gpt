import {
  AccountCarrierList,
  RecordActionButtons,
  RecordActions,
  accountCarrierID,
  accountDeleteRowAction,
  type AccountListPagination
} from '@byte-v-forge/common-ui';
import type { RowActionDescriptor } from '@byte-v-forge/common-ui';
import { isInvalidGptAccount, isUserAlreadyExistsAccount } from './account-utils';
import { accountActivationChannel, accountCodexPhoneState } from './account-job-semantics';
import { AccountChannelTag, AccountCodexPhoneTag, AccountSignalBadge } from './account-badges';
import type { AccountWorkflowRunner } from './account-action-specs';
import type { GptActionCatalog } from './action-catalog';
import { gptAccountRecord } from './account-record';
import { AccountRowAuthGroups } from './account-row-auth-groups';
import { OpenAIIcon } from './brand-icons';
import type { Account, Job } from './types';

export function AccountTable({ accounts, jobs, selected, actionCatalog, showSecrets, runningAccountIds, runningWorkflowByAccountID, pagination, busy, onSelect, runWorkflow, onDelete }: {
  accounts: Account[];
  jobs: Job[];
  selected?: string;
  actionCatalog?: GptActionCatalog;
  showSecrets: boolean;
  runningAccountIds: Set<string>;
  runningWorkflowByAccountID: Map<string, Job>;
  pagination?: AccountListPagination;
  busy: boolean;
  onSelect: (a: Account) => void;
  runWorkflow: AccountWorkflowRunner;
  onDelete: (a: Account) => void | Promise<void>;
}) {
  const byID = new Map(accounts.map((account) => [accountCarrierID(account), account] as const));
  return (
    <AccountCarrierList
      carriers={accounts}
      selectedID={selected}
      emptyText="暂无账号。可以先创建账号，或切换为全部状态查看。"
      listClassName="accountManagementList"
      pagination={pagination}
      onSelectCarrier={onSelect}
      recordOf={(account) => gptAccountRecord(account, showSecrets)}
      config={{
        icon: () => <OpenAIIcon size={15} />,
        title: (record) => <span className="accountCardEmail" title={record.display_name}>{record.display_name}</span>,
        subtitle: () => '',
        meta: (record) => {
          const account = byID.get(accountCarrierID(record));
          if (!account) return null;
          const activationChannel = accountActivationChannel(account, jobs, actionCatalog);
          const phoneState = accountCodexPhoneState(account, jobs, actionCatalog);
          return (
            <div className="accountCardTags">
              <AccountSignalBadge account={account} compact />
              <AccountCodexPhoneTag state={phoneState} />
              <AccountChannelTag channel={activationChannel} />
            </div>
          );
        }
      }}
      renderChildren={(account) => {
        const accountBusy = runningAccountIds.has(accountCarrierID(account));
        const currentWorkflow = runningWorkflowByAccountID.get(accountCarrierID(account));
        return <AccountRowActions account={account} actionCatalog={actionCatalog} accountBusy={accountBusy} currentWorkflow={currentWorkflow} busy={busy} runWorkflow={runWorkflow} onDelete={onDelete} />;
      }}
    />
  );
}

export function AccountRowActions({ account, actionCatalog, accountBusy, currentWorkflow, busy, runWorkflow, onDelete }: {
  account: Account;
  actionCatalog?: GptActionCatalog;
  accountBusy: boolean;
  currentWorkflow?: Job;
  busy: boolean;
  runWorkflow: AccountWorkflowRunner;
  onDelete: (a: Account) => void | Promise<void>;
}) {
  if (isInvalidGptAccount(account)) {
    const actions: RowActionDescriptor[] = [accountDeleteRowAction(() => onDelete(account), busy)];
    return (
      <RecordActions className="rowActions">
        <div className="rowActionsMain"><RecordActionButtons actions={actions} /></div>
      </RecordActions>
    );
  }
  if (accountBusy && currentWorkflow && !isUserAlreadyExistsAccount(account)) {
    return (
      <RecordActions className="rowActions">
        <div className="rowActionsMain"><span className="accountWorkflowNotice">流程运行中，请到工作流页处理</span></div>
      </RecordActions>
    );
  }
  return (
    <RecordActions className="rowActions">
      <div className="rowActionsMain"><AccountRowAuthGroups account={account} actionCatalog={actionCatalog} busy={busy} runWorkflow={runWorkflow} /></div>
    </RecordActions>
  );
}
