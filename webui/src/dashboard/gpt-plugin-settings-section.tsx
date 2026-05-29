import { Controller, DashboardField, Input, Label } from '@byte-v-forge/common-ui';
import type { Control } from '@byte-v-forge/common-ui';
import { GPTPluginConfigFieldKind } from '../proto/orchestrator_settings';
import type { GPTPluginConfigField, GPTPluginConfigSchema } from '../proto/orchestrator_settings';
import type { GPTSettingsForm } from './gpt-settings-model';

type PluginSettingsFormApi = {
  control: Control<GPTSettingsForm>;
  register: (name: any, options?: any) => Record<string, unknown>;
};

export function GPTPluginSettingsSection({ form, schemas }: {
  form: PluginSettingsFormApi;
  schemas: GPTPluginConfigSchema[];
}) {
  if (!schemas.length) return null;
  return (
    <section className="grid gap-3 rounded-lg border border-[var(--border-soft)] bg-[var(--surface)] p-4">
      <div>
        <h3 className="m-0 text-sm font-semibold">插件配置</h3>
        <p className="m-0 text-xs text-muted-foreground">私有流程配置从环境变量收敛到这里，由插件声明字段。</p>
      </div>
      <div className="grid gap-4">
        {schemas.map((schema) => (
          <div key={schema.plugin_key} className="grid gap-3 rounded-md border border-[var(--border-soft)] bg-[var(--surface-soft)] p-3">
            <h4 className="m-0 text-sm font-semibold">{schema.display_name}</h4>
            <div className="grid gap-3 md:grid-cols-2">
              {schema.fields.map((field) => <PluginField key={field.key} form={form} pluginKey={schema.plugin_key} field={field} />)}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function PluginField({ form, pluginKey, field }: {
  form: PluginSettingsFormApi;
  pluginKey: string;
  field: GPTPluginConfigField;
}) {
  const name = `pluginConfigs.${pluginKey}.${field.key}`;
  if (field.kind === GPTPluginConfigFieldKind.GPT_PLUGIN_CONFIG_FIELD_KIND_BOOLEAN) {
    return (
      <Controller control={form.control} name={name as any} render={({ field: valueField }) => (
        <Label className="flex items-center gap-2 rounded-md border border-[var(--border-soft)] p-3">
          <Input className="size-4" checked={isTruthy(valueField.value)} type="checkbox" onChange={(event) => valueField.onChange(event.target.checked ? 'true' : 'false')} />
          <span>{field.label}</span>
        </Label>
      )} />
    );
  }
  return (
    <DashboardField label={field.label}>
      <Input type={inputType(field)} {...form.register(name as any)} />
    </DashboardField>
  );
}

function inputType(field: GPTPluginConfigField) {
  switch (field.kind) {
    case GPTPluginConfigFieldKind.GPT_PLUGIN_CONFIG_FIELD_KIND_SECRET:
      return 'password';
    case GPTPluginConfigFieldKind.GPT_PLUGIN_CONFIG_FIELD_KIND_INTEGER:
    case GPTPluginConfigFieldKind.GPT_PLUGIN_CONFIG_FIELD_KIND_DURATION_SECONDS:
      return 'number';
    case GPTPluginConfigFieldKind.GPT_PLUGIN_CONFIG_FIELD_KIND_URL:
      return 'url';
    default:
      return 'text';
  }
}

function isTruthy(value: unknown) {
  return ['true', '1', 'yes', 'on'].includes(String(value ?? '').trim().toLowerCase());
}
