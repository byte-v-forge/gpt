import { AccountRowActionGroups, accountRowAction } from '@byte-v-forge/common-ui';
import type { RowActionDescriptor } from '@byte-v-forge/common-ui';
import type { GptActionCatalog } from './action-catalog';
import { ACCOUNT_ROW_ACTIONS, accountWorkflowActionProps, type AccountWorkflowRunner } from './account-action-specs';
import type { Account } from './types';

export function AccountRowAuthGroups({ account, actionCatalog, busy, runWorkflow }: {
  account: Account;
  actionCatalog?: GptActionCatalog;
  busy: boolean;
  runWorkflow: AccountWorkflowRunner;
}) {
  return <AccountRowActionGroups actions={rowWorkflowActions(account, actionCatalog, busy, runWorkflow)} />;
}

function rowWorkflowActions(account: Account, catalog: GptActionCatalog | undefined, busy: boolean, run: AccountWorkflowRunner): RowActionDescriptor[] {
  return ACCOUNT_ROW_ACTIONS
    .map((spec) => accountRowAction({ catalog, account, busy, placement: 'account_row' }, accountWorkflowActionProps(spec, run)))
    .filter((action): action is RowActionDescriptor => !!action);
}
