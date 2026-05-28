import { useEffect, useState } from 'react';
import type { ClipboardEvent } from 'react';
import { Copy, Save } from 'lucide-react';
import { Button, ControlledInputControl, Label, useForm } from '@byte-v-forge/common-ui';
import { buttonHint, formatUnix } from '@byte-v-forge/common-ui';
import type { Account } from './types';

export function TokenEditor({ label, field, account, token, expiresAtUnix, loading, showSecrets, onCopy, onSave }: {
  label: string;
  field: 'session_token' | 'access_token';
  account: Account;
  token: string;
  expiresAtUnix: number;
  loading: boolean;
  showSecrets: boolean;
  onCopy: (label: string, value: string) => void;
  onSave: (account: Account, token: string) => Promise<void>;
}) {
  void account;
  const [saving, setSaving] = useState(false);
  const { control, handleSubmit, reset, watch } = useForm<{ token: string }>({ defaultValues: { token: '' } });
  const value = watch('token');
  useEffect(() => reset({ token: showSecrets ? token : '' }), [account.account_id, field, reset, showSecrets, token]);
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
        placeholder={tokenPlaceholder(label, showSecrets, loading, token, expiresAtUnix)}
      />
      <Button className="copyButton" {...buttonHint(`复制 ${label}`)} disabled={!showSecrets || !value.trim()} onClick={() => onCopy(label, value)}><Copy size={14} /></Button>
      <Button type="submit" {...buttonHint(`保存 ${label}`)} disabled={saving || !value.trim() || value.trim() === token.trim()}><Save size={14} /> 保存</Button>
    </form>
  );
}

function tokenPlaceholder(label: string, showSecrets: boolean, loading: boolean, token: string, expiresAtUnix: number) {
  if (!showSecrets) return '敏感信息已隐藏';
  if (loading) return `读取 ${label} Token...`;
  if (!token.trim()) return `${label} Token 未保存或已过期`;
  return expiresAtUnix > 0 ? `有效至 ${formatUnix(expiresAtUnix)}` : `${label} Token 已保存`;
}
