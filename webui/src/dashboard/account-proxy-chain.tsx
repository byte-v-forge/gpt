import { Badge } from '@byte-v-forge/common-ui';
import { EgressHopRole, ProxySourceKind, type EgressHop } from '@byte-v-forge/common-ui/proto/byte/v/forge/contracts/proxyruntime/v1/proxy_runtime';

export function ProxyRouteHops({ hops }: { hops: EgressHop[] }) {
  if (!hops.length) return <p className="proxyUsageLine"><span>链路</span><strong>-</strong></p>;
  return <div className="proxyChainHopList">{hops.map((hop) => <ProxyRouteHopCard key={hop.hop_id || `${hop.role}-${hop.order}`} hop={hop} />)}</div>;
}

function ProxyRouteHopCard({ hop }: { hop: EgressHop }) {
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
        <MetaLine label="节点 IP" value={hopLabel(hop, 'observed_ip')} mono />
        <MetaLine label="位置" value={geoText(hop)} />
        <MetaLine label="延迟" value={hopLabel(hop, 'delay_ms') ? `${hopLabel(hop, 'delay_ms')}ms` : ''} />
      </div>
    </article>
  );
}

function MetaLine({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  if (!value) return null;
  return <p><span>{label}</span><strong className={mono ? 'monoCell' : undefined} title={value}>{value}</strong></p>;
}

function hopRole(hop: EgressHop) {
  if (hop.role === EgressHopRole.EGRESS_HOP_ROLE_FORWARD) return { label: '线路代理', className: 'line' };
  if (hop.role === EgressHopRole.EGRESS_HOP_ROLE_EXIT) return { label: '动态出口', className: 'exit' };
  return { label: `Hop ${hop.order || ''}`.trim(), className: 'neutral' };
}

function hopTitle(hop: EgressHop) {
  if (hop.role === EgressHopRole.EGRESS_HOP_ROLE_FORWARD) return [hopLabel(hop, 'source_display_name'), hopLabel(hop, 'node_display_name') || hopLabel(hop, 'node_id')].filter(Boolean).join(' / ') || '线路代理';
  if (hop.role === EgressHopRole.EGRESS_HOP_ROLE_EXIT) return [providerName(hop.endpoints[0]?.provider_id), gatewayName(hopLabel(hop, 'gateway_display_name'), hopLabel(hop, 'gateway_id'))].filter(Boolean).join(' / ') || '动态出口';
  return hopLabel(hop, 'node_display_name') || hopLabel(hop, 'source_display_name') || hopLabel(hop, 'node_id') || hopLabel(hop, 'source_id') || hop.hop_id || '-';
}

function sourceText(hop: EgressHop) {
  if (sourceKindText(hop) === ProxySourceKind.PROXY_SOURCE_KIND_SUBSCRIPTION) return hopLabel(hop, 'source_display_name') || hopLabel(hop, 'source_id') || '订阅线路';
  if (sourceKindText(hop) === ProxySourceKind.PROXY_SOURCE_KIND_FIXED_PROXY) return hopLabel(hop, 'source_display_name') || hopLabel(hop, 'source_id') || '固定代理';
  if (sourceKindText(hop) === ProxySourceKind.PROXY_SOURCE_KIND_DYNAMIC_IP) return providerName(hop.endpoints[0]?.provider_id) || '动态 IP';
  return hopLabel(hop, 'source_display_name') || hopLabel(hop, 'source_id') || providerName(hop.endpoints[0]?.provider_id);
}

function nodeText(hop: EgressHop) {
  if (hop.role === EgressHopRole.EGRESS_HOP_ROLE_EXIT) return gatewayName(hopLabel(hop, 'gateway_display_name'), hopLabel(hop, 'gateway_id')) || '最终出口';
  return hopLabel(hop, 'node_display_name') || hopLabel(hop, 'node_id') || '';
}

function geoText(hop: EgressHop) {
  return [hopLabel(hop, 'country_code'), hopLabel(hop, 'region'), hopLabel(hop, 'city')].filter(Boolean).join(' / ');
}


function sourceKindText(hop: EgressHop) {
  return hopLabel(hop, 'source_kind');
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

function hopLabel(hop: EgressHop, key: string) {
  for (const endpoint of hop.endpoints || []) {
    const value = String(endpoint.labels?.[key] || '').trim();
    if (value) return value;
  }
  return '';
}
