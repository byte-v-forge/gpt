import { ExternalLink } from 'lucide-react';
import { Badge, Button, EmptyBlock, accountActionLabel, formatUnix, statusText } from '@byte-v-forge/common-ui';
import type { GptActionCatalog } from './action-catalog';
import { renderWorkflowJobActions, type WorkflowJobActionMessageKind } from './job-action-renderers';
import type { Job, JobStep } from './types';

type AccountWorkflowTimelinePanelProps = {
  accountID: string;
  jobs: Job[];
  actionCatalog?: GptActionCatalog;
  onChanged?: () => void | Promise<void>;
  onMessage?: (kind: WorkflowJobActionMessageKind, message: string) => void;
  onError?: (error: unknown) => void;
};

export function AccountWorkflowTimelinePanel({ accountID, jobs, actionCatalog, onChanged, onMessage, onError }: AccountWorkflowTimelinePanelProps) {
  const current = selectCurrentJob(jobs
    .filter((job) => job.account_id === accountID)
    .sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0))
  );
  if (!current) return <EmptyBlock text="暂无工作流记录" />;
  const actions = renderWorkflowJobActions({
    job: current,
    progress: null,
    nowUnix: Math.floor(Date.now() / 1000),
    onChanged,
    onMessage,
    onError
  });
  return (
    <section className="accountWorkflowTimelinePanel">
      <WorkflowRunLine job={current} actionCatalog={actionCatalog} />
      {actions}
    </section>
  );
}

function WorkflowRunLine({ job, actionCatalog }: { job: Job; actionCatalog?: GptActionCatalog }) {
  const workflow = accountActionLabel(actionCatalog, job.action, compact(job.action));
  const step = currentStep(job);
  return (
    <article className="workflowRunLine">
      <StatusBadge status={job.status} />
      <span className="workflowRunLineItem">当前流程：<strong title={workflow}>{workflow}</strong></span>
      <span className="workflowRunLineItem">当前步骤：<strong title={step}>{step}</strong></span>
      <span className={`workflowRunMeta ${job.error_message ? 'workflowRunMetaError' : ''}`} title={job.error_message || job.job_id}>
        {job.error_message ? trimError(job.error_message) : `${formatUnix(job.updated_at)} · ${job.n8n_execution_id ? `n8n #${job.n8n_execution_id}` : job.job_id}`}
      </span>
      <Button asChild variant="outline" size="sm"><a href={workflowRuntimeHref(job)}><ExternalLink />工作流页</a></Button>
    </article>
  );
}

function selectCurrentJob(jobs: Job[]) {
  return jobs.find((job) => isRunning(job.status)) || jobs[0] || null;
}

function currentStep(job: Job) {
  if (job.last_step) return compact(job.last_step);
  const steps = sortSteps(job.steps || []);
  return compact(steps.at(-1)?.step_name || '-');
}

function sortSteps(steps: JobStep[]) {
  return [...steps].sort((a, b) => (a.started_at || 0) - (b.started_at || 0) || a.step_name.localeCompare(b.step_name));
}

function workflowRuntimeHref(job: Job) {
  const params = new URLSearchParams();
  if (job.n8n_execution_id) params.set('execution_id', job.n8n_execution_id);
  if (job.job_id) params.set('run_id', job.job_id);
  const query = params.toString();
  return `/workflow-runtime/workflow${query ? `?${query}` : ''}`;
}

function StatusBadge({ status }: { status?: string }) {
  const normalized = (status || 'UNKNOWN').toUpperCase();
  return <Badge className={`badge ${statusClass(normalized)}`} variant="outline">{statusText(normalized)}</Badge>;
}

function isRunning(status?: string) {
  return ['CREATED', 'RUNNING', 'WAITING'].includes((status || '').toUpperCase());
}

function statusClass(status?: string) {
  const normalized = (status || '').toUpperCase();
  if (normalized === 'SUCCEEDED') return 'good';
  if (normalized === 'RUNNING' || normalized === 'CREATED') return 'mid';
  if (normalized === 'CANCELED' || normalized.startsWith('FAILED')) return 'bad';
  return 'neutral';
}

function compact(value?: string) {
  return (value || '').replaceAll('_', ' ').toLowerCase().replace(/(^|\s)\S/g, (s) => s.toUpperCase());
}

function trimError(value?: string) {
  const text = (value || '').trim();
  return text.length > 260 ? `${text.slice(0, 260)}…` : text;
}
