import type { ComponentType } from 'react';
import type { GptActionCatalog } from './action-catalog';
import type { AccountWorkflowRunner } from './account-action-specs';
import type { Account } from './types';

export type AccountExtensionRenderProps = {
  account: Account;
  actionCatalog?: GptActionCatalog;
  busy: boolean;
  runWorkflow: AccountWorkflowRunner;
};

export type AccountExtensionRegistration = {
  id: string;
  Component: ComponentType<AccountExtensionRenderProps>;
};

const detailExtensions: AccountExtensionRegistration[] = [];
const rowExtensions: AccountExtensionRegistration[] = [];

export function registerAccountDetailActionExtensions(items: AccountExtensionRegistration[]) {
  registerExtensions(detailExtensions, items);
}

export function registerAccountRowActionExtensions(items: AccountExtensionRegistration[]) {
  registerExtensions(rowExtensions, items);
}

export function AccountDetailExtensionRows(props: AccountExtensionRenderProps) {
  return <>{detailExtensions.map(({ id, Component }) => <Component key={id} {...props} />)}</>;
}

export function AccountRowExtensionGroups(props: AccountExtensionRenderProps) {
  return <>{rowExtensions.map(({ id, Component }) => <Component key={id} {...props} />)}</>;
}

function registerExtensions(target: AccountExtensionRegistration[], items: AccountExtensionRegistration[]) {
  for (const item of items) {
    const index = target.findIndex((existing) => existing.id === item.id);
    if (index >= 0) target[index] = item;
    else target.push(item);
  }
}
