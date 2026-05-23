import { Eye, EyeOff, RefreshCcw } from 'lucide-react';
import {
  Button,
  IconActionButton,
  PanelHeader
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
  runningAccountIds: Set<string>;
  runningWorkflowByAccountID: Map<string, Job>;
  refreshingAccessTokenIds: Set<string>;
  onCreateDone: (message: string) => Promise<void>;
  onError: (message: string) => void;
  onToggleSecrets: () => void;
  onSyncMailboxes: () => void | Promise<void>;
  onSelectAccount: (account: Account) => void;
  onOpenWorkflow: (job: Job) => void | Promise<void>;
  onCancelWorkflow: (jobId: string) => Promise<void>;
  onRegister: (account: Account) => void | Promise<void>;
  onCodexOAuthAddPhone: (account: Account) => void | Promise<void>;
  onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onRefreshAccessToken: (account: Account) => Promise<void>;
  onSubmitOTP: (jobId: string, otp: string) => Promise<void>;
  onResendOTP: (jobId: string) => Promise<void>;
  onConfirmManualPayment: (jobId: string) => Promise<void>;
  onSelectAddBalance: (jobId: string, method: ConcreteGoPayAddBalanceMethod) => Promise<void>;
  onConfirmAddBalance: (jobId: string) => Promise<void>;
};

export function GptAccountsView(props: GptAccountsViewProps) {
  return (
    <section className="workspace singlePaneWorkspace">
      <div className="panel">
        <PanelHeader title="GPT账号" icon={<OpenAIIcon size={16} />}>
          <div className="headerControls accountHeaderControls">
            <CreateAccountForm compact domains={props.mailboxDomains} onDone={props.onCreateDone} onError={props.onError} />
            <Button className="secondaryButton" onClick={() => void props.onSyncMailboxes()} disabled={props.busy || props.mailboxSyncing}>
              <RefreshCcw size={15} /> {props.mailboxSyncing ? '同步中' : '同步邮箱'}
            </Button>
            <IconActionButton label={props.showSecrets ? '隐藏敏感信息' : '显示敏感信息'} icon={props.showSecrets ? <EyeOff size={15} /> : <Eye size={15} />} onClick={props.onToggleSecrets} />
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
          <Button className="secondaryButton" onClick={() => void props.onLoadState(true)} disabled={props.loading}>
            <RefreshCcw size={16} /> {props.loading ? '刷新中' : '刷新 state'}
          </Button>
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
