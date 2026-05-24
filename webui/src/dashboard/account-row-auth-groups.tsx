import { FileKey, KeyRound, Play } from 'lucide-react';
import { RecordActionButtons } from '@/dashboard/module-kit';
import type { RowActionDescriptor } from '@/dashboard/module-kit';
import { canLoginSession, canRefreshAccessToken, canRegister } from './account-utils';
import type { Account } from './types';

export function AccountRowAuthGroups({ account, busy, refreshingAccessToken, onRegister, onRegisterProtocol, onLogin, onLoginProtocol, onCodexOAuthAddPhone, onCodexOAuthProtocol, onRefreshAccessToken }: {
  account: Account;
  busy: boolean;
  refreshingAccessToken: boolean;
  onRegister: (a: Account) => void;
  onRegisterProtocol: (a: Account) => void;
  onLogin: (a: Account) => void;
  onLoginProtocol: (a: Account) => void;
  onCodexOAuthAddPhone: (a: Account) => void;
  onCodexOAuthProtocol: (a: Account) => void;
  onRefreshAccessToken: (a: Account) => Promise<void>;
}) {
  return (
    <div className="rowAuthGroups">
      <RowActionGroup label="浏览器" actions={browserActions(account, busy, onRegister, onLogin, onCodexOAuthAddPhone)} />
      <RowActionGroup label="协议" actions={protocolActions(account, busy, onRegisterProtocol, onLoginProtocol, onCodexOAuthProtocol)} />
      <RowActionGroup label="工具" actions={utilityActions(account, busy, refreshingAccessToken, onRefreshAccessToken)} />
    </div>
  );
}

function RowActionGroup({ label, actions }: { label: string; actions: RowActionDescriptor[] }) {
  if (actions.length === 0) return null;
  return (
    <span className="rowActionGroup">
      <span className="rowActionGroupLabel">{label}</span>
      <RecordActionButtons actions={actions} />
    </span>
  );
}

function browserActions(account: Account, busy: boolean, onRegister: (a: Account) => void, onLogin: (a: Account) => void, onAuth: (a: Account) => void): RowActionDescriptor[] {
  const actions: RowActionDescriptor[] = [];
  if (canRegister(account)) actions.push({ label: '浏览器注册', icon: <Play size={14} />, onClick: () => onRegister(account), disabled: busy, kind: 'primary' });
  if (canLoginSession(account)) {
    actions.push({ label: '浏览器登录', icon: <KeyRound size={14} />, onClick: () => onLogin(account), disabled: busy, kind: 'secondary' });
    actions.push({ label: '浏览器 auth.json', icon: <FileKey size={14} />, onClick: () => onAuth(account), disabled: busy, kind: 'secondary' });
  }
  return actions;
}

function protocolActions(account: Account, busy: boolean, onRegister: (a: Account) => void, onLogin: (a: Account) => void, onAuth: (a: Account) => void): RowActionDescriptor[] {
  const actions: RowActionDescriptor[] = [];
  if (canRegister(account)) actions.push({ label: '协议注册', icon: <Play size={14} />, onClick: () => onRegister(account), disabled: busy, kind: 'secondary' });
  if (canLoginSession(account)) {
    actions.push({ label: '协议登录', icon: <KeyRound size={14} />, onClick: () => onLogin(account), disabled: busy, kind: 'secondary' });
    actions.push({ label: '协议 auth.json', icon: <FileKey size={14} />, onClick: () => onAuth(account), disabled: busy, kind: 'secondary' });
  }
  return actions;
}

function utilityActions(account: Account, busy: boolean, refreshing: boolean, onRefresh: (a: Account) => Promise<void>): RowActionDescriptor[] {
  if (!canRefreshAccessToken(account)) return [];
  return [{ label: refreshing ? '获取中' : '获取 Access', icon: <KeyRound size={14} />, onClick: () => void onRefresh(account), disabled: busy || refreshing, kind: 'secondary' }];
}
