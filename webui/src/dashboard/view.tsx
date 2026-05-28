import { Eye, EyeOff, Phone, RefreshCcw, Trash2 } from 'lucide-react';
import { PanelHeader, ToolbarIconButton } from '@byte-v-forge/common-ui';
import type {
  Account,
  ConcreteGoPayPaymentChannel,
  GPTEmailAllocation,
  GoPayDashboardStateResponse,
  Job,
  Mailbox,
  MailboxDomain,
  MailboxProviderCapability
} from './types';
import { AccountTable, CreateAccountForm } from './accounts';
import { accountCodexPhoneState, canLoginSession, isInvalidGptAccount } from './account-utils';
import { invalidAccountsForCleanup } from './account-cleanup-actions';
import { OpenAIIcon } from './brand-icons';
import { GoPayActionsPanel } from './gopay-actions';
import { GoPayStatusCard } from './gopay';

export type GptAccountsViewProps = {
  accounts: Account[];
  jobs: Job[];
  mailboxes: Mailbox[];
  allocations: GPTEmailAllocation[];
  mailboxDomains: MailboxDomain[];
  mailboxProviderCapabilities: MailboxProviderCapability[];
  selectedAccountId?: string;
  showSecrets: boolean;
  busy: boolean;
  cleaningInvalidAccounts: boolean;
  runningAccountIds: Set<string>;
  runningWorkflowByAccountID: Map<string, Job>;
  onCreateDone: (message: string) => Promise<void>;
  onError: (message: string) => void;
  onToggleSecrets: () => void;
  onCleanInvalidAccounts: () => void | Promise<void>;
  onSelectAccount: (account: Account) => void;
  onRegisterProtocol: (account: Account) => void | Promise<void>;
  onCodexOAuthBatchAddPhone: (accounts: Account[]) => void | Promise<void>;
  onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onDeleteAccount: (account: Account) => void | Promise<void>;
};

export function GptAccountsView(props: GptAccountsViewProps) {
  const addPhoneAccounts = props.accounts.filter((account) => canLoginSession(account) && !accountCodexPhoneState(account, props.jobs).confirmed);
  const invalidAccounts = invalidAccountsForCleanup(props.accounts);
  return (
    <>
      <PanelHeader title="GPT账号" icon={<OpenAIIcon size={16} />}>
        <div className="headerControls accountHeaderControls">
          <CreateAccountForm compact domains={props.mailboxDomains} providerCapabilities={props.mailboxProviderCapabilities} onDone={props.onCreateDone} onError={props.onError} />
          {addPhoneAccounts.length > 0 && <ToolbarIconButton label={`add phone · ${addPhoneAccounts.length} 个未加手机账号`} icon={<Phone size={15} />} disabled={props.busy} onClick={() => void props.onCodexOAuthBatchAddPhone(addPhoneAccounts)} />}
          {invalidAccounts.length > 0 && <ToolbarIconButton label={props.cleaningInvalidAccounts ? '清理中' : `清理失效账号 · ${invalidAccounts.length}`} icon={<Trash2 size={15} />} disabled={props.busy || props.cleaningInvalidAccounts} onClick={() => void props.onCleanInvalidAccounts()} />}
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
        busy={props.busy}
        onSelect={props.onSelectAccount}
        onRegisterProtocol={props.onRegisterProtocol}
        onGoPayPayment={props.onGoPayPayment}
        onDelete={props.onDeleteAccount}
      />
    </>
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
    <>
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
    </>
  );
}
