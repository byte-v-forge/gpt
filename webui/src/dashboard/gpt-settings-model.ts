import type { GPTPluginConfigSchema, GPTProxyPreflightSettings, GPTSettings } from '../proto/orchestrator_settings';

export type GPTSettingsForm = {
  enabled: boolean;
  requireResidential: boolean;
  minIPPurityScore: number;
  cfCanaryEnabled: boolean;
  maxProxyAttempts: number;
  pluginConfigs: Record<string, Record<string, string>>;
};

export const defaultGPTSettingsForm: GPTSettingsForm = {
  enabled: false,
  requireResidential: true,
  minIPPurityScore: 90,
  cfCanaryEnabled: true,
  maxProxyAttempts: 10,
  pluginConfigs: {}
};

export function formFromGPTSettings(settings: GPTSettings | undefined, schemas: GPTPluginConfigSchema[] = []): GPTSettingsForm {
  const preflight = settings?.proxy_preflight;
  return {
    enabled: preflight?.enabled ?? defaultGPTSettingsForm.enabled,
    requireResidential: preflight?.require_residential ?? defaultGPTSettingsForm.requireResidential,
    minIPPurityScore: preflight?.min_ip_purity_score || defaultGPTSettingsForm.minIPPurityScore,
    cfCanaryEnabled: preflight?.cf_canary_enabled ?? defaultGPTSettingsForm.cfCanaryEnabled,
    maxProxyAttempts: preflight?.max_proxy_attempts || defaultGPTSettingsForm.maxProxyAttempts,
    pluginConfigs: pluginConfigValues(settings, schemas)
  };
}

export function settingsFromForm(values: GPTSettingsForm): GPTSettings {
  return {
    proxy_preflight: {
      enabled: values.enabled,
      require_residential: values.requireResidential,
      min_ip_purity_score: Number(values.minIPPurityScore) || defaultGPTSettingsForm.minIPPurityScore,
      cf_canary_enabled: values.cfCanaryEnabled,
      max_proxy_attempts: Number(values.maxProxyAttempts) || defaultGPTSettingsForm.maxProxyAttempts,
      target_connectivity_urls: ['https://api.openai.com/v1/models']
    } satisfies GPTProxyPreflightSettings,
    plugin_configs: Object.entries(values.pluginConfigs || {}).map(([plugin_key, fieldValues]) => ({
      plugin_key,
      values: Object.fromEntries(Object.entries(fieldValues || {}).map(([key, value]) => [key, String(value ?? '').trim()]))
    }))
  };
}

function pluginConfigValues(settings: GPTSettings | undefined, schemas: GPTPluginConfigSchema[]) {
  const out: Record<string, Record<string, string>> = {};
  for (const schema of schemas) {
    out[schema.plugin_key] = Object.fromEntries(schema.fields.map((field) => [field.key, field.default_value || '']));
  }
  for (const cfg of settings?.plugin_configs || []) {
    if (!cfg.plugin_key) continue;
    out[cfg.plugin_key] = { ...(out[cfg.plugin_key] || {}), ...(cfg.values || {}) };
  }
  return out;
}
