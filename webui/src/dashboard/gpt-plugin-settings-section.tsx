import { Badge, Controller, DashboardField, Input, Label, Switch } from '@byte-v-forge/common-ui';
import type { Control, Path, UseFormRegister } from 'react-hook-form';
import { GPTPluginConfigFieldKind } from '../proto/orchestrator_settings';
import type { GPTPluginConfigField, GPTPluginConfigSchema } from '../proto/orchestrator_settings';
import type { GPTSettingsForm } from './gpt-settings-model';

type PluginSettingsFormApi = {
  control: Control<GPTSettingsForm>;
  register: UseFormRegister<GPTSettingsForm>;
};

export function GPTPluginSettingsSection({ form, schemas }: {
  form: PluginSettingsFormApi;
  schemas: GPTPluginConfigSchema[];
}) {
  if (!schemas.length) return null;
  return (
    <>
      {schemas.map((schema) => (
        <section key={schema.plugin_key} className="grid w-[360px] max-w-full flex-none gap-3 rounded-xl border border-border/70 bg-background p-4 shadow-sm">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <h3 className="m-0 truncate text-sm font-semibold">{schema.display_name}</h3>
              <p className="m-0 mt-1 truncate text-xs text-muted-foreground">{schema.plugin_key}</p>
            </div>
            {schema.owner && <Badge variant="secondary">{schema.owner}</Badge>}
          </div>
          <div className="grid gap-3">
            {schema.fields.map((field) => <PluginField key={field.key} form={form} pluginKey={schema.plugin_key} field={field} />)}
          </div>
        </section>
      ))}
    </>
  );
}

function PluginField({ form, pluginKey, field }: {
  form: PluginSettingsFormApi;
  pluginKey: string;
  field: GPTPluginConfigField;
}) {
  const name = pluginConfigPath(pluginKey, field.key);
  if (field.kind === GPTPluginConfigFieldKind.GPT_PLUGIN_CONFIG_FIELD_KIND_BOOLEAN) {
    return (
      <Controller control={form.control} name={name} render={({ field: valueField }) => (
        <Label className="flex min-h-11 items-center justify-between gap-3 rounded-lg border border-border/70 bg-background p-3">
          <span>{field.label}</span>
          <Switch checked={isTruthy(valueField.value)} onCheckedChange={(checked) => valueField.onChange(checked ? 'true' : 'false')} />
        </Label>
      )} />
    );
  }
  return (
    <DashboardField label={field.label}>
      <Input type={inputType(field)} {...form.register(name)} />
      {field.help_text && <p className="mt-1 text-xs leading-5 text-muted-foreground">{field.help_text}</p>}
    </DashboardField>
  );
}

function pluginConfigPath(pluginKey: string, fieldKey: string): Path<GPTSettingsForm> {
  return `pluginConfigs.${pluginKey}.${fieldKey}` as Path<GPTSettingsForm>;
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
