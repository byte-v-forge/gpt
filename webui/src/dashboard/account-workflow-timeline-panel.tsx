import { Badge, EmptyBlock, formatUnix, statusText } from '@byte-v-forge/common-ui';
import { gptActionLabel, type GptActionCatalog } from './action-catalog';
import { stepDuration, stepProgressText } from './job-utils';
import type { Job, JobStep } from './types';

export function AccountWorkflowTimelinePanel({ accountID, jobs, actionCatalog }: { accountID: string; jobs: Job[]; actionCatalog?: GptActionCatalog }) {
  const rows = jobs
    .filter((job) => job.account_id === accountID)
    .sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0))
    .slice(0, 20);
  if (!rows.length) return <EmptyBlock text="暂无工作流记录" />;
  return <section className="accountWorkflowTimelinePanel">{rows.map((job) => <WorkflowRunCard key={job.job_id} job={job} actionCatalog={actionCatalog} />)}</section>;
}

function WorkflowRunCard({ job, actionCatalog }: { job: Job; actionCatalog?: GptActionCatalog }) {
  const steps = [...(job.steps || [])].sort((a, b) => (a.started_at || 0) - (b.started_at || 0) || a.step_name.localeCompare(b.step_name));
  return (
    <article className="workflowRunCard">
      <header className="workflowRunHeader">
        <div className="workflowRunTitle">
          <strong>{gptActionLabel(actionCatalog, job.action, compact(job.action))}</strong>
          <span>{formatUnix(job.updated_at)} · {job.n8n_execution_id ? `n8n #${job.n8n_execution_id}` : job.job_id}</span>
        </div>
        <StatusBadge status={job.status} />
      </header>
      {job.error_message && <p className="workflowRunError">{trimError(job.error_message)}</p>}
      <ol className="workflowStepList">
        {steps.length ? steps.map((step) => <WorkflowStepItem key={step.step_name} step={step} current={step.step_name === job.last_step} />) : <li className="workflowStepEmpty">等待节点上报</li>}
      </ol>
    </article>
  );
}

function WorkflowStepItem({ step, current }: { step: JobStep; current: boolean }) {
  const progress = stepProgressText(step, null);
  return (
    <li className={`workflowStepItem ${statusClass(step.status)} ${current ? 'current' : ''}`}>
      <span className="workflowStepDot" />
      <div className="workflowStepBody">
        <div className="workflowStepTopline">
          <strong>{compact(step.step_name)}</strong>
          <span>{stepDuration(step)}<StatusBadge status={step.status} /></span>
        </div>
        <p>{stepTimeRange(step)}</p>
        {progress && <p className="workflowStepProgress">{progress}</p>}
        {step.error_message && <p className="workflowStepError">{trimError(step.error_message)}</p>}
      </div>
    </li>
  );
}

function StatusBadge({ status }: { status?: string }) {
  const normalized = (status || 'UNKNOWN').toUpperCase();
  return <Badge className={`badge ${statusClass(normalized)}`} variant="outline">{statusText(normalized)}</Badge>;
}

function stepTimeRange(step: JobStep) {
  const started = step.started_at ? formatUnix(step.started_at) : '-';
  const completed = step.completed_at ? formatUnix(step.completed_at) : step.status === 'RUNNING' ? '进行中' : '-';
  return `${started} → ${completed}`;
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
