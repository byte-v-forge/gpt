import type { ReactNode } from 'react';
import { stepDetailData, type Step } from './job-utils';
import type { Job, WorkflowProgress } from '../proto/orchestrator_job';

export type WorkflowStepRendererProps = {
  job: Job;
  step: Step;
  progress: WorkflowProgress | null;
  nowUnix: number;
  detail: Record<string, any> | null;
};

export type WorkflowStepRendererRegistration = {
  id: string;
  stepNames: string[];
  jobActions?: string[];
  label?: string | ((props: WorkflowStepRendererProps) => string);
  render?: (props: WorkflowStepRendererProps) => ReactNode;
};

const workflowStepRenderers: WorkflowStepRendererRegistration[] = [];

export function registerWorkflowStepRenderers(items: WorkflowStepRendererRegistration[]) {
  for (const item of items) {
    const index = workflowStepRenderers.findIndex((existing) => existing.id === item.id);
    if (index >= 0) workflowStepRenderers[index] = item;
    else workflowStepRenderers.push(item);
  }
}

export function workflowStepLabel(job: Job, step: Step, progress: WorkflowProgress | null, nowUnix: number) {
  const props = workflowStepProps(job, step, progress, nowUnix);
  const renderer = workflowStepRendererFor(props);
  if (!renderer?.label) return step.step_name || '-';
  return typeof renderer.label === 'function' ? renderer.label(props) : renderer.label;
}

export function renderWorkflowStepExtension(job: Job, step: Step, progress: WorkflowProgress | null, nowUnix: number) {
  const props = workflowStepProps(job, step, progress, nowUnix);
  return workflowStepRendererFor(props)?.render?.(props) || null;
}

function workflowStepProps(job: Job, step: Step, progress: WorkflowProgress | null, nowUnix: number): WorkflowStepRendererProps {
  return { job, step, progress, nowUnix, detail: stepDetailData(step) };
}

function workflowStepRendererFor(props: WorkflowStepRendererProps) {
  return workflowStepRenderers.find((item) => {
    if (!item.stepNames.includes(props.step.step_name || '')) return false;
    return !item.jobActions?.length || item.jobActions.includes(props.job.action || '');
  });
}
