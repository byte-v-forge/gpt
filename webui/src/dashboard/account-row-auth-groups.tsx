import { AccountRowActionGroups, accountRowActions } from '@byte-v-forge/common-ui';
import type { RowActionDescriptor } from '@byte-v-forge/common-ui';
import type { GptActionCatalog } from './action-catalog';
import { AccountRowExtensionGroups } from './account-extension-registry';
import { ACCOUNT_ROW_ACTIONS, accountWorkflowActionProps, type AccountWorkflowRunner } from './account-action-specs';
import type { Account } from './types';

export function AccountRowAuthGroups({ account, actionCatalog, busy, runWorkflow }: {
  account: Account;
  actionCatalog?: GptActionCatalog;
  busy: boolean;
  runWorkflow: AccountWorkflowRunner;
}) {
  return (
    <>
      <AccountRowActionGroups actions={rowWorkflowActions(account, actionCatalog, busy, runWorkflow)} />
      <AccountRowExtensionGroups account={account} actionCatalog={actionCatalog} busy={busy} runWorkflow={runWorkflow} />
    </>
  );
}

function rowWorkflowActions(account: Account, catalog: GptActionCatalog | undefined, busy: boolean, run: AccountWorkflowRunner): RowActionDescriptor[] {
  return accountRowActions(
    { catalog, account, busy, placement: 'account_row' },
    ACCOUNT_ROW_ACTIONS.map((spec) => accountWorkflowActionProps(spec, run)),
  );
}
