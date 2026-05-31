import { Eye, EyeOff, Trash2 } from 'lucide-react';
import { PanelHeader, ToolbarIconButton, type AccountListPagination } from '@byte-v-forge/common-ui';
import type { Account, Job } from './types';
import { AccountTable, CreateAccountForm } from './accounts';
import type { AccountWorkflowRunner } from './account-action-specs';
import { ACCOUNT_BULK_WORKFLOW_ACTIONS, accountBulkToolbarAction, type AccountBulkWorkflowRunner } from './account-bulk-action-specs';
import type { GptActionCatalog } from './action-catalog';
import { invalidAccountsForCleanup } from './account-cleanup-actions';
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
  onCreateDone: (message: string) => Promise<void>;
  onError: (message: string) => void;
  onToggleSecrets: () => void;
  onCleanInvalidAccounts: () => void | Promise<void>;
  onSelectAccount: (account: Account) => void;
  runWorkflow: AccountWorkflowRunner;
  runBulkWorkflow: AccountBulkWorkflowRunner;
  onDeleteAccount: (account: Account) => void | Promise<void>;
};

export function GptAccountsView(props: GptAccountsViewProps) {
  const bulkActions = ACCOUNT_BULK_WORKFLOW_ACTIONS.map((spec) => accountBulkToolbarAction(spec, props.accounts, props.jobs, props.actionCatalog, props.busy, props.runBulkWorkflow));
  const invalidAccounts = invalidAccountsForCleanup(props.accounts);
  return (
    <>
      <PanelHeader title="GPT账号" icon={<OpenAIIcon size={16} />}>
        <div className="headerControls accountHeaderControls">
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
        </div>
      </PanelHeader>
      <AccountTable
        accounts={props.accounts}
        jobs={props.jobs}
        selected={props.selectedAccountId}
        actionCatalog={props.actionCatalog}
        showSecrets={props.showSecrets}
        runningAccountIds={props.runningAccountIds}
        runningWorkflowByAccountID={props.runningWorkflowByAccountID}
        pagination={props.accountsPagination}
        busy={props.busy}
        onSelect={props.onSelectAccount}
        runWorkflow={props.runWorkflow}
        onDelete={props.onDeleteAccount}
      />
    </>
  );
}
