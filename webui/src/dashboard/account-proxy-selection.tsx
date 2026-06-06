import { Badge } from '@byte-v-forge/common-ui';
import type { ProxyDynamicIPSelectionPlan } from '@byte-v-forge/common-ui/proto/byte/v/forge/contracts/proxyruntime/v1/proxy_runtime';

export function ProxyDynamicIPSelection({ plan }: { plan?: ProxyDynamicIPSelectionPlan }) {
  const endpoint = plan?.selected_endpoint;
  if (!endpoint) return <p className="proxyUsageLine"><span>动态IP</span><strong>-</strong></p>;
  return (
    <article className="proxyChainHop exit">
      <header className="proxyChainHopHeader">
        <Badge className="badge proxyChainRoleBadge exit" variant="outline">动态IP</Badge>
        <strong className="proxyChainHopTitle" title={endpoint.endpoint_url || endpoint.endpoint_id || endpoint.provider_id}>
          {providerName(endpoint.provider_id)}
        </strong>
      </header>
      <div className="proxyChainHopMeta">
        <MetaLine label="账号" value={endpoint.provider_account_id} mono />
        <MetaLine label="端点" value={endpoint.endpoint_url || endpoint.endpoint_id} />
        <MetaLine label="区域" value={(endpoint.geo_codes || []).join(' / ')} />
      </div>
    </article>
  );
}

function MetaLine({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  if (!value) return null;
  return <p><span>{label}</span><strong className={mono ? 'monoCell' : undefined} title={value}>{value}</strong></p>;
}

function providerName(value?: string) {
  const id = (value || '').toLowerCase();
  if (id.includes('1024')) return '1024Proxy';
  if (id.includes('b2')) return 'B2Proxy';
  if (id.includes('cliproxy')) return 'CliProxy';
  return value || '-';
}
