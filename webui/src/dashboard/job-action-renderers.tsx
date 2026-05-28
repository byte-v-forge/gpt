import { Fragment, type ReactNode } from 'react';
import type { Job, WorkflowProgress } from '../proto/orchestrator_job';

export type WorkflowJobActionMessageKind = 'ok' | 'error';

export type WorkflowJobActionRendererProps = {
  job: Job;
  progress: WorkflowProgress | null;
  nowUnix: number;
  onChanged?: () => void | Promise<void>;
  onMessage?: (kind: WorkflowJobActionMessageKind, message: string) => void;
  onError?: (error: unknown) => void;
};

export type WorkflowJobActionRendererRegistration = {
  id: string;
  jobActions?: string[];
  statuses?: string[];
  render: (props: WorkflowJobActionRendererProps) => ReactNode;
};

const workflowJobActionRenderers: WorkflowJobActionRendererRegistration[] = [];

export function registerWorkflowJobActionRenderers(items: WorkflowJobActionRendererRegistration[]) {
  for (const item of items) {
    const index = workflowJobActionRenderers.findIndex((existing) => existing.id === item.id);
    if (index >= 0) workflowJobActionRenderers[index] = item;
    else workflowJobActionRenderers.push(item);
  }
}

export function renderWorkflowJobActions(props: WorkflowJobActionRendererProps) {
  const nodes = workflowJobActionRenderers
    .filter((renderer) => matches(renderer, props.job))
    .map((renderer) => ({ id: renderer.id, node: renderer.render(props) }))
    .filter((item) => !!item.node);
  return nodes.length ? <>{nodes.map((item) => <Fragment key={item.id}>{item.node}</Fragment>)}</> : null;
}

function matches(renderer: WorkflowJobActionRendererRegistration, job: Job) {
  if (renderer.jobActions?.length && !renderer.jobActions.includes(job.action || '')) return false;
  return !renderer.statuses?.length || renderer.statuses.includes(job.status || '');
}
