import { Play } from 'lucide-react';
import { RecordActionButtons } from '@/dashboard/module-kit';
import type { RowActionDescriptor } from '@/dashboard/module-kit';
import { canRegister } from './account-utils';
import type { Account } from './types';

export function AccountRowAuthGroups({ account, busy, onRegisterProtocol }: {
  account: Account;
  busy: boolean;
  onRegisterProtocol: (a: Account) => void;
}) {
  return (
    <div className="rowAuthGroups">
      <RowActionGroup actions={protocolActions(account, busy, onRegisterProtocol)} />
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

function protocolActions(account: Account, busy: boolean, onRegister: (a: Account) => void): RowActionDescriptor[] {
  const actions: RowActionDescriptor[] = [];
  if (canRegister(account)) actions.push({ label: '注册', icon: <Play size={14} />, onClick: () => onRegister(account), disabled: busy, kind: 'secondary' });
  return actions;
}
