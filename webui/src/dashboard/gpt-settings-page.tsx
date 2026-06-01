import { useEffect } from 'react';
import { Save, Settings } from 'lucide-react';
import {
  Badge,
  Button,
  Controller,
  DashboardField,
  Input,
  Label,
  Switch,
  ToastMessage,
  useForm,
  useMutation,
  useQuery,
  useQueryClient,
  useToastMessage,
  WorkspaceToolbar
} from '@byte-v-forge/common-ui';
import type { Control } from '@byte-v-forge/common-ui';
import { getGPTSettings, gptSettingsQueryKey, updateGPTSettings } from './gpt-settings-api';
import { GPTPluginSettingsSection } from './gpt-plugin-settings-section';
import {
  defaultGPTSettingsForm,
  formFromGPTSettings,
  settingsFromForm,
  type GPTSettingsForm
} from './gpt-settings-model';

export function GPTSettingsPage() {
  const toast = useToastMessage();
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: gptSettingsQueryKey, queryFn: getGPTSettings });
  const form = useForm<GPTSettingsForm>({ defaultValues: defaultGPTSettingsForm });
  const showError = toast.showError;
  const mutation = useMutation({
    mutationFn: (values: GPTSettingsForm) => updateGPTSettings(settingsFromForm(values)),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: gptSettingsQueryKey });
      toast.showOK('GPT配置已保存');
    },
    onError: toast.showError
  });

  useEffect(() => {
    if (query.data?.settings) form.reset(formFromGPTSettings(query.data.settings, query.data.plugin_schemas));
  }, [form, query.data?.plugin_schemas, query.data?.settings]);
  useEffect(() => {
    if (query.error) showError(query.error);
  }, [query.error, showError]);
  const preflightEnabled = form.watch('enabled');
  const pluginCount = query.data?.plugin_schemas?.length || 0;

  return (
    <>
      <ToastMessage toast={toast.toast} />
      <form className="flex min-h-0 flex-1 flex-col" onSubmit={form.handleSubmit((values) => mutation.mutate(values))}>
        <WorkspaceToolbar
          title={<span className="inline-flex items-center gap-2"><Settings size={16} />GPT配置</span>}
          meta={`${pluginCount} 个插件配置`}
          actions={(
            <Button aria-label="保存" disabled={mutation.isPending || query.isLoading} size="icon-sm" type="submit">
              <Save size={14} />
            </Button>
          )}
        />
        <div className="overflow-auto bg-muted/30 p-4">
          <div className="flex flex-wrap items-start gap-3">
            <section className="grid w-[360px] max-w-full flex-none gap-3 rounded-xl border border-border/70 bg-background p-4 shadow-sm">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <h3 className="m-0 text-sm font-semibold">代理预检</h3>
                  <p className="m-0 mt-1 text-xs leading-5 text-muted-foreground">动态代理质量校验。</p>
                </div>
                <Badge aria-label={preflightEnabled ? '启用' : '停用'} className="h-5 w-5 rounded-full px-0" variant={preflightEnabled ? 'default' : 'secondary'}>
                  <span aria-hidden="true" className="size-1.5 rounded-full bg-current" />
                </Badge>
              </div>
              <BooleanField control={form.control} name="enabled" label="启用预检" />
              {preflightEnabled && (
                <div className="grid gap-3 rounded-lg border border-border/70 bg-muted/30 p-3">
                  <DashboardField label="纯净值阈值">
                    <Input min={0} max={100} type="number" {...form.register('minIPPurityScore', { valueAsNumber: true })} />
                  </DashboardField>
                  <DashboardField label="最大代理尝试次数">
                    <Input min={1} max={50} type="number" {...form.register('maxProxyAttempts', { valueAsNumber: true })} />
                  </DashboardField>
                  <BooleanField control={form.control} name="requireResidential" label="要求住宅" />
                  <BooleanField control={form.control} name="cfCanaryEnabled" label="CF canary" />
                </div>
              )}
            </section>
            <GPTPluginSettingsSection form={form} schemas={query.data?.plugin_schemas || []} />
          </div>
        </div>
      </form>
    </>
  );
}

function BooleanField({ control, name, label }: {
  control: Control<GPTSettingsForm>;
  name: 'enabled' | 'requireResidential' | 'cfCanaryEnabled';
  label: string;
}) {
  return (
    <Controller control={control} name={name} render={({ field }) => (
      <Label className="flex min-h-11 items-center justify-between gap-3 rounded-lg border border-border/70 bg-background p-3">
        <span>{label}</span>
        <Switch checked={!!field.value} onCheckedChange={field.onChange} />
      </Label>
    )} />
  );
}
