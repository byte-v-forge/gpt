import { Badge, EmptyBlock, api, formatUnix, useQuery } from '@byte-v-forge/common-ui';
import type { ReactNode } from 'react';
import type { AccountProxyUsage } from './types';

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
          <InfoLine label="网络" value={compact(row.network_kind) || '-'} />
          <InfoLine label="匿名" value={compact(row.anonymizer_kind) || '-'} />
        </InfoGroup>
        <InfoGroup title="链路">
          <InfoLine label="动态网关" value={dynamicGateway(row)} />
          <InfoLine label="线路" value={lineProxy(row)} />
          <InfoLine label="会话" value={row.session_id_hash || '-'} mono />
          <InfoLine label="代理" value={proxyEndpoint(row)} mono />
        </InfoGroup>
        <InfoGroup title="IP 风控">
          <RiskBadge value={row.fraud_risk_level} score={row.fraud_risk_score} />
        </InfoGroup>
        <InfoGroup title="CF Canary">
          <RiskBadge value={row.edge_risk_level} score={row.edge_risk_score} />
        </InfoGroup>
      </div>
      {row.error_message && <p className="proxyUsageError">{row.error_message}</p>}
    </article>
  );
}

function InfoGroup({ title, children }: { title: string; children: ReactNode }) {
  return <section className="proxyUsageGroup"><h4>{title}</h4><div>{children}</div></section>;
}

function InfoLine({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return <p className="proxyUsageLine"><span>{label}</span><strong className={mono ? 'monoCell' : undefined} title={value}>{value}</strong></p>;
}

function StatusBadge({ row }: { row: AccountProxyUsage }) {
  const cls = row.accepted ? 'good' : row.error_message ? 'bad' : 'neutral';
  return <Badge className={`badge ${cls}`} variant="outline">{row.accepted ? '通过' : row.error_message ? '失败' : '记录'}</Badge>;
}

function RiskBadge({ value, score }: { value?: string; score?: number }) {
  const label = riskLabel(value);
  const suffix = typeof score === 'number' && score > 0 ? ` ${score}` : '';
  return <Badge className={`badge ${riskClass(label)}`} variant="outline">{label}{suffix}</Badge>;
}

function locationText(row: AccountProxyUsage) {
  return [row.country_code, row.region, row.city].filter(Boolean).join(' / ') || '-';
}

function dynamicGateway(row: AccountProxyUsage) {
  const chain = row.chain;
  return [chain?.dynamic_provider_id, chain?.dynamic_gateway_name || chain?.dynamic_gateway_id].filter(Boolean).join(' / ') || '-';
}

function lineProxy(row: AccountProxyUsage) {
  const chain = row.chain;
  return chain?.line_display_name || chain?.line_node_id || chain?.line_source_id || 'direct';
}

function proxyEndpoint(row: AccountProxyUsage) {
  return [compact(row.proxy_protocol), [row.proxy_host, row.proxy_port].filter(Boolean).join(':')].filter(Boolean).join(' · ') || '-';
}

function riskLabel(value?: string) {
  return compact(value).replace(/^IP FRAUD RISK LEVEL /, '').replace(/^EDGE ACCESS RISK LEVEL /, '') || '-';
}

function riskClass(label: string) {
  if (/BLOCK|CHALLENGE|CRITICAL|HIGH/.test(label)) return 'bad';
  if (/MEDIUM/.test(label)) return 'mid';
  if (/LOW/.test(label)) return 'good';
  return 'neutral';
}

function compact(value?: string) {
  return (value || '').replace(/^PROXY_/, '').replace(/^GPT_/, '').replaceAll('_', ' ').trim();
}
