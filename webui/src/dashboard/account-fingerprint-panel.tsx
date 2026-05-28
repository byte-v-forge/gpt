import { EmptyBlock, errorText, formatUnix, KVList, useQuery } from '@byte-v-forge/common-ui';
import { api } from '@byte-v-forge/common-ui';
import type { KVDescriptor } from '@byte-v-forge/common-ui';
import type { AccountBrowserFingerprint } from './types';

export function AccountFingerprintPanel({ accountID, onCopy }: {
  accountID: string;
  onCopy: (label: string, value: string) => void;
}) {
  const query = useQuery({
    queryKey: ['gpt', 'account-fingerprint', accountID],
    queryFn: () => api<AccountBrowserFingerprint>(`/api/gpt/accounts/${encodeURIComponent(accountID)}/fingerprint`),
    enabled: !!accountID,
    retry: false
  });

  if (query.isLoading) return <EmptyBlock text="正在加载指纹信息" />;
  if (query.error) return <EmptyBlock text={notFound(query.error) ? '尚未生成账号指纹' : errorText(query.error)} />;
  if (!query.data) return <EmptyBlock text="暂无指纹信息" />;

  return <section className="accountFingerprintPanel"><KVList items={fingerprintFields(query.data)} onCopy={onCopy} /></section>;
}

function fingerprintFields(item: AccountBrowserFingerprint): KVDescriptor[] {
  return [
    field('browser-template', '浏览器模板', item.browser_profile_template),
    field('browser', '浏览器', [item.browser_family, item.browser_major_version].filter(Boolean).join(' ')),
    field('os', '系统', item.os_family),
    field('tls-family', 'TLS 指纹族', item.tls_profile_family, true),
    field('tls-variant', 'TLS 变体', item.tls_fingerprint_variant, true),
    field('ua', 'User-Agent', item.user_agent, true),
    field('accept-language', 'Accept-Language', item.accept_language),
    field('language', 'Language', item.language),
    field('locale', 'Locale', item.locale),
    field('timezone', 'Timezone', item.timezone),
    field('device', 'OAI Device ID', item.device_id, true),
    { id: 'created', label: '创建时间', value: formatUnix(item.created_at || 0) },
    { id: 'updated', label: '更新时间', value: formatUnix(item.updated_at || 0) }
  ].filter((row) => row.value);
}

function field(id: string, label: string, value: string, mono = false): KVDescriptor {
  return { id, label, value, copyValue: value, copyDisabled: !value, mono };
}

function notFound(error: unknown) {
  return errorText(error).toLowerCase().includes('not generated');
}
