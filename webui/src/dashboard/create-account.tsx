import { useEffect, useMemo, useState } from 'react';
import { Plus } from 'lucide-react';
import { ActionButtonGroup, api, Button, ControlledInputFieldList, ControlledSelectField, DashboardDialog, errorText, ToolbarIconButton, useAsyncActionRunner, useForm } from '@byte-v-forge/common-ui';
import type { ActionButtonDescriptor, Control, ControlledInputFieldDescriptor, SubmitHandler } from '@byte-v-forge/common-ui';
import { accountEmail, defaultMailboxChoice, domainsForProvider, emailStrategyForValues, mailboxChoiceOptions, mailboxProviderOptions, type CreateAccountValues } from './create-account-options';
import { CreateAccountGeoFields } from './create-account-geo-fields';
import { useCreateAccountMailboxData } from './create-account-mailbox-data';
import { randomRegionForCountry, regionOptionsForCountry } from './geo-options';
import { requireAccount } from './account-response';
import type { CreateGPTAccountRequest, CreateGPTAccountResponse } from '../proto/orchestrator_account';
import { accountCarrierID } from '@byte-v-forge/common-ui';

const defaultCountryCode = 'JP';

export function CreateAccountForm({ compact, onDone, onError }: { compact?: boolean; onDone: (message: string) => void; onError: (message: string) => void }) {
  const [open, setOpen] = useState(false);
  const runner = useAsyncActionRunner();
  const { domains, providerCapabilities } = useCreateAccountMailboxData(open);
  const { control, getValues, handleSubmit, reset, setValue, watch } = useForm<CreateAccountValues>({ defaultValues: createDefaultValues() });
  const providerOptions = useMemo(() => mailboxProviderOptions(providerCapabilities, domains), [providerCapabilities, domains]);
  const selectedProviderKey = watch('provider_key');
  const providerKey = selectedProviderKey || providerOptions[0]?.value || 'manual';
  const choiceOptions = useMemo(() => mailboxChoiceOptions(providerKey, domains), [providerKey, domains]);
  const mailboxChoice = (watch('mailbox_choice') || defaultMailboxChoice(providerKey, domains)) as CreateAccountValues['mailbox_choice'];
  const activeDomains = useMemo(() => domainsForProvider(domains, providerKey), [domains, providerKey]);
  const activeDomain = watch('domain') || activeDomains[0] || '';
  const countryCode = watch('country_code');
  const regionOptions = useMemo(() => regionOptionsForCountry(countryCode), [countryCode]);
  const requiresDomain = mailboxChoice === 'domain';
  const isManual = mailboxChoice === 'manual';

  useEffect(() => {
    if (!choiceOptions.some((option) => option.value === mailboxChoice && !option.disabled)) {
      setValue('mailbox_choice', defaultMailboxChoice(providerKey, domains));
    }
  }, [choiceOptions, domains, mailboxChoice, providerKey, setValue]);

  useEffect(() => {
    if (requiresDomain && activeDomains.length && !activeDomains.includes(getValues('domain'))) {
      setValue('domain', activeDomains[0]);
    }
  }, [activeDomains, getValues, requiresDomain, setValue]);

  useEffect(() => {
    const region = getValues('region');
    if (!regionOptions.length) {
      if (region) setValue('region', '');
      return;
    }
    if (!regionOptions.some((option) => option.value === region)) setValue('region', randomRegionForCountry(countryCode));
  }, [countryCode, getValues, regionOptions, setValue]);

  const createAccount: SubmitHandler<CreateAccountValues> = async (values) => {
    const effective = {
      ...values,
      provider_key: providerKey,
      mailbox_choice: mailboxChoice,
      domain: activeDomain
    };
    if (requiresDomain && !activeDomain) return onError('未配置可用域名');
    const email = accountEmail(effective, activeDomain);
    if (isManual && !email) return onError('邮箱不能为空');
    await runner.tryRun('创建账号', async () => {
      const payload: CreateGPTAccountRequest = {
        account_id: '',
        email,
        password: values.password,
        country_code: values.country_code,
        region: values.region,
        email_strategy: emailStrategyForValues(effective)
      };
      const resp = await api<CreateGPTAccountResponse>('/api/gpt/accounts', {
        method: 'POST',
        body: JSON.stringify(payload)
      });
      const account = requireAccount(resp.account);
      onDone(`创建账号 已提交: ${accountCarrierID(account) || email || 'ok'}`);
      reset({
        ...createDefaultValues(),
        provider_key: providerKey,
        mailbox_choice: mailboxChoice,
        domain: activeDomain
      });
      setOpen(false);
    }, { onError: (err) => onError(errorText(err)) });
  };

  return (
    <>
      {compact ? (
        <ToolbarIconButton label="创建 GPT 账号" tone="primary" icon={<Plus className="size-4" />} onClick={() => setOpen(true)} />
      ) : (
        <Button size="sm" onClick={() => setOpen(true)}>
          <Plus className="size-4" /> 添加账号
        </Button>
      )}
      <DashboardDialog open={open} title="创建 GPT 账号" description="选择 Mailbox Provider、邮箱类型和地区。" size="sm" footer={<ActionButtonGroup actions={dialogActions(runner.activeKey, requiresDomain && !activeDomain)} />} onOpenChange={setOpen}>
        <form id="create-account-form" className="grid gap-3" onSubmit={handleSubmit(createAccount)}>
          <CreateAccountMailboxFields control={control} providerOptions={providerOptions} providerKey={providerKey} choiceOptions={choiceOptions} mailboxChoice={mailboxChoice} domains={activeDomains} activeDomain={activeDomain} showDomain={requiresDomain} showEmail={isManual} />
          <div className="grid gap-2 sm:grid-cols-2">
            <CreateAccountGeoFields control={control} regionOptions={regionOptions} />
          </div>
          <ControlledInputFieldList control={control} fields={passwordFields} />
        </form>
      </DashboardDialog>
    </>
  );
}

