import type { GPTProxyPreflightSettings, GPTSettings } from '../proto/orchestrator_settings';

export type GPTSettingsForm = {
  enabled: boolean;
  requireResidential: boolean;
  minIPPurityScore: number;
  cfCanaryEnabled: boolean;
  maxProxyAttempts: number;
};

export const defaultGPTSettingsForm: GPTSettingsForm = {
  enabled: false,
  requireResidential: true,
  minIPPurityScore: 90,
  cfCanaryEnabled: true,
  maxProxyAttempts: 10
};

export function formFromGPTSettings(settings: GPTSettings | undefined): GPTSettingsForm {
  const preflight = settings?.proxy_preflight;
  return {
    enabled: preflight?.enabled ?? defaultGPTSettingsForm.enabled,
    requireResidential: preflight?.require_residential ?? defaultGPTSettingsForm.requireResidential,
    minIPPurityScore: preflight?.min_ip_purity_score || defaultGPTSettingsForm.minIPPurityScore,
    cfCanaryEnabled: preflight?.cf_canary_enabled ?? defaultGPTSettingsForm.cfCanaryEnabled,
    maxProxyAttempts: preflight?.max_proxy_attempts || defaultGPTSettingsForm.maxProxyAttempts
  };
}

export function settingsFromForm(values: GPTSettingsForm): GPTSettings {
  return {
    proxy_preflight: {
      enabled: values.enabled,
      require_residential: values.requireResidential,
      min_ip_purity_score: Number(values.minIPPurityScore) || defaultGPTSettingsForm.minIPPurityScore,
      cf_canary_enabled: values.cfCanaryEnabled,
      max_proxy_attempts: Number(values.maxProxyAttempts) || defaultGPTSettingsForm.maxProxyAttempts
    } satisfies GPTProxyPreflightSettings
  };
}
