import {
  api,
  useQuery
} from '@byte-v-forge/common-ui';
import type { GPTActionCatalogResponse } from '../proto/orchestrator_job';

export const GPT_ACTIONS = {
  register: 'REGISTER',
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
  n8nWorkflow: 'n8n_workflow',
} as const;

export type GptActionID = string;
export type GptCapability = typeof GPT_CAPABILITIES[keyof typeof GPT_CAPABILITIES];
export type GptActionCatalog = GPTActionCatalogResponse;
export type GptActionPlacement = 'account_header_browser' | 'account_header_protocol' | 'account_header_tools' | 'account_row' | 'account_detail' | 'account_bulk';

export function useGptActionCatalog() {
  return useQuery<GPTActionCatalogResponse>({
    queryKey: ['gpt', 'action-catalog'],
    queryFn: () => api<GPTActionCatalogResponse>('/api/gpt/action-catalog'),
    staleTime: 60_000,
  });
}
