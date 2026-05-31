import { Phone } from 'lucide-react';
import type { ReactNode } from 'react';
import { accountActionAvailability, accountActionLabel } from '@byte-v-forge/common-ui';
import { GPT_ACTIONS, type GptActionCatalog, type GptActionID, type GptActionPlacement } from './action-catalog';
import { accountCodexPhoneState } from './account-job-semantics';
import { canLoginSession } from './account-utils';
import type { Account, Job } from './types';

export type AccountBulkWorkflowRunner = (actionID: GptActionID, accounts: Account[], fallbackLabel: string) => void | Promise<void>;

export type AccountBulkWorkflowActionSpec = {
  id: string;
  actionID: GptActionID;
  fallbackLabel: string;
  icon: ReactNode;
  placement: GptActionPlacement;
  countLabel: string;
  selectAccounts: (accounts: Account[], jobs: Job[], catalog?: GptActionCatalog) => Account[];
};

export type AccountBulkToolbarAction = {
  id: string;
  visible: boolean;
  label: string;
  icon: ReactNode;
  disabled: boolean;
  onClick: () => void;
};

export const ACCOUNT_BULK_WORKFLOW_ACTIONS: AccountBulkWorkflowActionSpec[] = [
  {
    id: 'bulk-codex-oauth-add-phone',
    actionID: GPT_ACTIONS.codexOAuthBatchAddPhone,
    fallbackLabel: '批量 Add Phone',
    icon: <Phone size={15} />,
    placement: 'account_bulk',
    countLabel: '个未加手机账号',
    selectAccounts: codexOAuthAddPhoneAccounts,
  },
];

export function accountBulkToolbarAction(spec: AccountBulkWorkflowActionSpec, accounts: Account[], jobs: Job[], catalog: GptActionCatalog | undefined, busy: boolean, run: AccountBulkWorkflowRunner): AccountBulkToolbarAction {
  const selected = spec.selectAccounts(accounts, jobs, catalog);
  const availability = accountActionAvailability(catalog, spec.actionID, undefined, spec.placement);
  return {
    id: spec.id,
    visible: selected.length > 0 && availability.visible,
    label: `${accountActionLabel(catalog, spec.actionID, spec.fallbackLabel, spec.placement)} · ${selected.length} ${spec.countLabel}`,
    icon: spec.icon,
    disabled: busy || !availability.enabled,
    onClick: () => void run(spec.actionID, selected, spec.fallbackLabel),
  };
}

function codexOAuthAddPhoneAccounts(accounts: Account[], jobs: Job[], catalog?: GptActionCatalog) {
  return accounts.filter((account) => canLoginSession(account) && !accountCodexPhoneState(account, jobs, catalog).confirmed);
}
