import { FileKey, KeyRound, Play } from 'lucide-react';
import { RecordActionButtons } from '@/dashboard/module-kit';
import type { RowActionDescriptor } from '@/dashboard/module-kit';
import { canLoginSession, canRegister } from './account-utils';
import type { Account } from './types';

export function AccountRowAuthGroups({ account, busy, onRegisterProtocol, onLoginProtocol, onCodexOAuthProtocol }: {
  account: Account;
  busy: boolean;
  onRegisterProtocol: (a: Account) => void;
  onLoginProtocol: (a: Account) => void;
  onCodexOAuthProtocol: (a: Account) => void;
}) {
  return (
    <div className="rowAuthGroups">
      <RowActionGroup actions={protocolActions(account, busy, onRegisterProtocol, onLoginProtocol, onCodexOAuthProtocol)} />
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

function protocolActions(account: Account, busy: boolean, onRegister: (a: Account) => void, onLogin: (a: Account) => void, onAuth: (a: Account) => void): RowActionDescriptor[] {
  const actions: RowActionDescriptor[] = [];
  if (canRegister(account)) actions.push({ label: '注册', icon: <Play size={14} />, onClick: () => onRegister(account), disabled: busy, kind: 'secondary' });
  if (canLoginSession(account)) {
    actions.push({ label: '登录', icon: <KeyRound size={14} />, onClick: () => onLogin(account), disabled: busy, kind: 'secondary' });
    actions.push({ label: 'auth.json', icon: <FileKey size={14} />, onClick: () => onAuth(account), disabled: busy, kind: 'secondary' });
  }
  return actions;
}
