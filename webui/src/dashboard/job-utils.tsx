import { formatUnix, numberValue, objectValue, statusText, stringValue } from '@byte-v-forge/common-ui';
import type { Job, JobEvent, JobSnapshot, JobStep, WorkflowProgress } from '../proto/orchestrator_job';

export type Step = JobStep;

export function isRunningSnapshot(snapshot: JobSnapshot) {
  return snapshot.job?.status === 'RUNNING';
}

export function jobSnapshotMatchesStatus(snapshot: JobSnapshot, status: string) {
  return !status || snapshot.job?.status === status;
}

export function mergeJobSnapshots(prev: JobSnapshot[], snapshot: JobSnapshot, include: boolean) {
  const jobID = snapshot.job?.job_id;
  if (!jobID) return prev;
  const index = prev.findIndex((item) => item.job?.job_id === jobID);
  if (!include) return index === -1 ? prev : prev.filter((item) => item.job?.job_id !== jobID);
  if (index === -1) return [snapshot, ...prev];
  const next = [...prev];
  next[index] = snapshot;
  return next;
}

export function mergeJobEvents(prev: JobEvent[], event: JobEvent, jobID: string) {
  if (!event?.event_id || event.job_id !== jobID) return prev;
  return [event, ...prev.filter((item) => item.event_id !== event.event_id)].sort((a, b) => b.event_id - a.event_id).slice(0, 80);
}

export function stepDetailData(step?: Step): Record<string, any> | null {
  if (!step?.detail || typeof step.detail !== 'object') return null;
  return step.detail as Record<string, any>;
}

export function stepDuration(step: Step, nowUnix?: number) {
  if (!step.started_at) return null;
  const end = step.completed_at || nowUnix || Math.floor(Date.now() / 1000);
  const seconds = Math.max(0, end - step.started_at);
  if (seconds < 1) return <small className="stepTime">刚刚</small>;
  if (seconds < 60) return <small className="stepTime">{seconds}s</small>;
  return <small className="stepTime">{Math.floor(seconds / 60)}m {seconds % 60}s</small>;
}

export function eventTime(event: JobEvent) {
  const updated = event.snapshot?.progress?.updated_at_unix || event.snapshot?.job?.updated_at || 0;
  return formatUnix(updated);
}

export function formatJobTime(value: string | number) {
  if (!value) return '-';
  if (typeof value === 'number') return formatUnix(value);
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function stepProgressText(step: Step, workflowProgress?: WorkflowProgress | null) {
  const data = stepDetailData(step);
  if (data) {
    const progress = objectValue(data.progress);
    const message = stringValue(data.progress_message) || stringValue(progress.message);
    if (message) {
      const atUnix = numberValue(data.progress_at_unix) || numberValue(progress.at_unix);
      return atUnix ? `${message} · ${formatUnix(atUnix)}` : message;
    }
  }
  if (!workflowProgress || workflowProgress.step_name !== step.step_name) return '';
  const message = workflowProgress.error_message || statusText(workflowProgress.status.toUpperCase());
  return message && workflowProgress.updated_at_unix ? `${message} · ${formatUnix(workflowProgress.updated_at_unix)}` : message;
}

export function latestJobMap(jobs: Job[], keyOf: (job: Job) => string) {
  const map = new Map<string, Job>();
  for (const job of jobs) {
    const key = keyOf(job);
    const previous = key ? map.get(key) : undefined;
    if (key && (!previous || (job.updated_at || 0) > (previous.updated_at || 0))) map.set(key, job);
  }
  return map;
}
