import { Badge } from '@byte-v-forge/common-ui';
import type { ProxyChainHop } from '@byte-v-forge/common-ui/proto/byte/v/forge/contracts/proxyruntime/v1/proxy_runtime';

export function ProxyChainHops({ hops }: { hops: ProxyChainHop[] }) {
  if (!hops.length) return <p className="proxyUsageLine"><span>链路</span><strong>-</strong></p>;
  return <div className="proxyChainHopList">{hops.map((hop) => <ProxyChainHopCard key={hop.hop_id || `${hop.role}-${hop.order}`} hop={hop} />)}</div>;
}

function ProxyChainHopCard({ hop }: { hop: ProxyChainHop }) {
  const role = hopRole(hop);
  return (
    <article className={`proxyChainHop ${role.className}`}>
      <header className="proxyChainHopHeader">
        <Badge className={`badge proxyChainRoleBadge ${role.className}`} variant="outline">{role.label}</Badge>
        <strong className="proxyChainHopTitle" title={hopTitle(hop)}>{hopTitle(hop)}</strong>
      </header>
      <div className="proxyChainHopMeta">
        <MetaLine label="来源" value={sourceText(hop)} />
        <MetaLine label="节点" value={nodeText(hop)} />
        <MetaLine label="节点 IP" value={hop.observed_ip} mono />
        <MetaLine label="位置" value={geoText(hop)} />
        <MetaLine label="延迟" value={hop.delay_ms ? `${hop.delay_ms}ms` : ''} />
      </div>
    </article>
  );
}

function MetaLine({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  if (!value) return null;
  return <p><span>{label}</span><strong className={mono ? 'monoCell' : undefined} title={value}>{value}</strong></p>;
}

function hopRole(hop: ProxyChainHop) {
  const role = roleText(hop);
  if (role.includes('LINE_PROXY')) return { label: '线路代理', className: 'line' };
  if (role.includes('DYNAMIC_GATEWAY')) return { label: '动态网关', className: 'gateway' };
  if (role.includes('DYNAMIC_EXIT')) return { label: '动态出口', className: 'exit' };
  return { label: `Hop ${hop.order || ''}`.trim(), className: 'neutral' };
}

function hopTitle(hop: ProxyChainHop) {
  if (roleText(hop).includes('LINE_PROXY')) return [hop.source_display_name, hop.node_display_name || hop.node_id].filter(Boolean).join(' / ') || '线路代理';
  if (roleText(hop).includes('DYNAMIC_GATEWAY')) return [providerName(hop.provider_id), gatewayName(hop.gateway_display_name, hop.gateway_id)].filter(Boolean).join(' / ') || '动态网关';
  if (roleText(hop).includes('DYNAMIC_EXIT')) return [providerName(hop.provider_id), '最终出口'].filter(Boolean).join(' / ') || '动态出口';
  return hop.node_display_name || hop.source_display_name || hop.node_id || hop.source_id || '-';
}

function sourceText(hop: ProxyChainHop) {
  if (sourceKindText(hop).includes('SUBSCRIPTION')) return hop.source_display_name || hop.source_id || '订阅线路';
  if (sourceKindText(hop).includes('FIXED')) return hop.source_display_name || hop.source_id || '固定代理';
  if (sourceKindText(hop).includes('DYNAMIC')) return providerName(hop.provider_id) || '动态 IP';
  return hop.source_display_name || hop.source_id || providerName(hop.provider_id);
}

function nodeText(hop: ProxyChainHop) {
  if (roleText(hop).includes('DYNAMIC_GATEWAY')) return gatewayName(hop.gateway_display_name, hop.gateway_id);
  if (roleText(hop).includes('DYNAMIC_EXIT')) return '最终出口';
  return hop.node_display_name || hop.node_id || '';
}

function geoText(hop: ProxyChainHop) {
  return [hop.country_code, hop.region, hop.city].filter(Boolean).join(' / ');
}


function roleText(hop: ProxyChainHop) {
  return String(hop.role || '');
}

function sourceKindText(hop: ProxyChainHop) {
  return String(hop.source_kind || '');
}

function providerName(value?: string) {
  const id = (value || '').toLowerCase();
  if (id.includes('1024')) return '1024Proxy';
  if (id.includes('b2')) return 'B2Proxy';
  if (id.includes('cliproxy')) return 'CliProxy';
  return value || '';
}

function gatewayName(name?: string, id?: string) {
  if ((id || name || '').toLowerCase() === 'direct') return '';
  if (!name && !id) return '';
  if (!id || name === id) return name || id || '';
  return `${name || id} (${id})`;
}
