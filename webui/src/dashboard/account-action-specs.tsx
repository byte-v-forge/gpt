import { KeyRound, Play, Search } from 'lucide-react';
import type { AccountCatalogActionBase, RowActionDescriptor } from '@byte-v-forge/common-ui';
import { GPT_ACTIONS, type GptActionID, type GptActionPlacement } from './action-catalog';
import { CodexIcon } from './brand-icons';
import { canLoginSession, canProbeAccount, canRegister, loginActionHint, probeAccountHint } from './account-utils';
import type { Account } from './types';

export type AccountWorkflowRunner = (actionID: GptActionID, account: Account, payload?: Record<string, unknown>, placement?: GptActionPlacement, fallbackLabel?: string) => void | Promise<void>;

export type AccountWorkflowActionSpec = AccountCatalogActionBase<Account, GptActionID> & {
  id: string;
  kind?: RowActionDescriptor['kind'];
};

export function accountWorkflowActionProps(spec: AccountWorkflowActionSpec, run: AccountWorkflowRunner) {
  return { ...spec, onClick: (account: Account) => run(spec.actionID, account) };
}

export const ACCOUNT_HEADER_BROWSER_ACTIONS: AccountWorkflowActionSpec[] = [
  {
    id: 'browser-register',
    actionID: GPT_ACTIONS.register,
    fallbackLabel: '浏览器注册',
    icon: <Play size={14} />,
    allowed: canRegister,
    disabledReason: '当前账号不可注册',
    hint: '使用浏览器自动化注册账号',
  },
  {
    id: 'browser-login-session',
    actionID: GPT_ACTIONS.loginSession,
    fallbackLabel: '浏览器登录',
    icon: <KeyRound size={14} />,
    allowed: canLoginSession,
    disabledReason: '需要邮箱和密码',
    hint: (value) => `${loginActionHint(value)}（浏览器）`,
  },
  {
    id: 'browser-codex-oauth',
    actionID: GPT_ACTIONS.codexOAuth,
    fallbackLabel: '浏览器 auth.json',
    icon: <CodexIcon size={14} />,
    allowed: canLoginSession,
    disabledReason: '需要邮箱和密码',
    hint: '浏览器 OAuth 登录并产出 auth.json；遇到 add phone 页面会停下报错',
  },
];

export const ACCOUNT_HEADER_PROTOCOL_ACTIONS: AccountWorkflowActionSpec[] = [
  {
    id: 'protocol-register',
    actionID: GPT_ACTIONS.registerProtocol,
    fallbackLabel: '协议注册',
    icon: <Play size={14} />,
    allowed: canRegister,
    disabledReason: '当前账号不可注册',
    hint: '不依赖浏览器，使用协议注册账号',
  },
  {
    id: 'protocol-login-session',
    actionID: GPT_ACTIONS.loginSessionProtocol,
    fallbackLabel: '协议登录',
    icon: <KeyRound size={14} />,
    allowed: canLoginSession,
    disabledReason: '需要邮箱和密码',
    hint: (value) => `${loginActionHint(value)}（协议）`,
  },
  {
    id: 'protocol-codex-oauth',
    actionID: GPT_ACTIONS.codexOAuthProtocol,
    fallbackLabel: '协议 auth.json',
    icon: <CodexIcon size={14} />,
    allowed: canLoginSession,
    disabledReason: '需要邮箱和密码',
    hint: '不依赖浏览器，使用协议 OAuth 产出 auth.json',
  },
];

export const ACCOUNT_TOOL_WORKFLOW_ACTIONS: AccountWorkflowActionSpec[] = [
  {
    id: 'probe-account',
    actionID: GPT_ACTIONS.probeAccount,
    fallbackLabel: '探测账号',
    icon: <Search size={14} />,
    allowed: canProbeAccount,
    disabledReason: '需要已注册账号',
    hint: probeAccountHint,
  },
];

export const ACCOUNT_ROW_ACTIONS: AccountWorkflowActionSpec[] = [
  {
    id: 'row-protocol-register',
    actionID: GPT_ACTIONS.registerProtocol,
    fallbackLabel: '注册',
    icon: <Play size={14} />,
    allowed: canRegister,
    disabledReason: '当前账号不可注册',
    hint: '协议注册',
    kind: 'secondary',
  },
];
