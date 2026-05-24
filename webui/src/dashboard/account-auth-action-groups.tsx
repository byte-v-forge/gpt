import { KeyRound, Play, RefreshCw, Search } from 'lucide-react';
import { ActionButtonGroup } from '@/dashboard/module-kit';
import type { ActionButtonDescriptor } from '@/dashboard/module-kit';
import { CodexIcon } from './brand-icons';
import { canLoginSession, canProbeAccount, canRefreshAccessToken, canRegister, loginActionHint, probeAccountHint } from './account-utils';
import type { Account } from './types';

export function AccountPrimaryActions({ account, busy, refreshingAccessToken, onProbeAccount, onRegister, onRegisterProtocol, onLogin, onLoginProtocol, onCodexOAuthAddPhone, onCodexOAuthProtocol, onRefreshAccessToken }: {
  account: Account;
  busy: boolean;
  refreshingAccessToken: boolean;
  onProbeAccount: (account: Account) => void;
  onRegister: (account: Account) => void;
  onRegisterProtocol: (account: Account) => void;
  onLogin: (account: Account) => void;
  onLoginProtocol: (account: Account) => void;
  onCodexOAuthAddPhone: (account: Account) => void;
  onCodexOAuthProtocol: (account: Account) => void;
  onRefreshAccessToken: (account: Account) => Promise<void>;
}) {
  return (
    <div className="accountHeaderActionGroups">
      <HeaderActionRow label="浏览器" actions={browserActions(account, busy, onRegister, onLogin, onCodexOAuthAddPhone)} />
      <HeaderActionRow label="协议" actions={protocolActions(account, busy, onRegisterProtocol, onLoginProtocol, onCodexOAuthProtocol)} />
      <HeaderActionRow label="工具" actions={utilityActions(account, busy, refreshingAccessToken, onRefreshAccessToken, onProbeAccount)} />
    </div>
  );
}

function HeaderActionRow({ label, actions }: { label: string; actions: ActionButtonDescriptor[] }) {
  if (!actions.some((action) => action.visible !== false)) return null;
  return (
    <div className="detailActionRow accountHeaderActionRow">
      <span className="detailActionLabel">{label}</span>
      <ActionButtonGroup className="sectionActions accountHeaderActions" actions={actions} />
    </div>
  );
}

function browserActions(account: Account, busy: boolean, onRegister: (account: Account) => void, onLogin: (account: Account) => void, onAuth: (account: Account) => void): ActionButtonDescriptor[] {
  return [{
    id: 'browser-register',
    visible: canRegister(account),
    label: '浏览器注册',
    hint: '使用浏览器自动化注册账号',
    icon: <Play size={14} />,
    disabled: busy,
    onClick: () => onRegister(account),
  }, {
    id: 'browser-login-session',
    visible: canLoginSession(account),
    label: '浏览器登录',
    hint: `${loginActionHint(account)}（浏览器）`,
    icon: <KeyRound size={14} />,
    disabled: busy,
    onClick: () => onLogin(account),
  }, {
    id: 'browser-codex-oauth',
    visible: canLoginSession(account),
    label: '浏览器 auth.json',
    hint: '浏览器 OAuth 登录并产出 auth.json；遇到 add phone 页面会停下报错',
    icon: <CodexIcon size={14} />,
    disabled: busy,
    onClick: () => onAuth(account),
  }];
}

function protocolActions(account: Account, busy: boolean, onRegister: (account: Account) => void, onLogin: (account: Account) => void, onAuth: (account: Account) => void): ActionButtonDescriptor[] {
  return [{
    id: 'protocol-register',
    visible: canRegister(account),
    label: '协议注册',
    hint: '不依赖浏览器，使用协议注册账号',
    icon: <Play size={14} />,
    disabled: busy,
    onClick: () => onRegister(account),
  }, {
    id: 'protocol-login-session',
    visible: canLoginSession(account),
    label: '协议登录',
    hint: `${loginActionHint(account)}（协议）`,
    icon: <KeyRound size={14} />,
    disabled: busy,
    onClick: () => onLogin(account),
  }, {
    id: 'protocol-codex-oauth',
    visible: canLoginSession(account),
    label: '协议 auth.json',
    hint: '不依赖浏览器，使用协议 OAuth 产出 auth.json',
    icon: <CodexIcon size={14} />,
    disabled: busy,
    onClick: () => onAuth(account),
  }];
}

function utilityActions(account: Account, busy: boolean, refreshing: boolean, onRefresh: (account: Account) => Promise<void>, onProbe: (account: Account) => void): ActionButtonDescriptor[] {
  return [{
    id: 'refresh-access-token',
    visible: canRefreshAccessToken(account),
    label: refreshing ? '刷新中' : '刷新 Token',
    hint: '使用当前 Session 刷新 Access Token',
    icon: <RefreshCw size={14} />,
    disabled: busy || refreshing,
    onClick: () => void onRefresh(account),
  }, {
    id: 'probe-account',
    label: '探测账号',
    hint: probeAccountHint(account),
    icon: <Search size={14} />,
    disabled: busy || !canProbeAccount(account),
    onClick: () => onProbe(account),
  }];
}