function CreateAccountMailboxFields({ control, providerOptions, providerKey, choiceOptions, mailboxChoice, domains, activeDomain, showDomain, showEmail }: { control: Control<CreateAccountValues>; providerOptions: ReturnType<typeof mailboxProviderOptions>; providerKey: string; choiceOptions: ReturnType<typeof mailboxChoiceOptions>; mailboxChoice: CreateAccountValues['mailbox_choice']; domains: string[]; activeDomain: string; showDomain: boolean; showEmail: boolean }) {
  return (
    <>
      <div className="grid gap-2 sm:grid-cols-2">
        <ControlledSelectField control={control} name="provider_key" label="Mailbox Provider" value={providerKey} options={providerOptions} />
        <ControlledSelectField control={control} name="mailbox_choice" label="邮箱类型" value={mailboxChoice} options={choiceOptions} />
      </div>
      {showDomain && (
        <div className="grid gap-2 sm:grid-cols-[1fr_1.25fr]">
          <ControlledInputFieldList control={control} fields={localFields} />
          <ControlledSelectField control={control} name="domain" label="域名" inputId="create-account-domain" value={activeDomain} placeholder="未配置域名" options={domains.map((item) => ({ value: item, label: item }))} />
        </div>
      )}
      {showEmail && <ControlledInputFieldList control={control} fields={manualEmailFields} />}
    </>
  );
}

function dialogActions(working: string, disabled: boolean): ActionButtonDescriptor[] {
  return [
    {
      id: 'create-account',
      label: working ? '提交中' : '创建',
      icon: <Plus className="size-4" />,
      type: 'submit',
      form: 'create-account-form',
      disabled: !!working || disabled
    }
  ];
}

const localFields: ControlledInputFieldDescriptor<CreateAccountValues>[] = [
  {
    id: 'local',
    name: 'local',
    label: '邮箱前缀',
    placeholder: '留空自动生成',
    inputId: 'create-account-local'
  }
];
const manualEmailFields: ControlledInputFieldDescriptor<CreateAccountValues>[] = [
  {
    id: 'manual-email',
    name: 'email',
    label: '邮箱',
    placeholder: '邮箱',
    inputId: 'create-account-email'
  }
];
const passwordFields: ControlledInputFieldDescriptor<CreateAccountValues>[] = [
  {
    id: 'password',
    name: 'password',
    label: '密码',
    placeholder: '密码，可空',
    type: 'password',
    inputId: 'create-account-password'
  }
];

function createDefaultValues(): CreateAccountValues {
  return {
    provider_key: '',
    mailbox_choice: 'domain',
    email: '',
    password: '',
    local: '',
    domain: '',
    country_code: defaultCountryCode,
    region: randomRegionForCountry(defaultCountryCode)
  };
}
