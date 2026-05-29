import { KeyRound, Play, RefreshCw, Search } from 'lucide-react';
import { ActionButtonGroup } from '@byte-v-forge/common-ui';
import type { ActionButtonDescriptor } from '@byte-v-forge/common-ui';
import { GPT_ACTIONS, gptActionAvailability, gptActionLabel, type GptActionCatalog, type GptActionPlacement } from './action-catalog';
import { CodexIcon } from './brand-icons';
import { canLoginSession, canProbeAccount, canRefreshAccessToken, canRegister, loginActionHint, probeAccountHint } from './account-utils';
import type { Account } from './types';

export function AccountPrimaryActions({ account, actionCatalog, busy, refreshingAccessToken, onProbeAccount, onRegister, onRegisterProtocol, onLogin, onLoginProtocol, onCodexOAuthAddPhone, onCodexOAuthProtocol, onRefreshAccessToken }: {
  account: Account;
  actionCatalog?: GptActionCatalog;
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
    <div className="detailActionRows accountPrimaryActionRows">
      <HeaderActionRow label="浏览器" actions={browserActions(actionCatalog, account, busy, onRegister, onLogin, onCodexOAuthAddPhone)} />
      <HeaderActionRow label="协议" actions={protocolActions(actionCatalog, account, busy, onRegisterProtocol, onLoginProtocol, onCodexOAuthProtocol)} />
      <HeaderActionRow label="工具" actions={utilityActions(actionCatalog, account, busy, refreshingAccessToken, onRefreshAccessToken, onProbeAccount)} />
    </div>
  );
}

function HeaderActionRow({ label, actions }: { label: string; actions: ActionButtonDescriptor[] }) {
  if (!actions.some((action) => action.visible !== false)) return null;
  return (
    <div className="detailActionRow accountPrimaryActionRow">
      <span className="detailActionLabel">{label}</span>
      <ActionButtonGroup className="sectionActions accountPrimaryActions" actions={actions} />
    </div>
  );
}

function browserActions(catalog: GptActionCatalog | undefined, account: Account, busy: boolean, onRegister: (account: Account) => void, onLogin: (account: Account) => void, onAuth: (account: Account) => void): ActionButtonDescriptor[] {
  const placement: GptActionPlacement = 'account_header_browser';
  const register = gptActionAvailability(catalog, GPT_ACTIONS.register, account, placement);
  const login = gptActionAvailability(catalog, GPT_ACTIONS.loginSession, account, placement);
  const codex = gptActionAvailability(catalog, GPT_ACTIONS.codexOAuth, account, placement);
  return [{
    id: 'browser-register',
    visible: register.visible,
    label: gptActionLabel(catalog, GPT_ACTIONS.register, '浏览器注册', placement),
    hint: actionHint(register.reason, canRegister(account), '当前账号不可注册', '使用浏览器自动化注册账号'),
    icon: <Play size={14} />,
    disabled: busy || !register.enabled || !canRegister(account),
    onClick: () => onRegister(account),
  }, {
    id: 'browser-login-session',
    visible: login.visible,
    label: gptActionLabel(catalog, GPT_ACTIONS.loginSession, '浏览器登录', placement),
    hint: actionHint(login.reason, canLoginSession(account), '需要邮箱和密码', `${loginActionHint(account)}（浏览器）`),
    icon: <KeyRound size={14} />,
    disabled: busy || !login.enabled || !canLoginSession(account),
    onClick: () => onLogin(account),
  }, {
    id: 'browser-codex-oauth',
    visible: codex.visible,
    label: gptActionLabel(catalog, GPT_ACTIONS.codexOAuth, '浏览器 auth.json', placement),
    hint: actionHint(codex.reason, canLoginSession(account), '需要邮箱和密码', '浏览器 OAuth 登录并产出 auth.json；遇到 add phone 页面会停下报错'),
    icon: <CodexIcon size={14} />,
    disabled: busy || !codex.enabled || !canLoginSession(account),
    onClick: () => onAuth(account),
  }];
}

function protocolActions(catalog: GptActionCatalog | undefined, account: Account, busy: boolean, onRegister: (account: Account) => void, onLogin: (account: Account) => void, onAuth: (account: Account) => void): ActionButtonDescriptor[] {
  const placement: GptActionPlacement = 'account_header_protocol';
  const register = gptActionAvailability(catalog, GPT_ACTIONS.registerProtocol, account, placement);
  const login = gptActionAvailability(catalog, GPT_ACTIONS.loginSessionProtocol, account, placement);
  const codex = gptActionAvailability(catalog, GPT_ACTIONS.codexOAuthProtocol, account, placement);
  return [{
    id: 'protocol-register',
    visible: register.visible,
    label: gptActionLabel(catalog, GPT_ACTIONS.registerProtocol, '协议注册', placement),
    hint: actionHint(register.reason, canRegister(account), '当前账号不可注册', '不依赖浏览器，使用协议注册账号'),
    icon: <Play size={14} />,
    disabled: busy || !register.enabled || !canRegister(account),
    onClick: () => onRegister(account),
  }, {
    id: 'protocol-login-session',
    visible: login.visible,
    label: gptActionLabel(catalog, GPT_ACTIONS.loginSessionProtocol, '协议登录', placement),
    hint: actionHint(login.reason, canLoginSession(account), '需要邮箱和密码', `${loginActionHint(account)}（协议）`),
    icon: <KeyRound size={14} />,
    disabled: busy || !login.enabled || !canLoginSession(account),
    onClick: () => onLogin(account),
  }, {
    id: 'protocol-codex-oauth',
    visible: codex.visible,
    label: gptActionLabel(catalog, GPT_ACTIONS.codexOAuthProtocol, '协议 auth.json', placement),
    hint: actionHint(codex.reason, canLoginSession(account), '需要邮箱和密码', '不依赖浏览器，使用协议 OAuth 产出 auth.json'),
    icon: <CodexIcon size={14} />,
    disabled: busy || !codex.enabled || !canLoginSession(account),
    onClick: () => onAuth(account),
  }];
}

function utilityActions(catalog: GptActionCatalog | undefined, account: Account, busy: boolean, refreshing: boolean, onRefresh: (account: Account) => Promise<void>, onProbe: (account: Account) => void): ActionButtonDescriptor[] {
  const placement: GptActionPlacement = 'account_header_tools';
  const probe = gptActionAvailability(catalog, GPT_ACTIONS.probeAccount, account, placement);
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
    visible: probe.visible,
    label: gptActionLabel(catalog, GPT_ACTIONS.probeAccount, '探测账号', placement),
    hint: actionHint(probe.reason, canProbeAccount(account), '需要已注册账号', probeAccountHint(account)),
    icon: <Search size={14} />,
    disabled: busy || !probe.enabled || !canProbeAccount(account),
    onClick: () => onProbe(account),
  }];
}

function actionHint(catalogReason: string, allowed: boolean, localReason: string, fallback: string) {
  return catalogReason || (allowed ? fallback : localReason);
}
