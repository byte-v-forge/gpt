import { useMemo } from 'react';
import { api, useQuery, useQueryClient } from '@byte-v-forge/common-ui';
import { GPT_CAPABILITIES, gptActionsWithCapability, type GptActionCatalog } from './action-catalog';
import { useJobEventCache } from './job-events';
import type { GoPayDashboardStateResponse, Job, JobSnapshot } from './types';

export const gopayQueryKeys = {
  state: ['gpt', 'gopay', 'state', 'local'] as const,
  runningJobs: ['gpt', 'gopay', 'running-jobs'] as const
};

export function useGoPayData(actionCatalog?: GptActionCatalog) {
  const queryClient = useQueryClient();
  const goPayJobActions = useMemo(() => new Set(gptActionsWithCapability(actionCatalog, GPT_CAPABILITIES.goPay)), [actionCatalog]);
  const stateQuery = useQuery({ queryKey: gopayQueryKeys.state, queryFn: fetchGoPayState });
  const jobsQuery = useQuery({ queryKey: gopayQueryKeys.runningJobs, queryFn: fetchRunningGoPayJobs });
  const runningJobs = useMemo(() => snapshotsToJobs(jobsQuery.data || [], goPayJobActions), [goPayJobActions, jobsQuery.data]);
  const currentJob = runningJobs[0];

  useJobEventCache({
    apiBase: '/api/gpt',
    lists: [{ queryKey: gopayQueryKeys.runningJobs, include: (snapshot) => isRunningGoPaySnapshot(snapshot, goPayJobActions), limit: 10 }],
    onEvent: () => { void queryClient.invalidateQueries({ queryKey: gopayQueryKeys.state }); }
  });

  return {
    state: stateQuery.data || null,
    currentJob,
    loading: stateQuery.isFetching,
    loadError: stateQuery.error || jobsQuery.error,
    refreshState: () => stateQuery.refetch(),
    refreshJobs: () => queryClient.invalidateQueries({ queryKey: gopayQueryKeys.runningJobs })
  };
}

function fetchGoPayState() {
  return api<GoPayDashboardStateResponse>('/api/gpt/gopay/state?user_id=local');
}

function fetchRunningGoPayJobs() {
  return api<JobSnapshot[]>('/api/gpt/jobs?limit=20&status=RUNNING');
}

function snapshotsToJobs(snapshots: JobSnapshot[], actions: Set<string>) {
  return (Array.isArray(snapshots) ? snapshots : []).map((snapshot) => snapshot.job).filter((job): job is Job => !!job && actions.has(job.action));
}

function isGoPaySnapshot(snapshot: JobSnapshot | undefined, actions: Set<string>) {
  return !!snapshot?.job?.action && actions.has(snapshot.job.action);
}

function isRunningGoPaySnapshot(snapshot: JobSnapshot | undefined, actions: Set<string>) {
  return isGoPaySnapshot(snapshot, actions) && snapshot?.job?.status === 'RUNNING';
}
