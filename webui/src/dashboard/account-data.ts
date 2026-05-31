import { useMemo, type Dispatch, type SetStateAction } from 'react';
import { ACCOUNT_PAGE_SIZE, accountCarrierEmail, accountQueryKey, accountQueryPrefix, api, cursorPageURL, fetchAccountList, keySet, latestByKey, responseList, uniqueBy, uniqueStrings, useAccountManagementController, useQuery } from '@byte-v-forge/common-ui';
import { useJobEventCache } from './job-events';
import type { ListAccountsResponse, ListGPTEmailAllocationsResponse } from '../proto/gpt_account';
import type { ListJobsResponse } from '../proto/orchestrator_job';
import type { Account, GPTEmailAllocation, Job, JobSnapshot, Mailbox } from './types';

type ListMailboxesResponse = { mailboxes?: Mailbox[] };

export const accountQueryKeys = {
  accounts: accountQueryPrefix('gpt', 'accounts'),
  jobs: accountQueryPrefix('gpt', 'jobs'),
  runningJobs: accountQueryPrefix('gpt', 'running-jobs'),
  mailboxes: accountQueryPrefix('gpt', 'mailboxes'),
  allocations: accountQueryPrefix('gpt', 'email-allocations')
};

export function useGptAccountData(selectedAccountID: string, setSelectedAccountID?: Dispatch<SetStateAction<string>>) {
  const accountsQuery = useAccountManagementController<Account, ListAccountsResponse>({
    queryKey: accountQueryKeys.accounts,
    queryFn: (cursor) => fetchAccountList<Account, ListAccountsResponse>({ path: '/api/gpt/accounts', cursor, limit: ACCOUNT_PAGE_SIZE }),
    pageSize: ACCOUNT_PAGE_SIZE,
    selectedID: selectedAccountID,
    setSelectedID: setSelectedAccountID
  });
  const accounts = accountsQuery.accounts;
  const selected = accountsQuery.selected;
  const selectedEmail = accountCarrierEmail(selected) || '';
  const jobsQuery = useQuery({
    queryKey: accountQueryKeys.jobs,
    queryFn: () => api<ListJobsResponse>('/api/gpt/jobs?limit=200')
  });
  const runningJobsQuery = useQuery({
    queryKey: accountQueryKeys.runningJobs,
    queryFn: () => api<ListJobsResponse>('/api/gpt/jobs?limit=200&status=RUNNING')
  });
  const allocationsQuery = useQuery({
    queryKey: accountQueryKey(accountQueryKeys.allocations, selectedEmail),
    queryFn: () => api<ListGPTEmailAllocationsResponse>(emailAllocationURL(selectedEmail)),
    enabled: !!selectedEmail
  });
  const allocations = responseList<GPTEmailAllocation, ListGPTEmailAllocationsResponse, 'allocations'>(allocationsQuery.data, 'allocations');
  const allocationPrimaryEmail = allocations[0]?.primary_email || '';
  const mailboxEmails = useMemo(
    () => uniqueStrings([selectedEmail, selected?.primary_mailbox_email || '', allocationPrimaryEmail], { lowerCase: true }),
    [selectedEmail, selected?.primary_mailbox_email, allocationPrimaryEmail]
  );
  const mailboxesQuery = useQuery({
    queryKey: accountQueryKey(accountQueryKeys.mailboxes, mailboxEmails.join('|')),
    queryFn: () => loadMailboxesByEmail(mailboxEmails),
    enabled: mailboxEmails.length > 0
  });
  const jobs = snapshotsToJobs(jobSnapshots(jobsQuery.data));
  const runningJobs = snapshotsToJobs(jobSnapshots(runningJobsQuery.data));
  const runningIds = useMemo(() => keySet(runningJobs, (job) => job.account_id), [runningJobs]);
  const runningByAccount = useMemo(() => latestByKey(runningJobs, (job) => job.account_id, (job) => job.updated_at || 0), [runningJobs]);

  useJobEventCache({
    apiBase: '/api/gpt',
    lists: [
      { queryKey: accountQueryKeys.jobs },
      { queryKey: accountQueryKeys.runningJobs }
    ]
  });

  return {
    accounts,
    jobs,
    selected,
    setSelectedAccountID: accountsQuery.setSelectedID,
    runningIds,
    runningByAccount,
    mailboxes: responseList<Mailbox, ListMailboxesResponse, 'mailboxes'>(mailboxesQuery.data, 'mailboxes'),
    allocations,
    busy: accountsQuery.isLoading || jobsQuery.isLoading || runningJobsQuery.isLoading,
    accountsPagination: accountsQuery.accountsPagination,
    loadError: accountsQuery.error || jobsQuery.error || runningJobsQuery.error || mailboxesQuery.error || allocationsQuery.error,
    invalidate: accountsQuery.invalidate,
    cacheAccount: accountsQuery.cacheAccount,
    runAccountMutation: accountsQuery.runAccountMutation,
    isAccountActionActive: accountsQuery.isAccountActionActive,
    deleteAccount: accountsQuery.deleteAccount
  };
}

export type GptAccountData = ReturnType<typeof useGptAccountData>;

function snapshotsToJobs(snapshots: JobSnapshot[]) {
  return (Array.isArray(snapshots) ? snapshots : []).map((snapshot) => snapshot.job).filter((job): job is Job => !!job);
}

function jobSnapshots(response?: ListJobsResponse | null) {
  return Array.isArray(response?.snapshots) ? response.snapshots : [];
}

function emailAllocationURL(email: string) {
  return cursorPageURL('/api/gpt/email-allocations', { limit: 1, params: { email } });
}

async function loadMailboxesByEmail(emails: string[]): Promise<ListMailboxesResponse> {
  const pages = await Promise.all(emails.map((email) => api<ListMailboxesResponse>(mailboxURL(email))));
  return { mailboxes: uniqueBy(pages.flatMap((page) => responseList<Mailbox, ListMailboxesResponse, 'mailboxes'>(page, 'mailboxes')), (mailbox) => mailbox.email_address) };
}

function mailboxURL(email: string) {
  return cursorPageURL('/api/mailbox/mailboxes', { limit: 1, params: { email_address: email } });
}
