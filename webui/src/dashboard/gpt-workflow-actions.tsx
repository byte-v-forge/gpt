import { useEffect, useState } from 'react';
import { ExternalLink, RotateCcw, Send } from 'lucide-react';
import QRCode from 'qrcode';
import { Button, Input, api, buttonHint } from '@byte-v-forge/common-ui';
import { registerWorkflowJobActionRenderers, type WorkflowJobActionRendererProps } from './job-action-renderers';
import { GoPayAddBalanceActions, hasGoPayAddBalanceActions } from './gopay-add-balance-actions';
import { addBalanceMethodLabel, canConfirmManualGoPayPayment, goPayAddBalancePayload, manualGoPayPaymentView } from './gopay-utils';
import type { ConcreteGoPayAddBalanceMethod } from './types';

const OTP_WAIT_STEPS = new Set([
  'register_account_otp_wait', 'register_account_protocol_otp_wait',
  'login_session_otp_wait', 'login_session_protocol_otp_wait',
  'gopay_payment', 'gopay_app_change_phone_sms_wait', 'gopay_app_deactivate_sms_wait'
]);
let registered = false;

export function registerGptWorkflowActionRenderers() {
  if (registered) return;
  registered = true;
  registerWorkflowJobActionRenderers([{
    id: 'gpt.workflow.actions',
    statuses: ['RUNNING'],
    render: (props) => <GptWorkflowActions {...props} />
  }]);
}

function GptWorkflowActions(props: WorkflowJobActionRendererProps) {
  const { job, progress } = props;
  const [otp, setOtp] = useState('');
  const [pending, setPending] = useState('');
  const payment = manualGoPayPaymentView(job);
  const canConfirmPayment = canConfirmManualGoPayPayment(job, progress, payment);
  const canHandleAddBalance = hasGoPayAddBalanceActions(job, progress);
  const waitingOTP = OTP_WAIT_STEPS.has(progress?.step_name || job.last_step || '') && !canConfirmPayment;
  if (!waitingOTP && !canConfirmPayment && !canHandleAddBalance) return null;

  async function post(key: string, url: string, body: Record<string, any>, successText: string) {
    setPending(key);
    try {
      const resp = await api<{ success?: boolean; error_message?: string }>(url, { method: 'POST', body: JSON.stringify(body) });
      props.onMessage?.(resp.error_message ? 'error' : 'ok', resp.error_message || successText);
      if (!resp.error_message) await props.onChanged?.();
    } catch (err) {
      props.onError?.(err);
    } finally {
      setPending('');
    }
  }

  async function selectAddBalance(jobId: string, method: ConcreteGoPayAddBalanceMethod) {
    await post(method, `/api/gpt/jobs/${jobId}/add-balance/select`, { addBalance: goPayAddBalancePayload(method) }, `已选择加余额方式：${addBalanceMethodLabel(method)}`);
  }

  return (
    <div className="gptWorkflowActionCard">
      <div className="gptWorkflowActionHead">
        <strong>流程待处理</strong>
        <span>{job.action || job.job_id}</span>
      </div>
      {waitingOTP && (
        <form className="gptWorkflowOtpForm" onSubmit={(event) => {
          event.preventDefault();
          if (otp.trim()) void post('otp', `/api/gpt/jobs/${job.job_id}/otp`, { otp: otp.trim() }, 'OTP 已提交').then(() => setOtp(''));
        }}>
          <Input className="gptWorkflowOtpInput" inputMode="numeric" autoComplete="one-time-code" placeholder="OTP" value={otp} onChange={(event) => setOtp(event.target.value)} />
          <Button type="submit" disabled={!!pending || !otp.trim()}><Send size={14} />{pending === 'otp' ? '提交中' : '提交 OTP'}</Button>
          {canResendOTP(job) && <Button type="button" variant="outline" disabled={!!pending} onClick={() => void post('resend', `/api/gpt/jobs/${job.job_id}/otp/resend`, {}, 'OTP 重发已触发')}><RotateCcw size={14} />重发 OTP</Button>}
        </form>
      )}
      {canConfirmPayment && payment && (
        <ManualQRISPaymentActions payment={payment} busy={pending === 'payment'} onConfirm={() => void post('payment', `/api/gpt/jobs/${job.job_id}/gopay-payment/confirm`, {}, '已确认支付，继续后续步骤')} />
      )}
      {canHandleAddBalance && (
        <GoPayAddBalanceActions job={job} progress={progress} busy={!!pending} onSelect={selectAddBalance} onConfirm={(jobId) => post('add-balance-confirm', `/api/gpt/jobs/${jobId}/add-balance/confirm`, {}, '已确认加余额，继续后续步骤')} />
      )}
    </div>
  );
}

function canResendOTP(job: { action?: string; last_step?: string }) {
  return job.last_step === 'register_account_otp_wait';
}

function ManualQRISPaymentActions({ payment, busy, onConfirm }: {
  payment: NonNullable<ReturnType<typeof manualGoPayPaymentView>>;
  busy: boolean;
  onConfirm: () => void;
}) {
  const dataUrl = useQRCodeDataURL(payment.qr_payload);
  return (
    <div className="gptWorkflowQRIS">
      {dataUrl ? <img src={dataUrl} alt="QRIS" /> : <div className="gptWorkflowQRPlaceholder">QRIS</div>}
      <div className="gptWorkflowQRISInfo">
        <strong>扫码支付 QRIS</strong>
        {payment.charge_ref && <span>{payment.charge_ref}</span>}
        {payment.qr_url && <a href={payment.qr_url} target="_blank" rel="noreferrer">打开远端码<ExternalLink size={12} /></a>}
        {payment.deeplink_url && <a href={payment.deeplink_url} target="_blank" rel="noreferrer">GoPay deeplink<ExternalLink size={12} /></a>}
        <Button type="button" disabled={busy} onClick={onConfirm} {...buttonHint('确认已完成 QRIS 支付')}>{busy ? '确认中' : '我已支付，继续'}</Button>
      </div>
    </div>
  );
}

function useQRCodeDataURL(payload: string) {
  const [dataUrl, setDataUrl] = useState('');
  useEffect(() => {
    let alive = true;
    if (!payload) { setDataUrl(''); return; }
    QRCode.toDataURL(payload, { errorCorrectionLevel: 'M', margin: 1, width: 192 })
      .then((value) => { if (alive) setDataUrl(value); })
      .catch(() => { if (alive) setDataUrl(''); });
    return () => { alive = false; };
  }, [payload]);
  return dataUrl;
}
