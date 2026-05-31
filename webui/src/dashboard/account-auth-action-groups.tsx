import { RefreshCw } from 'lucide-react';
import { AccountActionRow, AccountActionRows, accountActionButton } from '@byte-v-forge/common-ui';
import type { ActionButtonDescriptor } from '@byte-v-forge/common-ui';
import type { GptActionCatalog } from './action-catalog';
import { ACCOUNT_HEADER_BROWSER_ACTIONS, ACCOUNT_HEADER_PROTOCOL_ACTIONS, ACCOUNT_TOOL_WORKFLOW_ACTIONS, accountWorkflowActionProps, type AccountWorkflowActionSpec, type AccountWorkflowRunner } from './account-action-specs';
import { canUpdateWebAccessToken } from './account-utils';
import type { Account } from './types';

export function AccountPrimaryActions({ account, actionCatalog, busy, updatingWebAccessToken, runWorkflow, onUpdateWebAccessToken }: {
  account: Account;
  actionCatalog?: GptActionCatalog;
  busy: boolean;
  updatingWebAccessToken: boolean;
  runWorkflow: AccountWorkflowRunner;
  onUpdateWebAccessToken: (account: Account) => Promise<void>;
}) {
  return (
    <AccountActionRows className="accountPrimaryActionRows">
      <AccountActionRow className="accountPrimaryActionRow" buttonGroupClassName="sectionActions accountPrimaryActions" label="浏览器" actions={workflowActionButtons(ACCOUNT_HEADER_BROWSER_ACTIONS, actionCatalog, account, busy, 'account_header_browser', runWorkflow)} />
      <AccountActionRow className="accountPrimaryActionRow" buttonGroupClassName="sectionActions accountPrimaryActions" label="协议" actions={workflowActionButtons(ACCOUNT_HEADER_PROTOCOL_ACTIONS, actionCatalog, account, busy, 'account_header_protocol', runWorkflow)} />
      <AccountActionRow className="accountPrimaryActionRow" buttonGroupClassName="sectionActions accountPrimaryActions" label="工具" actions={utilityActions(actionCatalog, account, busy, updatingWebAccessToken, onUpdateWebAccessToken, runWorkflow)} />
    </AccountActionRows>
  );
}

function workflowActionButtons(specs: AccountWorkflowActionSpec[], catalog: GptActionCatalog | undefined, account: Account, busy: boolean, placement: 'account_header_browser' | 'account_header_protocol' | 'account_header_tools', run: AccountWorkflowRunner): ActionButtonDescriptor[] {
  const ctx = { catalog, account, busy, placement };
  return specs.map((spec) => accountActionButton(ctx, accountWorkflowActionProps(spec, run)));
}

function utilityActions(catalog: GptActionCatalog | undefined, account: Account, busy: boolean, refreshing: boolean, onRefresh: (account: Account) => Promise<void>, run: AccountWorkflowRunner): ActionButtonDescriptor[] {
  return [{
    id: 'refresh-access-token',
    visible: canUpdateWebAccessToken(account),
    label: refreshing ? '更新中' : '更新 Web AT',
    hint: '使用当前 Session Token 获取 ChatGPT Web AT',
    icon: <RefreshCw size={14} />,
    disabled: busy || refreshing,
    onClick: () => void onRefresh(account),
  }, ...workflowActionButtons(ACCOUNT_TOOL_WORKFLOW_ACTIONS, catalog, account, busy, 'account_header_tools', run)];
}
