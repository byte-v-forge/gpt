import { AlertTriangle, Clock, ShieldCheck } from 'lucide-react';
import { compactCellError, formatUnix } from '@byte-v-forge/common-ui';
import { formatJobTime } from './job-utils';
import type { GptActionCatalog } from './action-catalog';
import { actionText } from './labels';
import type { DisplayLabelMap, GoPayDashboardStateResponse, GoPayUserStatusResponse, Job } from './types';

export function GoPayStatusCard({ actionCatalog, state, currentJob, loading }: { actionCatalog?: GptActionCatalog; state: GoPayDashboardStateResponse | null; currentJob?: Job; loading: boolean }) {
  const status = state?.status;
  const error = state?.error_message || state?.wa_phone_error_message || status?.error_message || '';
  const stage = String(status?.stage || '').trim();
  const tone = error ? 'bad' : status?.token_present && stage === 'ready' ? 'good' : 'mid';
  const icon = loading ? <Clock size={17} /> : error ? <AlertTriangle size={17} /> : <ShieldCheck size={17} />;
  const title = loading ? '正在刷新' : error ? 'GoPay 状态异常' : status ? goPayStateStageText(stage) : '未加载';
  const facts = goPayFacts(state, currentJob);

  return (
    <section className={`goPayStatusCard ${tone}`}>
      <div className="goPayStatusHead">
        {icon}
        <div>
          <strong>{title}</strong>
          <span title={error || facts.join(' · ')}>{error ? compactCellError(error) : facts.join(' · ')}</span>
        </div>
      </div>
      {currentJob && (
        <div className="goPayCurrentFlow">
          <span>当前流程</span>
          <strong>{actionText(currentJob.action, actionCatalog)}</strong>
          <em>{currentJob.last_step || currentJob.status}</em>
        </div>
      )}
    </section>
  );
}

function goPayFacts(state: GoPayDashboardStateResponse | null, currentJob?: Job) {
  const status = state?.status;
  const values = [
    `WA ${state?.wa_phone || '-'}`,
    `GoPay ${status?.phone || '-'}`,
    status?.token_present ? 'Token 已保存' : '无 Token',
    status?.pin_setup ? 'PIN 已设置' : 'PIN 未确认',
    status ? goPayBalanceText(status) : '余额 -',
    latestGoPayOtpWindow(status) || ''
  ].filter(Boolean);
  if (currentJob?.updated_at) values.push(`流程更新 ${formatJobTime(currentJob.updated_at)}`);
  return values;
}

function goPayStateStageText(stage: string) {
  const labels: DisplayLabelMap = {
    ready: 'GoPay ready',
    login_otp_sent: '等待登录 OTP',
    signup_otp_sent: '等待注册 OTP',
    signup_pin_otp_sent: '等待 PIN OTP',
    change_phone_otp_sent: '等待换绑 OTP',
    deactivation_otp_sent: '等待注销 OTP'
  };
  return labels[stage] || stage || '未保存状态';
}

function goPayBalanceText(status: NonNullable<GoPayUserStatusResponse['status']>) {
  const currency = status.balance_currency || 'IDR';
  const amount = Number(status.balance_amount || 0);
  if (!status.token_present && amount === 0) return '余额 -';
  return `余额 ${amount} ${currency}${status.has_min_balance ? ' 足额' : ' 未达标'}`;
}

function latestGoPayOtpWindow(status: GoPayUserStatusResponse['status']) {
  if (!status) return '';
  const windows = [
    { label: '登录 OTP', sent: status.login_otp_sent_at_unix, expires: status.login_otp_expires_at_unix },
    { label: '注册 OTP', sent: status.signup_otp_sent_at_unix, expires: status.signup_otp_expires_at_unix },
    { label: 'PIN OTP', sent: status.signup_pin_otp_sent_at_unix, expires: status.signup_pin_otp_expires_at_unix }
  ].filter((item) => item.sent || item.expires);
  if (windows.length === 0) return '';
  windows.sort((a, b) => (b.sent || b.expires) - (a.sent || a.expires));
  const latest = windows[0];
  return `${latest.label} ${formatUnix(latest.sent)} - ${formatUnix(latest.expires)}`;
}
