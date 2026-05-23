import type { Account, GPTEmailAllocation } from '@/proto/account_db';
import type { FetchAccountMailboxResponse, SyncAccountMailboxesResponse } from '@/proto/orchestrator_account';
import type { InboxMessage, InboxResponse, InboxResult, LatestOtp, Mailbox, MailboxDomain } from '@/dashboard/modules/mailbox/sdk';
import type { Job, JobSnapshot, WorkflowProgress } from '@/dashboard/modules/workflow/sdk';
import type { GoPayUserStatusResponse } from '@/proto/orchestrator_gopay_app';

export type { Account, FetchAccountMailboxResponse, GoPayUserStatusResponse, GPTEmailAllocation, InboxMessage, InboxResponse, InboxResult, Job, JobSnapshot, LatestOtp, Mailbox, MailboxDomain, SyncAccountMailboxesResponse, WorkflowProgress };

export type GoPayDashboardStateResponse = GoPayUserStatusResponse & {
  user_id: string;
  wa_phone: string;
  wa_phone_error_message?: string;
};

export type AccountMailboxContext = {
  account_email: string;
  primary_email: string;
  provider: string | number;
  is_split: boolean;
  known: boolean;
};

export type GoPayOTPChannel = '' | 'sms' | 'wa';
export type GoPayPaymentChannel = GoPayOTPChannel | 'app_wa';
export type ConcreteGoPayPaymentChannel = Exclude<GoPayPaymentChannel, ''>;
export type GoPayAddBalanceMethod = '' | 'manual_transfer' | 'envelope' | 'rekberinaja';
export type ConcreteGoPayAddBalanceMethod = Exclude<GoPayAddBalanceMethod, ''>;
export type DisplayLabelMap = Record<string, string>;
