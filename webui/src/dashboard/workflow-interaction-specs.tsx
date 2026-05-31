import { RotateCcw, Send } from 'lucide-react';
import type { ReactNode } from 'react';
import { GPT_ACTIONS } from './action-catalog';
import { jobDataObject } from './job-data';
import type { Job, WorkflowProgress } from './types';

export type WorkflowJobInteractionAction = {
  key: string;
  label: string;
  pendingLabel: string;
  successText: string;
  icon: ReactNode;
  variant?: 'outline';
  url: string;
  body: (value: string) => Record<string, unknown>;
  enabled: (value: string) => boolean;
  clearOnSuccess?: boolean;
};

export type WorkflowJobInteraction = {
  id: string;
  title: string;
  subtitle: string;
  valueKey: string;
  input: {
    className: string;
    inputMode: 'numeric';
    autoComplete: string;
    placeholder: string;
  };
  submit: WorkflowJobInteractionAction;
  secondary?: WorkflowJobInteractionAction;
};

type WorkflowJobInteractionSpec = {
  id: string;
  matches: (job: Job, progress: WorkflowProgress | null) => boolean;
  create: (job: Job, progress: WorkflowProgress | null) => WorkflowJobInteraction | null;
};

type ChannelOTPContext = {
  step: string;
  channel: string;
  target: string;
};

const WORKFLOW_JOB_INTERACTIONS: WorkflowJobInteractionSpec[] = [
  {
    id: 'channel-otp',
    matches: (job, progress) => !!channelOTPContext(job, progress),
    create: (job, progress) => {
      const ctx = channelOTPContext(job, progress);
      return ctx ? channelOTPInteraction(job, ctx) : null;
    },
  },
];

export function workflowJobInteraction(job: Job, progress: WorkflowProgress | null) {
  const spec = WORKFLOW_JOB_INTERACTIONS.find((item) => item.matches(job, progress));
  return spec?.create(job, progress) || null;
}

function channelOTPInteraction(job: Job, ctx: ChannelOTPContext): WorkflowJobInteraction {
  return {
    id: 'channel-otp',
    title: '流程待处理',
    subtitle: `${job.action || job.job_id} · ${ctx.channel}:${ctx.target}`,
    valueKey: 'otp',
    input: {
      className: 'gptWorkflowOtpInput',
      inputMode: 'numeric',
      autoComplete: 'one-time-code',
      placeholder: 'OTP',
    },
    submit: {
      key: 'otp',
      label: '提交 OTP',
      pendingLabel: '提交中',
      successText: 'OTP 已提交',
      icon: <Send size={14} />,
      url: '/api/gpt/otp',
      body: (value) => ({ channel: ctx.channel, target: ctx.target, otp: value.trim() }),
      enabled: (value) => !!value.trim(),
      clearOnSuccess: true,
    },
    secondary: canResendChannelOTP(job, ctx.step) ? resendOTPAction(encodeURIComponent(job.job_id)) : undefined,
  };
}

function resendOTPAction(jobID: string): WorkflowJobInteractionAction {
  return {
    key: 'resend',
    label: '重发 OTP',
    pendingLabel: '重发中',
    successText: 'OTP 重发已触发',
    icon: <RotateCcw size={14} />,
    variant: 'outline',
    url: `/api/gpt/jobs/${jobID}/otp/resend`,
    body: () => ({}),
    enabled: () => true,
  };
}

function canResendChannelOTP(job: Job, step: string) {
  return job.action === GPT_ACTIONS.register && normalizeStep(step) === 'REGISTER_ACCOUNT_OTP_WAIT';
}

function channelOTPContext(job: Job, progress: WorkflowProgress | null): ChannelOTPContext | null {
  const step = workflowStep(job, progress);
  if (!isOTPWaitStep(step)) return null;
  const detail = workflowStepDetail(job, step);
  const channel = stringValue(detail?.channel || detail?.channel_otp_channel);
  const target = stringValue(detail?.target || detail?.channel_otp_target);
  return channel && target ? { step, channel, target } : null;
}

function workflowStepDetail(job: Job, step: string): Record<string, unknown> | undefined {
  const normalized = normalizeStep(step);
  const steps = [...(job.steps || [])].reverse();
  return jobDataObject(steps.find((item) => normalizeStep(item.step_name) === normalized)?.detail);
}

function workflowStep(job: Job, progress: WorkflowProgress | null) {
  return progress?.step_name || job.last_step || '';
}

function isOTPWaitStep(step: string) {
  const normalized = normalizeStep(step);
  return normalized === 'OTP_WAIT' || normalized.endsWith('_OTP_WAIT');
}

function normalizeStep(step: string) {
  return String(step || '').trim().toUpperCase().replace(/[\s-]+/g, '_');
}

function stringValue(value: unknown) {
  return String(value || '').trim();
}
