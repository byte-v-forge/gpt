import { Eye, EyeOff, Phone, RefreshCcw, Trash2 } from 'lucide-react';
import {
  PanelHeader,
  ToolbarIconButton
} from '@/dashboard/module-kit';
import type {
  Account,
  ConcreteGoPayAddBalanceMethod,
  ConcreteGoPayPaymentChannel,
  GoPayDashboardStateResponse,
  Job,
  MailboxDomain
} from './types';
import { AccountTable, CreateAccountForm } from './accounts';
import { accountCodexPhoneState, canLoginSession } from './account-utils';
import { invalidAccountsForCleanup } from './account-cleanup-actions';
import { OpenAIIcon } from './brand-icons';
import { GoPayActionsPanel } from './gopay-actions';
import { GoPayStatusCard } from './gopay';

export type GptAccountsViewProps = {
  accounts: Account[];
  jobs: Job[];
  mailboxDomains: MailboxDomain[];
  selectedAccountId?: string;
  showSecrets: boolean;
  busy: boolean;
  mailboxSyncing: boolean;
  cleaningInvalidAccounts: boolean;
  runningAccountIds: Set<string>;
  runningWorkflowByAccountID: Map<string, Job>;
  refreshingAccessTokenIds: Set<string>;
  onCreateDone: (message: string) => Promise<void>;
  onError: (message: string) => void;
  onToggleSecrets: () => void;
  onSyncMailboxes: () => void | Promise<void>;
  onCleanInvalidAccounts: () => void | Promise<void>;
  onSelectAccount: (account: Account) => void;
  onOpenWorkflow: (job: Job) => void | Promise<void>;
  onCancelWorkflow: (jobId: string) => Promise<void>;
  onRegister: (account: Account) => void | Promise<void>;
  onCodexOAuthAddPhone: (account: Account) => void | Promise<void>;
  onCodexOAuthBatchAddPhone: (accounts: Account[]) => void | Promise<void>;
  onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onRefreshAccessToken: (account: Account) => Promise<void>;
  onSubmitOTP: (jobId: string, otp: string) => Promise<void>;
  onResendOTP: (jobId: string) => Promise<void>;
  onConfirmManualPayment: (jobId: string) => Promise<void>;
  onSelectAddBalance: (jobId: string, method: ConcreteGoPayAddBalanceMethod) => Promise<void>;
  onConfirmAddBalance: (jobId: string) => Promise<void>;
};

export function GptAccountsView(props: GptAccountsViewProps) {
  const addPhoneAccounts = props.accounts.filter((account) => canLoginSession(account) && !accountCodexPhoneState(account, props.jobs).confirmed);
  const invalidAccounts = invalidAccountsForCleanup(props.accounts);
  return (
    <section className="workspace singlePaneWorkspace">
      <div className="panel">
        <PanelHeader title="GPT账号" icon={<OpenAIIcon size={16} />}>
          <div className="headerControls accountHeaderControls">
            <CreateAccountForm compact domains={props.mailboxDomains} onDone={props.onCreateDone} onError={props.onError} />
            <ToolbarIconButton label={`add phone · ${addPhoneAccounts.length} 个未加手机账号`} icon={<Phone size={15} />} disabled={props.busy || !addPhoneAccounts.length} onClick={() => void props.onCodexOAuthBatchAddPhone(addPhoneAccounts)} />
            <ToolbarIconButton label={props.cleaningInvalidAccounts ? '清理中' : `清理失效账号 · ${invalidAccounts.length}`} icon={<Trash2 size={15} />} disabled={props.busy || props.cleaningInvalidAccounts} onClick={() => void props.onCleanInvalidAccounts()} />
            <ToolbarIconButton label={props.mailboxSyncing ? '同步邮箱中' : '同步邮箱'} icon={<RefreshCcw size={15} />} disabled={props.busy || props.mailboxSyncing} onClick={() => void props.onSyncMailboxes()} />
            <ToolbarIconButton label={props.showSecrets ? '隐藏敏感信息' : '显示敏感信息'} icon={props.showSecrets ? <EyeOff size={15} /> : <Eye size={15} />} onClick={props.onToggleSecrets} />
          </div>
        </PanelHeader>
        <AccountTable
          accounts={props.accounts}
          jobs={props.jobs}
          selected={props.selectedAccountId}
          showSecrets={props.showSecrets}
          runningAccountIds={props.runningAccountIds}
          runningWorkflowByAccountID={props.runningWorkflowByAccountID}
          refreshingAccessTokenIds={props.refreshingAccessTokenIds}
          busy={props.busy}
          onSelect={props.onSelectAccount}
          onOpenWorkflow={props.onOpenWorkflow}
          onCancelWorkflow={props.onCancelWorkflow}
          onRegister={props.onRegister}
          onCodexOAuthAddPhone={props.onCodexOAuthAddPhone}
          onGoPayPayment={props.onGoPayPayment}
          onRefreshAccessToken={props.onRefreshAccessToken}
          onSubmitOTP={props.onSubmitOTP}
          onResendOTP={props.onResendOTP}
          onConfirmManualPayment={props.onConfirmManualPayment}
          onSelectAddBalance={props.onSelectAddBalance}
          onConfirmAddBalance={props.onConfirmAddBalance}
        />
      </div>
    </section>
  );
}

export type GoPayLabViewProps = {
  state: GoPayDashboardStateResponse | null;
  loading: boolean;
  currentJob?: Job;
  onLoadState: (showToast?: boolean) => void | Promise<void>;
  onRefreshJobs: () => void | Promise<void>;
  onGoPayActionDone: (message: string, error?: boolean) => void;
  onCancelWorkflow: (jobId: string) => Promise<void>;
};

export function GoPayLabView(props: GoPayLabViewProps) {
  return (
    <section className="workspace singlePaneWorkspace">
      <div className="panel">
        <PanelHeader title="GoPay" icon={<RefreshCcw size={16} />}>
          <ToolbarIconButton label={props.loading ? '刷新 state 中' : '刷新 state'} icon={<RefreshCcw size={16} />} disabled={props.loading} onClick={() => void props.onLoadState(true)} />
        </PanelHeader>
        <GoPayStatusCard state={props.state} currentJob={props.currentJob} loading={props.loading} />
        <GoPayActionsPanel
          currentJob={props.currentJob}
          onDone={props.onGoPayActionDone}
          onCancelWorkflow={props.onCancelWorkflow}
          onRefreshState={() => props.onLoadState(false)}
          onRefreshJobs={props.onRefreshJobs}
        />
      </div>
    </section>
  );
}
