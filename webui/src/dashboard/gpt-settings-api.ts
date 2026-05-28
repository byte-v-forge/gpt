import { api } from '@byte-v-forge/common-ui';
import type {
  GetGPTSettingsResponse,
  GPTSettings,
  UpdateGPTSettingsResponse
} from '../proto/orchestrator_settings';

export const gptSettingsQueryKey = ['gpt', 'settings'] as const;

export function getGPTSettings() {
  return api<GetGPTSettingsResponse>('/api/gpt/settings');
}

export function updateGPTSettings(settings: GPTSettings) {
  return api<UpdateGPTSettingsResponse>('/api/gpt/settings', {
    method: 'PUT',
    body: JSON.stringify({ settings })
  });
}
