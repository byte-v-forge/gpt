import { useEffect } from 'react';
import { Save, Settings } from 'lucide-react';
import {
  Button,
  Controller,
  DashboardField,
  Input,
  Label,
  PanelHeader,
  ToastMessage,
  useForm,
  useMutation,
  useQuery,
  useQueryClient,
  useToastMessage
} from '@byte-v-forge/common-ui';
import type { Control } from '@byte-v-forge/common-ui';
import { getGPTSettings, gptSettingsQueryKey, updateGPTSettings } from './gpt-settings-api';
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
  const mutation = useMutation({
    mutationFn: (values: GPTSettingsForm) => updateGPTSettings(settingsFromForm(values)),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: gptSettingsQueryKey });
      toast.showOK('GPT配置已保存');
    },
    onError: toast.showError
  });

  useEffect(() => {
    if (query.data?.settings) form.reset(formFromGPTSettings(query.data.settings));
  }, [form, query.data?.settings]);
  useEffect(() => {
    if (query.error) toast.showError(query.error);
  }, [query.error, toast.showError]);
  const preflightEnabled = form.watch('enabled');

  return (
    <>
      <ToastMessage toast={toast.toast} />
      <form className="flex min-h-0 flex-1 flex-col" onSubmit={form.handleSubmit((values) => mutation.mutate(values))}>
        <PanelHeader title="GPT配置" icon={<Settings size={16} />}>
          <Button disabled={mutation.isPending || query.isLoading} size="sm" type="submit">
            <Save size={14} /> 保存
          </Button>
        </PanelHeader>
        <div className="grid gap-4 overflow-auto bg-[var(--surface-soft)] p-4">
          <section className="grid gap-3 rounded-lg border border-[var(--border-soft)] bg-[var(--surface)] p-4">
            <div>
              <h3 className="m-0 text-sm font-semibold">代理预检</h3>
              <p className="m-0 text-xs text-muted-foreground">获取动态代理后，按配置校验 IP 纯净度和 Cloudflare 可访问性。</p>
            </div>
            <BooleanField control={form.control} name="enabled" label="启用预检" />
            {preflightEnabled && (
              <div className="grid gap-3 rounded-md border border-[var(--border-soft)] bg-[var(--surface-soft)] p-3 md:grid-cols-2">
                <DashboardField label="纯净值阈值">
                  <Input min={90} max={100} type="number" {...form.register('minIPPurityScore', { valueAsNumber: true })} />
                </DashboardField>
                <DashboardField label="最大代理尝试次数">
                  <Input min={1} max={50} type="number" {...form.register('maxProxyAttempts', { valueAsNumber: true })} />
                </DashboardField>
                <BooleanField control={form.control} name="requireResidential" label="要求住宅" />
                <BooleanField control={form.control} name="cfCanaryEnabled" label="CF canary" />
              </div>
            )}
          </section>
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
      <Label className="flex items-center gap-2 rounded-md border border-[var(--border-soft)] p-3">
        <Input className="size-4" checked={!!field.value} type="checkbox" onChange={(event) => field.onChange(event.target.checked)} />
        <span>{label}</span>
      </Label>
    )} />
  );
}
