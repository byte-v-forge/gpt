import { useState } from 'react';
import { RotateCcw, Send } from 'lucide-react';
import { Button, Input, buttonHint } from '@/dashboard/module-kit';
import { WorkflowActionButton } from '@/dashboard/modules/workflow/sdk';
import type { Job } from './types';

const otpWaitSteps = new Set([
  'register_account_otp_wait',
  'login_session_otp_wait',
  'gopay_payment',
  'gopay_app_ensure_token_available',
  'gopay_app_signup',
  'gopay_app_signup_retry',
  'gopay_app_ensure_pin_settled',
  'gopay_app_change_phone_sms_wait',
  'gopay_app_deactivate_sms_wait'
]);

export function AccountRunningWorkflowActions({ job, busy, onOpenWorkflow, onSubmitOTP, onResendOTP }: {
  job: Job;
  busy: boolean;
  onOpenWorkflow: (job: Job) => void;
  onSubmitOTP: (jobId: string, otp: string) => Promise<void>;
  onResendOTP: (jobId: string) => Promise<void>;
}) {
  const [otp, setOtp] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [resending, setResending] = useState(false);
  const waitingOTP = otpWaitSteps.has(job.last_step || '');
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

  if (!waitingOTP) {
    return <WorkflowActionButton job={job} onOpen={onOpenWorkflow} />;
  }

  return (
    <div className="runningOtpActions" onClick={(event) => event.stopPropagation()}>
      <WorkflowActionButton job={job} onOpen={onOpenWorkflow} />
      <form className="runningOtpForm" onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}>
        <Input
          className="runningOtpInput"
          inputMode="numeric"
          autoComplete="one-time-code"
          placeholder="OTP"
          value={otp}
          onChange={(event) => setOtp(event.target.value)}
        />
        <Button className="iconActionButton" type="submit" {...buttonHint('提交 OTP')} disabled={busy || submitting || !otp.trim()}>
          <Send size={14} />
        </Button>
        {canResend && (
          <Button className="iconActionButton" type="button" {...buttonHint('重发 OTP')} disabled={busy || resending} onClick={() => void resend()}>
            <RotateCcw size={14} />
          </Button>
        )}
      </form>
    </div>
  );
}
