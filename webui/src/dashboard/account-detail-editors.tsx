import { useEffect, useState } from 'react';
import type { ClipboardEvent } from 'react';
import { Copy, Save } from 'lucide-react';
import { Button, ControlledInputControl, Label, useForm } from '@/dashboard/module-kit';
import { buttonHint } from '@/dashboard/module-kit';
import type { Account } from './types';

export function TokenEditor({ label, field, account, showSecrets, onCopy, onSave }: {
  label: string;
  field: 'session_token' | 'access_token';
  account: Account;
  showSecrets: boolean;
  onCopy: (label: string, value: string) => void;
  onSave: (account: Account, token: string) => Promise<void>;
}) {
  const current = account[field] || '';
  const [saving, setSaving] = useState(false);
  const { control, handleSubmit, reset, watch } = useForm<{ token: string }>({ defaultValues: { token: current } });
  const value = watch('token');
  useEffect(() => reset({ token: account[field] || '' }), [account.account_id, account.session_token, account.access_token, field, reset]);
  async function save(values: { token: string }) {
    setSaving(true);
    try {
      await onSave(account, values.token.trim());
    } finally {
      setSaving(false);
    }
  }
  function copyFromInput(event: ClipboardEvent<HTMLInputElement>) {
    if (!value.trim()) return;
    event.preventDefault();
    event.clipboardData.setData('text/plain', value);
  }
  return (
    <form className="editLine" onSubmit={handleSubmit(save)}>
      <Label>{label}</Label>
      <ControlledInputControl
        control={control}
        name="token"
        className="mono"
        type={showSecrets ? 'text' : 'password'}
        onCopy={copyFromInput}
        placeholder={`${label.toLowerCase()} token`}
      />
      <Button className="copyButton" {...buttonHint(`复制 ${label}`)} disabled={!value.trim()} onClick={() => onCopy(label, value)}><Copy size={14} /></Button>
      <Button type="submit" {...buttonHint(`保存 ${label}`)} disabled={saving || value.trim() === current}><Save size={14} /> 保存</Button>
    </form>
  );
}
