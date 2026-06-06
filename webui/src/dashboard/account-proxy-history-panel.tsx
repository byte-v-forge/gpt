import { Badge, EmptyBlock, api, formatUnix, useQuery } from '@byte-v-forge/common-ui';
import type { ReactNode } from 'react';
import { ProxyDynamicIPSelection } from './account-proxy-selection';
import type { AccountProxyUsage } from './types';
import type { ListAccountProxyUsagesResponse } from '../proto/orchestrator_account';

export function AccountProxyHistoryPanel({ accountID }: { accountID: string }) {
  const query = useQuery({
    queryKey: ['gpt', 'account-proxy-usages', accountID],
    queryFn: () => api<ListAccountProxyUsagesResponse>(`/api/gpt/accounts/${encodeURIComponent(accountID)}/proxy-usages?limit=100`),
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
        <InfoGroup title="动态IP" className="proxyUsageGroupWide">
          <ProxyDynamicIPSelection plan={row.dynamic_ip_selection} />
        </InfoGroup>
        <InfoGroup title="IP 风控">
          <RiskBadge value={row.ip_fraud_check?.risk_level} score={riskScore(row.ip_fraud_check)} />
          <InfoLine label="评分源" value={fraudProviderText(row.ip_fraud_check) || '历史记录未记录'} />
          <InfoLine label="纯净度" value={scoreText(purityScore(row))} />
        </InfoGroup>
        <InfoGroup title="CF Canary">
          <RiskBadge value={row.edge_access_check?.risk_level} score={riskScore(row.edge_access_check)} />
        </InfoGroup>
        <InfoGroup title="目标连通">
          <ConnectivityBadge row={row} />
        </InfoGroup>
      </div>
      {row.error_message && <p className="proxyUsageError">{row.error_message}</p>}
    </article>
  );
}

function InfoGroup({ title, children, className }: { title: string; children: ReactNode; className?: string }) {
  return (
    <section className={['proxyUsageGroup', className].filter(Boolean).join(' ')}>
      <h4>{title}</h4>
      <div>{children}</div>
    </section>
  );
}

function InfoLine({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  if (!value) return null;
  return <p className="proxyUsageLine"><span>{label}</span><strong className={mono ? 'monoCell' : undefined} title={value}>{value}</strong></p>;
}

function StatusBadge({ row }: { row: AccountProxyUsage }) {
  const cls = row.accepted ? 'good' : row.error_message ? 'bad' : 'neutral';
  return <Badge className={`badge ${cls}`} variant="outline">{row.accepted ? '通过' : row.error_message ? '失败' : '记录'}</Badge>;
}

function RiskBadge({ value, score }: { value?: unknown; score?: number }) {
  const label = riskLabel(value);
  return <Badge className={`badge ${riskClass(compact(value) || label)}`} variant="outline">{label} · {scoreText(score)}</Badge>;
}

function ConnectivityBadge({ row }: { row: AccountProxyUsage }) {
  return <Badge className={`badge ${row.target_reachable ? 'good' : 'bad'}`} variant="outline">{row.target_reachable ? '可达' : '不可达'}</Badge>;
}

function locationText(row: AccountProxyUsage) {
  return [row.country_code, row.region, row.city].filter(Boolean).join(' / ') || '-';
}

function riskLabel(value?: unknown) {
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
  const score = riskScore(row.ip_fraud_check);
  if (typeof score !== 'number' || !Number.isFinite(score)) return undefined;
  return Math.max(0, Math.min(100, 100 - score));
}

function riskScore(check?: { risk_level?: unknown; risk_score?: number }) {
  const score = Number(check?.risk_score);
  if (Number.isFinite(score)) return score;
  const label = compact(check?.risk_level);
  if (label.includes('LOW')) return 0;
  return undefined;
}

function fraudProviderText(check?: { provider_display_name?: string; provider_id?: string }) {
  return check?.provider_display_name || providerLabel(check?.provider_id) || '';
}

function providerLabel(value?: string) {
  const id = (value || '').toLowerCase();
  if (id === 'ipqualityscore') return 'IPQualityScore';
  if (id === 'abuseipdb') return 'AbuseIPDB';
  if (id === 'ipapi') return 'ipapi.is';
  if (id === 'ip-api-com') return 'IP-API.com';
  if (id === 'ip2location') return 'IP2Location.io';
  return value || '';
}

function compact(value?: unknown) {
  return String(value || '').replace(/^PROXY_/, '').replace(/^GPT_/, '').replaceAll('_', ' ').trim();
}
