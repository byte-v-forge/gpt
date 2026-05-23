import { useEffect, useState, type ReactNode } from 'react';
import { ExternalLink, RotateCcw, Send, XCircle } from 'lucide-react';
import QRCode from 'qrcode';
import { Button, Input, buttonHint } from '@/dashboard/module-kit';
import { WorkflowActionButton } from '@/dashboard/modules/workflow/sdk';
import { GoPayAddBalanceActions, hasGoPayAddBalanceActions } from './gopay-add-balance-actions';
import { canConfirmManualGoPayPayment, manualGoPayPaymentView } from './gopay-utils';
import type { ConcreteGoPayAddBalanceMethod, Job } from './types';

const otpWaitSteps = new Set([
  'register_account_otp_wait',
  'login_session_otp_wait',
  'gopay_payment',
  'gopay_app_ensure_token_available',
  'gopay_app_signup',
  'gopay_app_signup_retry',
  'gopay_app_ensure_pin_setup',
  'gopay_app_change_phone_sms_wait',
  'gopay_app_deactivate_sms_wait'
]);

export function AccountRunningWorkflowActions({ job, busy, onOpenWorkflow, onCancelWorkflow, onSubmitOTP, onResendOTP, onConfirmManualPayment, onSelectAddBalance, onConfirmAddBalance }: {
  job: Job;
  busy: boolean;
  onOpenWorkflow: (job: Job) => void;
  onCancelWorkflow: (jobId: string) => Promise<void>;
  onSubmitOTP: (jobId: string, otp: string) => Promise<void>;
  onResendOTP: (jobId: string) => Promise<void>;
  onConfirmManualPayment: (jobId: string) => Promise<void>;
  onSelectAddBalance: (jobId: string, method: ConcreteGoPayAddBalanceMethod) => Promise<void>;
  onConfirmAddBalance: (jobId: string) => Promise<void>;
}) {
  const [otp, setOtp] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [resending, setResending] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [canceling, setCanceling] = useState(false);
  const payment = manualGoPayPaymentView(job);
  const canConfirmPayment = canConfirmManualGoPayPayment(job, null, payment);
  const canHandleAddBalance = hasGoPayAddBalanceActions(job);
  const qrisActivation = job.action === 'GOPAY_QRIS_PAYMENT_ACTIVATE';
  const waitingOTP = otpWaitSteps.has(job.last_step || '') && !canConfirmPayment && !qrisActivation;
  const canResend = job.last_step === 'register_account_otp_wait' && ['REGISTER', 'REGISTER_AND_ACTIVATE'].includes(job.action || '');

  async function submit() {
    const code = otp.trim();
    if (!code) return;
    setSubmitting(true);
    try {
      await onSubmitOTP(job.job_id, code);
      setOtp('');
    } finally {
      setSubmitting(false);
    }
  }

  async function resend() {
    setResending(true);
    try {
      await onResendOTP(job.job_id);
    } finally {
      setResending(false);
    }
  }

  async function confirmPayment() {
    setConfirming(true);
    try {
      await onConfirmManualPayment(job.job_id);
    } finally {
      setConfirming(false);
    }
  }

  async function cancelWorkflow() {
    setCanceling(true);
    try {
      await onCancelWorkflow(job.job_id);
    } finally {
      setCanceling(false);
    }
  }

  const controls = (
    <>
      <WorkflowActionButton job={job} onOpen={onOpenWorkflow} />
      <Button className="iconActionButton" variant="destructive" type="button" {...buttonHint('取消流程')} disabled={busy || canceling} onClick={() => void cancelWorkflow()}><XCircle size={14} /></Button>
    </>
  );

  if (canHandleAddBalance) {
    return (
      <div className="runningOtpActions" onClick={(event) => event.stopPropagation()}>
        {controls}
        <GoPayAddBalanceActions job={job} busy={busy} onSelect={onSelectAddBalance} onConfirm={onConfirmAddBalance} />
      </div>
    );
  }
  if (qrisActivation) return <div className="runningOtpActions" onClick={(event) => event.stopPropagation()}>{controls}</div>;
  if (canConfirmPayment && payment) {
    return <ManualQRISPaymentActions payment={payment} busy={busy || confirming} controls={controls} onConfirm={() => void confirmPayment()} />;
  }
  if (!waitingOTP) return <div className="runningOtpActions" onClick={(event) => event.stopPropagation()}>{controls}</div>;

  return (
    <div className="runningOtpActions" onClick={(event) => event.stopPropagation()}>
      {controls}
      <form className="runningOtpForm" onSubmit={(event) => { event.preventDefault(); void submit(); }}>
        <Input className="runningOtpInput" inputMode="numeric" autoComplete="one-time-code" placeholder="OTP" value={otp} onChange={(event) => setOtp(event.target.value)} />
        <Button className="iconActionButton" type="submit" {...buttonHint('提交 OTP')} disabled={busy || submitting || !otp.trim()}><Send size={14} /></Button>
        {canResend && <Button className="iconActionButton" type="button" {...buttonHint('重发 OTP')} disabled={busy || resending} onClick={() => void resend()}><RotateCcw size={14} /></Button>}
      </form>
    </div>
  );
}

function ManualQRISPaymentActions({ payment, busy, controls, onConfirm }: {
  payment: NonNullable<ReturnType<typeof manualGoPayPaymentView>>;
  busy: boolean;
  controls: ReactNode;
  onConfirm: () => void;
}) {
  const dataUrl = useQRCodeDataURL(payment.qr_payload);
  return (
    <div className="flex max-w-[360px] items-center justify-end gap-3" onClick={(event) => event.stopPropagation()}>
      {controls}
      <div className="flex items-center gap-2 rounded-xl border bg-background p-2 shadow-sm">
        {dataUrl ? <img src={dataUrl} alt="QRIS" className="h-24 w-24 rounded bg-white p-1" /> : <div className="flex h-24 w-24 items-center justify-center rounded bg-muted text-xs text-muted-foreground">QRIS</div>}
        <div className="grid min-w-0 gap-1 text-left text-xs">
          <strong>扫码支付 QRIS</strong>
          {payment.charge_ref && <span className="max-w-[180px] truncate text-muted-foreground">{payment.charge_ref}</span>}
          {payment.qr_url && <a className="inline-flex items-center gap-1 text-primary" href={payment.qr_url} target="_blank" rel="noreferrer">打开远端码<ExternalLink size={12} /></a>}
          {payment.deeplink_url && <a className="inline-flex items-center gap-1 text-primary" href={payment.deeplink_url} target="_blank" rel="noreferrer">GoPay deeplink<ExternalLink size={12} /></a>}
          <Button className="h-7 px-2 text-xs" type="button" disabled={busy} onClick={onConfirm}>{busy ? '确认中' : '我已支付，继续'}</Button>
        </div>
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
