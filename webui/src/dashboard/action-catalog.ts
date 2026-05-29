import { api, useQuery } from '@byte-v-forge/common-ui';
import type { GPTActionCatalogResponse, GPTActionDefinition } from '../proto/orchestrator_job';
import type { Account } from './types';

export const GPT_ACTIONS = {
  register: 'REGISTER',
  goPayApp: 'GOPAY_APP',
  goPayPayment: 'GOPAY_PAYMENT',
  goPayQRISPaymentActivate: 'GOPAY_QRIS_PAYMENT_ACTIVATE',
  goPayWAPayment: 'GOPAY_WA_PAYMENT',
  goPayPaymentRebind: 'GOPAY_PAYMENT_REBIND',
  probeAccount: 'PROBE_ACCOUNT',
  loginSession: 'LOGIN_SESSION',
  registerProtocol: 'REGISTER_PROTOCOL',
  loginSessionProtocol: 'LOGIN_SESSION_PROTOCOL',
  codexOAuth: 'CODEX_OAUTH',
  codexOAuthProtocol: 'CODEX_OAUTH_PROTOCOL',
  codexOAuthAddPhone: 'CODEX_OAUTH_ADD_PHONE',
  codexOAuthBatchAddPhone: 'CODEX_OAUTH_BATCH_ADD_PHONE',
} as const;

export const GPT_CAPABILITIES = {
  accountProbe: 'account_probe',
  browserAuth: 'browser_auth',
  protocolAuth: 'protocol_auth',
  registration: 'registration',
  activation: 'activation',
  payment: 'payment',
  login: 'login',
  codexOAuth: 'codex_oauth',
  phoneBinding: 'phone_binding',
  goPay: 'gopay',
  n8nWorkflow: 'n8n_workflow',
} as const;

export type GptActionID = string;
export type GptCapability = typeof GPT_CAPABILITIES[keyof typeof GPT_CAPABILITIES];
export type GptActionCatalog = GPTActionCatalogResponse;
export type GptActionAvailability = { action?: GPTActionDefinition; visible: boolean; enabled: boolean; reason: string };
export type GptActionPlacement = 'account_header_browser' | 'account_header_protocol' | 'account_header_tools' | 'account_row' | 'account_detail' | 'account_bulk' | 'gopay';

export function useGptActionCatalog() {
  return useQuery<GPTActionCatalogResponse>({
    queryKey: ['gpt', 'action-catalog'],
    queryFn: () => api<GPTActionCatalogResponse>('/api/gpt/action-catalog'),
    staleTime: 60_000,
  });
}

export function gptActionAvailability(catalog: GptActionCatalog | undefined, actionID: GptActionID, account?: Account, placement?: GptActionPlacement): GptActionAvailability {
  const action = findGptAction(catalog, actionID);
  if (!action) return { visible: false, enabled: false, reason: '动作未注册' };
  if (placement && !gptActionButton(action, placement)) return { action, visible: false, enabled: false, reason: '' };
  if (!account) return { action, visible: true, enabled: true, reason: '' };
  const status = normalize(account.status);
  const blockedStatuses = action.blocked_account_statuses || [];
  if (blockedStatuses.map(normalize).includes(status)) return { action, visible: true, enabled: false, reason: `账号状态不可用：${account.status || '-'}` };
  const requiredAccountStatuses = action.required_account_statuses || [];
  const requiredStatuses = requiredAccountStatuses.map(normalize);
  if (requiredStatuses.length && !requiredStatuses.includes(status)) return { action, visible: true, enabled: false, reason: `需要账号状态：${requiredAccountStatuses.join('/')}` };
  const missing = (action.required_fields || []).filter((field) => !String((account as unknown as Record<string, unknown>)[field] ?? '').trim());
  if (missing.length) return { action, visible: true, enabled: false, reason: `缺少字段：${missing.join(', ')}` };
  return { action, visible: true, enabled: true, reason: '' };
}

export function workflowStartPath(catalog: GptActionCatalog | undefined, actionID: GptActionID, placement?: GptActionPlacement) {
  const action = findGptAction(catalog, actionID);
  const path = gptActionButton(action, placement)?.start_path || action?.workflow?.start_path || '';
  if (!path) return '';
  return path.startsWith('/api/gpt/') ? path : `/api/gpt${path.startsWith('/') ? path : `/${path}`}`;
}

export function gptActionLabel(catalog: GptActionCatalog | undefined, actionID: GptActionID, fallback: string, placement?: GptActionPlacement) {
  const action = findGptAction(catalog, actionID);
  return gptActionButton(action, placement)?.label || action?.display_name || fallback;
}

export function gptCatalogHasCapability(catalog: GptActionCatalog | undefined, capability: GptCapability) {
  return gptActionsWithCapability(catalog, capability).length > 0;
}

export function gptActionHasCapability(catalog: GptActionCatalog | undefined, actionID: string | undefined, capability: GptCapability) {
  return !!gptActionDefinition(catalog, actionID)?.capabilities?.includes(capability);
}

export function gptActionDefinition(catalog: GptActionCatalog | undefined, actionID: string | undefined) {
  return catalog?.actions.find((item) => item.action_id === actionID);
}

export function gptActionsWithCapability(catalog: GptActionCatalog | undefined, capability: GptCapability) {
  return catalog?.actions.filter((action) => action.capabilities?.includes(capability)).map((action) => action.action_id) || [];
}

function findGptAction(catalog: GptActionCatalog | undefined, actionID: GptActionID) {
  return gptActionDefinition(catalog, actionID);
}

function gptActionButton(action: GPTActionDefinition | undefined, placement?: GptActionPlacement) {
  if (!action) return undefined;
  if (!placement) return action.ui_buttons?.[0];
  return action.ui_buttons?.find((button) => button.placement === placement);
}

function normalize(value: string) {
  return String(value || '').trim().toUpperCase();
}
