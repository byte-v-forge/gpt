import { useEffect, useMemo, useState } from 'react';
import { Plus } from 'lucide-react';
import {
  ActionButtonGroup,
  api,
  Button,
  ControlledInputFieldList,
  ControlledSelectField,
  errorText,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  ToolbarIconButton,
  useForm
} from '@/dashboard/module-kit';
import type { ActionButtonDescriptor, Control, ControlledInputFieldDescriptor, SubmitHandler } from '@/dashboard/module-kit';
import type { MailboxDomain } from './types';

type EmailSource = 'cloudflare' | 'outlook_primary' | 'outlook_alias' | 'manual';
type CreateAccountValues = { source: EmailSource; email: string; password: string; local: string; domain: string };
const emailSourceOptions = [
  { value: 'cloudflare', label: 'Cloudflare 域名' },
  { value: 'outlook_primary', label: 'Outlook 主邮箱' },
  { value: 'outlook_alias', label: 'Outlook 别名' },
  { value: 'manual', label: '手动邮箱' },
];

export function CreateAccountForm({ compact, domains = [], onDone, onError }: {
  compact?: boolean;
  domains?: MailboxDomain[];
  onDone: (message: string) => void;
  onError: (message: string) => void;
}) {
  const cloudflareDomains = useMemo(() => Array.from(new Set(domains
    .filter((domain) => domain.enabled !== false)
    .filter((domain) => String(domain.provider).toLowerCase().includes('cloudflare') || Number(domain.provider) === 2)
    .map((domain) => domain.domain.trim())
    .filter(Boolean))), [domains]);
  const [open, setOpen] = useState(false);
  const [working, setWorking] = useState('');
  const { control, getValues, handleSubmit, reset, setValue, watch } = useForm<CreateAccountValues>({
    defaultValues: { source: 'cloudflare', email: '', password: '', local: '', domain: '' }
  });
  const source = watch('source');
  const activeDomain = watch('domain') || cloudflareDomains[0] || '';
  const accountFields: ControlledInputFieldDescriptor<CreateAccountValues>[] = [{
    id: 'manual-email',
    name: 'email',
    label: '邮箱',
    placeholder: '邮箱',
    inputId: 'create-account-email',
    visible: source === 'manual',
  }, {
    id: 'password',
    name: 'password',
    label: '密码',
    placeholder: '密码，可空',
    type: 'password',
    inputId: 'create-account-password',
  }];
  const footerActions: ActionButtonDescriptor[] = [{
    id: 'create-account',
    label: working ? '提交中' : '创建',
    icon: <Plus className="size-4" />,
    type: 'submit',
    form: 'create-account-form',
    disabled: !!working || (source === 'cloudflare' && !activeDomain),
  }];

  useEffect(() => {
    if (source === 'cloudflare' && !getValues('domain') && cloudflareDomains[0]) {
      setValue('domain', cloudflareDomains[0]);
    }
  }, [cloudflareDomains, getValues, setValue, source]);

  function accountEmail(values: CreateAccountValues) {
    if (values.source === 'manual') return values.email.trim();
    if (values.source === 'outlook_primary' || values.source === 'outlook_alias') return '';
    if (!activeDomain) return '';
    return `${cloudflareLocalPart(values.local)}@${activeDomain}`;
  }

  const createAccount: SubmitHandler<CreateAccountValues> = async (values) => {
    if (values.source === 'cloudflare' && !activeDomain) {
      onError('Cloudflare 域名未配置');
      return;
    }
    const email = accountEmail(values);
    if (values.source === 'manual' && !email) {
      onError('手动邮箱不能为空');
      return;
    }
    setWorking('创建账号');
    try {
      const resp = await api<any>('/api/accounts', {
        method: 'POST',
        body: JSON.stringify({
          email,
          password: values.password,
          email_strategy: values.source
        })
      });
      if (resp.error_message) {
        onError(resp.error_message);
      } else {
        onDone(`创建账号 已提交: ${resp.job_id || resp.account_id || email || 'ok'}`);
        reset({ source: values.source, email: '', password: '', local: '', domain: activeDomain });
        setOpen(false);
      }
    } catch (err) {
      onError(errorText(err));
    } finally {
      setWorking('');
    }
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      {compact ? (
        <ToolbarIconButton label="创建 GPT 账号" tone="primary" icon={<Plus className="size-4" />} onClick={() => setOpen(true)} />
      ) : (
        <Button size="sm" onClick={() => setOpen(true)}><Plus className="size-4" /> 添加账号</Button>
      )}
      <SheetContent className="w-[420px] max-w-[calc(100vw-24px)]">
        <SheetHeader>
          <SheetTitle>创建 GPT 账号</SheetTitle>
          <SheetDescription>选择邮箱来源后创建账号记录。</SheetDescription>
        </SheetHeader>
        <form id="create-account-form" className="grid gap-3 px-4" onSubmit={handleSubmit(createAccount)}>
          <CreateAccountSourceField control={control} />
          {source === 'cloudflare' && <CloudflareEmailFields control={control} domains={cloudflareDomains} domain={activeDomain} />}
          {source === 'outlook_primary' && <StrategyHint text="从 GPT 邮箱池分配一个 Outlook 主邮箱" />}
          {source === 'outlook_alias' && <StrategyHint text="主邮箱不足时允许创建 Outlook 别名" />}
          <ControlledInputFieldList control={control} fields={accountFields} />
        </form>
        <SheetFooter>
          <ActionButtonGroup actions={footerActions} />
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

function StrategyHint({ text }: { text: string }) {
  return <div className="min-h-8 rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">{text}</div>;
}

function CreateAccountSourceField({ control }: { control: Control<CreateAccountValues> }) {
  return <ControlledSelectField control={control} name="source" options={emailSourceOptions} />;
}

function CloudflareEmailFields({ control, domains, domain }: {
  control: Control<CreateAccountValues>;
  domains: string[];
  domain: string;
}) {
  const fields: ControlledInputFieldDescriptor<CreateAccountValues>[] = [{
    id: 'local',
    name: 'local',
    label: '邮箱前缀',
    placeholder: '留空自动生成',
    inputId: 'create-account-local',
  }];

  return (
    <>
      <ControlledInputFieldList control={control} fields={fields} />
      <ControlledSelectField
        control={control}
        name="domain"
        label="域名"
        inputId="create-account-domain"
        placeholder="未配置域名"
        value={domain}
        options={domains.map((item) => ({ value: item, label: item }))}
      />
    </>
  );
}

function randomLocalPart() {
  const bytes = new Uint8Array(6);
  crypto.getRandomValues(bytes);
  return `gpt-${Array.from(bytes).map((byte) => byte.toString(16).padStart(2, '0')).join('')}`;
}

function cloudflareLocalPart(value: string) {
  return (value.trim().split('@')[0] || randomLocalPart()).toLowerCase();
}
