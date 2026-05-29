import { Badge, EmptyBlock, api, formatUnix, useQuery } from '@byte-v-forge/common-ui';
import type { ReactNode } from 'react';
import type { AccountProxyChainHop, AccountProxyUsage } from './types';

type AccountProxyUsagesResponse = { usages?: AccountProxyUsage[] };

export function AccountProxyHistoryPanel({ accountID }: { accountID: string }) {
  const query = useQuery({
    queryKey: ['gpt', 'account-proxy-usages', accountID],
    queryFn: () => api<AccountProxyUsagesResponse>(`/api/gpt/accounts/${encodeURIComponent(accountID)}/proxy-usages?limit=100`),
    enabled: !!accountID
  });
  const rows = query.data?.usages || [];

  if (query.isLoading) return <EmptyBlock text="正在加载 IP 使用记录" />;
  if (query.error) return <EmptyBlock text="IP 使用记录加载失败" />;
  if (!rows.length) return <EmptyBlock text="暂无 IP 使用记录" />;

  return <section className="accountProxyHistoryPanel">{rows.map((row) => <ProxyUsageCard key={row.id} row={row} />)}</section>;
}

function ProxyUsageCard({ row }: { row: AccountProxyUsage }) {
  return (
    <article className="proxyUsageCard">
      <header className="proxyUsageHeader">
        <div className="proxyUsageTitle">
          <strong>{formatUnix(row.created_at)}</strong>
          <span>{compact(row.purpose) || '用途未记录'} · 尝试 {row.attempt_index || '-'}</span>
        </div>
        <StatusBadge row={row} />
      </header>
      <div className="proxyUsageGrid">
        <InfoGroup title="出口">
          <InfoLine label="IP" value={row.exit_ip || '-'} mono />
          <InfoLine label="位置" value={locationText(row)} />
        </InfoGroup>
        <InfoGroup title="链路">
          {chainHops(row).map((hop) => <InfoLine key={hop.hop_id || `${hop.role}-${hop.order}`} label={hopLabel(hop)} value={hopValue(hop)} />)}
          {!chainHops(row).length && <InfoLine label="链路" value="-" />}
        </InfoGroup>
        <InfoGroup title="IP 风控">
          <RiskBadge value={row.ip_fraud_check?.risk_level} score={row.ip_fraud_check?.risk_score} />
          <InfoLine label="纯净度" value={scoreText(purityScore(row))} />
        </InfoGroup>
        <InfoGroup title="CF Canary">
          <RiskBadge value={row.edge_access_check?.risk_level} score={row.edge_access_check?.risk_score} />
        </InfoGroup>
        <InfoGroup title="目标连通">
          <ConnectivityBadge row={row} />
        </InfoGroup>
      </div>
      {row.error_message && <p className="proxyUsageError">{row.error_message}</p>}
    </article>
  );
}

function InfoGroup({ title, children }: { title: string; children: ReactNode }) {
  return <section className="proxyUsageGroup"><h4>{title}</h4><div>{children}</div></section>;
}

function InfoLine({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  if (!value) return null;
  return <p className="proxyUsageLine"><span>{label}</span><strong className={mono ? 'monoCell' : undefined} title={value}>{value}</strong></p>;
}

function StatusBadge({ row }: { row: AccountProxyUsage }) {
  const cls = row.accepted ? 'good' : row.error_message ? 'bad' : 'neutral';
  return <Badge className={`badge ${cls}`} variant="outline">{row.accepted ? '通过' : row.error_message ? '失败' : '记录'}</Badge>;
}

function RiskBadge({ value, score }: { value?: string; score?: number }) {
  const label = riskLabel(value);
  return <Badge className={`badge ${riskClass(value || label)}`} variant="outline">{label} · {scoreText(score)}</Badge>;
}

function ConnectivityBadge({ row }: { row: AccountProxyUsage }) {
  return <Badge className={`badge ${row.target_reachable ? 'good' : 'bad'}`} variant="outline">{row.target_reachable ? '可达' : '不可达'}</Badge>;
}

function locationText(row: AccountProxyUsage) {
  return [row.country_code, row.region, row.city].filter(Boolean).join(' / ') || '-';
}

function chainHops(row: AccountProxyUsage) {
  return [...(row.chain?.hops || [])].sort((a, b) => (a.order || 0) - (b.order || 0));
}

function hopLabel(hop: AccountProxyChainHop) {
  const role = compact(hop.role).replace(/^CHAIN HOP ROLE /, '');
  if (role === 'LINE PROXY') return `线路 ${hop.order || ''}`.trim();
  if (role === 'DYNAMIC GATEWAY') return `网关 ${hop.order || ''}`.trim();
  if (role === 'DYNAMIC EXIT') return `出口 ${hop.order || ''}`.trim();
  return `Hop ${hop.order || ''}`.trim();
}

function hopValue(hop: AccountProxyChainHop) {
  const name = hopName(hop);
  const geo = [hop.country_code, hop.region, hop.city].filter(Boolean).join('/');
  const ip = hop.observed_ip ? `IP ${hop.observed_ip}` : '';
  const delay = hop.delay_ms ? `${hop.delay_ms}ms` : '';
  return [name, ip, geo, delay].filter(Boolean).join(' · ') || '-';
}

function hopName(hop: AccountProxyChainHop) {
  if (hop.role?.includes('DYNAMIC_GATEWAY')) {
    return [providerName(hop.provider_id), gatewayName(hop.gateway_display_name, hop.gateway_id)].filter(Boolean).join(' / ');
  }
  if (hop.source_kind?.includes('SUBSCRIPTION')) return [hop.source_display_name, hop.node_display_name || hop.node_id].filter(Boolean).join(' / ');
  return hop.node_display_name || hop.source_display_name || hop.node_id || hop.source_id || '';
}

function riskLabel(value?: string) {
  const label = compact(value).replace(/^IP FRAUD RISK LEVEL /, '').replace(/^EDGE ACCESS RISK LEVEL /, '');
  if (label === 'LOW') return '低风险';
  if (label === 'MEDIUM') return '中风险';
  if (label === 'HIGH') return '高风险';
  if (label === 'CRITICAL') return '严重风险';
  if (label === 'UNSUPPORTED') return '不支持';
  if (label === 'UNKNOWN' || label === 'UNSPECIFIED') return '未知';
  return label || '-';
}

function riskClass(label: string) {
  if (/BLOCK|CHALLENGE|CRITICAL|HIGH/.test(label)) return 'bad';
  if (/MEDIUM/.test(label)) return 'mid';
  if (/LOW/.test(label)) return 'good';
  return 'neutral';
}

function scoreText(score?: number) {
  return Number.isFinite(score) ? `${Math.round(Number(score))}/100` : '-';
}

function purityScore(row: AccountProxyUsage) {
  const score = Number(row.ip_fraud_check?.risk_score || 0);
  if (!row.ip_fraud_check?.risk_level && score <= 0) return undefined;
  return Math.max(0, Math.min(100, 100 - score));
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

function compact(value?: string) {
  return (value || '').replace(/^PROXY_/, '').replace(/^GPT_/, '').replaceAll('_', ' ').trim();
}
