import { useMemo } from 'react';
import { api, useQuery, useQueryClient } from '@byte-v-forge/common-ui';
import { useJobEventCache } from './job-events';
import { latestJobMap } from './job-utils';
import type { Account, GPTEmailAllocation, Job, JobSnapshot, Mailbox } from './types';

type ListMailboxesResponse = { mailboxes?: Mailbox[] };

export const accountQueryKeys = {
  accounts: ['gpt', 'accounts'] as const,
  jobs: ['gpt', 'jobs'] as const,
  runningJobs: ['gpt', 'running-jobs'] as const,
  mailboxes: ['gpt', 'mailboxes'] as const,
  allocations: ['gpt', 'email-allocations'] as const
};

export function useGptAccountData(selectedAccountID: string) {
  const queryClient = useQueryClient();
  const selectedMailboxEnabled = !!selectedAccountID;
  const accountsQuery = useQuery({
    queryKey: accountQueryKeys.accounts,
    queryFn: () => api<Account[]>('/api/gpt/accounts?limit=200')
  });
  const jobsQuery = useQuery({
    queryKey: accountQueryKeys.jobs,
    queryFn: () => api<JobSnapshot[]>('/api/gpt/jobs?limit=200')
  });
  const runningJobsQuery = useQuery({
    queryKey: accountQueryKeys.runningJobs,
    queryFn: () => api<JobSnapshot[]>('/api/gpt/jobs?limit=200&status=RUNNING')
  });
  const mailboxesQuery = useQuery({
    queryKey: accountQueryKeys.mailboxes,
    queryFn: () => api<ListMailboxesResponse>('/api/mailbox/mailboxes?limit=500'),
    enabled: selectedMailboxEnabled
  });
  const allocationsQuery = useQuery({
    queryKey: accountQueryKeys.allocations,
    queryFn: () => api<GPTEmailAllocation[]>('/api/gpt/email-allocations?limit=500'),
    enabled: selectedMailboxEnabled
  });
  const accounts = Array.isArray(accountsQuery.data) ? accountsQuery.data : [];
  const jobs = snapshotsToJobs(jobsQuery.data || []);
  const runningJobs = snapshotsToJobs(runningJobsQuery.data || []);
  const selected = accounts.find((account) => account.account_id === selectedAccountID) || null;
  const runningIds = useMemo(() => new Set(runningJobs.map((job) => job.account_id).filter(Boolean)), [runningJobs]);
  const runningByAccount = useMemo(
    () =>
      latestJobMap(
        runningJobs.filter((job) => job.account_id),
        (job) => job.account_id
      ),
    [runningJobs]
  );

  useJobEventCache({
    apiBase: '/api/gpt',
    lists: [
      { queryKey: accountQueryKeys.jobs, limit: 200 },
      {
        queryKey: accountQueryKeys.runningJobs,
        include: isRunningSnapshot,
        limit: 200
      }
    ]
  });

  return {
    accounts,
    jobs,
    selected,
    runningIds,
    runningByAccount,
    mailboxes: Array.isArray(mailboxesQuery.data?.mailboxes) ? mailboxesQuery.data.mailboxes : [],
    allocations: Array.isArray(allocationsQuery.data) ? allocationsQuery.data : [],
    busy: accountsQuery.isLoading || jobsQuery.isLoading || runningJobsQuery.isLoading,
    loadError: accountsQuery.error || jobsQuery.error || runningJobsQuery.error || mailboxesQuery.error || allocationsQuery.error,
    invalidate: () => invalidateAccountQueries(queryClient),
    cacheAccount: (account: Account) => queryClient.setQueryData<Account[]>(accountQueryKeys.accounts, (prev) => updateAccountCache(prev, account))
  };
}

export type GptAccountData = ReturnType<typeof useGptAccountData>;

function invalidateAccountQueries(queryClient: ReturnType<typeof useQueryClient>) {
  return Promise.all(Object.values(accountQueryKeys).map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}

export function updateAccountCache(prev: Account[] | undefined, updated: Account) {
  if (!Array.isArray(prev)) return [updated];
  if (!prev.some((item) => item.account_id === updated.account_id)) return [updated, ...prev];
  return prev.map((item) => (item.account_id === updated.account_id ? updated : item));
}

function isRunningSnapshot(snapshot: JobSnapshot) {
  return snapshot.job?.status === 'RUNNING';
}

function snapshotsToJobs(snapshots: JobSnapshot[]) {
  return (Array.isArray(snapshots) ? snapshots : []).map((snapshot) => snapshot.job).filter((job): job is Job => !!job);
}
