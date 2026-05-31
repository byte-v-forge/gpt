import type { Account, GPTEmailAllocation } from '../proto/gpt_account';
import type {
  AccountFingerprintResponse as AccountBrowserFingerprint,
  AccountProxyUsage,
  FetchAccountMailboxResponse as ProtoFetchAccountMailboxResponse,
} from '../proto/orchestrator_account';
import type { InboxMessage, InboxResponse, InboxResult, LatestOtp, Mailbox, MailboxDomain, MailboxProviderCapability } from '@byte-v-forge/common-ui';
import type { Job, JobData, JobSnapshot, JobStep, WorkflowProgress } from '../proto/orchestrator_job';

export type FetchAccountMailboxResponse = Omit<ProtoFetchAccountMailboxResponse, 'inbox'> & { inbox?: InboxResponse };

export type {
  Account,
  AccountBrowserFingerprint,
  AccountProxyUsage,
  GPTEmailAllocation,
  InboxMessage,
  InboxResponse,
  InboxResult,
  Job,
  JobData,
  JobSnapshot,
  JobStep,
  LatestOtp,
  Mailbox,
  MailboxDomain,
  MailboxProviderCapability,
  WorkflowProgress,
};


export type DisplayLabelMap = Record<string, string>;
