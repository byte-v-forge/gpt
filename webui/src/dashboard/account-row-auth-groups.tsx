import { Play } from 'lucide-react';
import { RecordActionButtons } from '@byte-v-forge/common-ui';
import type { RowActionDescriptor } from '@byte-v-forge/common-ui';
import { GPT_ACTIONS, gptActionAvailability, gptActionLabel, type GptActionCatalog } from './action-catalog';
import { canRegister } from './account-utils';
import type { Account } from './types';

export function AccountRowAuthGroups({ account, actionCatalog, busy, onRegisterProtocol }: {
  account: Account;
  actionCatalog?: GptActionCatalog;
  busy: boolean;
  onRegisterProtocol: (a: Account) => void;
}) {
  return (
    <div className="rowAuthGroups">
      <RowActionGroup actions={protocolActions(account, actionCatalog, busy, onRegisterProtocol)} />
    </div>
  );
}

function RowActionGroup({ actions }: { actions: RowActionDescriptor[] }) {
  if (actions.length === 0) return null;
  return (
    <span className="rowActionGroup">
      <RecordActionButtons actions={actions} />
    </span>
  );
}

function protocolActions(account: Account, catalog: GptActionCatalog | undefined, busy: boolean, onRegister: (a: Account) => void): RowActionDescriptor[] {
  const placement = 'account_row';
  const availability = gptActionAvailability(catalog, GPT_ACTIONS.registerProtocol, account, placement);
  if (!availability.visible) return [];
  return [{
    label: gptActionLabel(catalog, GPT_ACTIONS.registerProtocol, '注册', placement),
    icon: <Play size={14} />,
    onClick: () => onRegister(account),
    disabled: busy || !availability.enabled || !canRegister(account),
    kind: 'secondary'
  }];
}
