import { useMemo } from 'react';
import { api, useQuery, useQueryClient } from '@/dashboard/module-kit';
import { useJobEventCache } from '@/dashboard/modules/workflow/sdk';
import type { GoPayDashboardStateResponse, Job, JobSnapshot } from './types';

const GO_PAY_JOB_ACTIONS = new Set(['GOPAY_APP', 'GOPAY_PAYMENT_REBIND']);

export const gopayQueryKeys = {
  state: ['gpt', 'gopay', 'state', 'local'] as const,
  runningJobs: ['gpt', 'gopay', 'running-jobs'] as const
};

export function useGoPayData() {
  const queryClient = useQueryClient();
  const stateQuery = useQuery({ queryKey: gopayQueryKeys.state, queryFn: fetchGoPayState });
  const jobsQuery = useQuery({ queryKey: gopayQueryKeys.runningJobs, queryFn: fetchRunningGoPayJobs });
  const runningJobs = useMemo(() => snapshotsToJobs(jobsQuery.data || []), [jobsQuery.data]);
  const currentJob = runningJobs[0];

  useJobEventCache({
    lists: [{ queryKey: gopayQueryKeys.runningJobs, include: isRunningGoPaySnapshot, limit: 10 }],
    onEvent: (event) => {
      if (isGoPaySnapshot(event.snapshot)) void queryClient.invalidateQueries({ queryKey: gopayQueryKeys.state });
    }
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
  return api<GoPayDashboardStateResponse>('/api/gopay/state?user_id=local');
}

function fetchRunningGoPayJobs() {
  return api<JobSnapshot[]>('/api/jobs?limit=20&status=RUNNING');
}

function snapshotsToJobs(snapshots: JobSnapshot[]) {
  return (Array.isArray(snapshots) ? snapshots : []).map((snapshot) => snapshot.job).filter((job): job is Job => !!job && GO_PAY_JOB_ACTIONS.has(job.action));
}

function isGoPaySnapshot(snapshot?: JobSnapshot) {
  return !!snapshot?.job?.action && GO_PAY_JOB_ACTIONS.has(snapshot.job.action);
}

function isRunningGoPaySnapshot(snapshot?: JobSnapshot) {
  return isGoPaySnapshot(snapshot) && snapshot?.job?.status === 'RUNNING';
}
