import { FileKey, KeyRound, Play, Zap } from 'lucide-react';
import {
  RecordActionButtons,
  RecordActions,
  RecordCard,
  RecordIdentity,
  RecordList,
  RecordMain,
  RecordTop
} from '@/dashboard/module-kit';
import type { RowActionDescriptor } from '@/dashboard/module-kit';
import { maskEmail } from '@/dashboard/modules/mailbox/sdk';
import {
  accountActivationChannel,
  accountCodexPhoneState,
  canGoPayPayment,
  canLoginSession,
  canRefreshAccessToken,
  canRegister,
  isUserAlreadyExistsAccount
} from './account-utils';
import { AccountChannelTag, AccountCodexPhoneTag, AccountSignalBadge, PaymentChannelIcon } from './account-badges';
import { AccountRunningWorkflowActions } from './account-otp-actions';
import { OpenAIIcon } from './brand-icons';
import { GO_PAY_PAYMENT_CHANNELS, goPayPaymentActionLabel } from './gopay-utils';
import type { Account, ConcreteGoPayAddBalanceMethod, ConcreteGoPayPaymentChannel, Job } from './types';

export function AccountTable({ accounts, jobs, selected, showSecrets, runningAccountIds, runningWorkflowByAccountID, refreshingAccessTokenIds, busy, onSelect, onOpenWorkflow, onCancelWorkflow, onRegister, onCodexOAuthAddPhone, onCodexOAuthProtocol, onGoPayPayment, onRefreshAccessToken, onSubmitOTP, onResendOTP, onConfirmManualPayment, onSelectAddBalance, onConfirmAddBalance }: {
  accounts: Account[];
  jobs: Job[];
  selected?: string;
  showSecrets: boolean;
  runningAccountIds: Set<string>;
  runningWorkflowByAccountID: Map<string, Job>;
  refreshingAccessTokenIds: Set<string>;
  busy: boolean;
  onSelect: (a: Account) => void;
  onOpenWorkflow: (job: Job) => void;
  onCancelWorkflow: (jobId: string) => Promise<void>;
  onRegister: (a: Account) => void;
  onCodexOAuthAddPhone: (a: Account) => void;
  onCodexOAuthProtocol: (a: Account) => void;
  onGoPayPayment: (a: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onRefreshAccessToken: (a: Account) => Promise<void>;
  onSubmitOTP: (jobId: string, otp: string) => Promise<void>;
  onResendOTP: (jobId: string) => Promise<void>;
  onConfirmManualPayment: (jobId: string) => Promise<void>;
  onSelectAddBalance: (jobId: string, method: ConcreteGoPayAddBalanceMethod) => Promise<void>;
  onConfirmAddBalance: (jobId: string) => Promise<void>;
}) {
  return (
    <RecordList className="accountsList" emptyText="暂无账号。可以先创建账号，或切换为全部状态查看。">
      {accounts.map((account) => {
        const accountBusy = runningAccountIds.has(account.account_id);
        const currentWorkflow = runningWorkflowByAccountID.get(account.account_id);
        const refreshingAccessToken = refreshingAccessTokenIds.has(account.account_id);
        const activationChannel = accountActivationChannel(account, jobs);
        const phoneState = accountCodexPhoneState(account, jobs);
        return (
          <RecordCard key={account.account_id} selected={selected === account.account_id} onClick={() => onSelect(account)}>
            <RecordMain>
              <RecordTop>
                <AccountCardIdentity account={account} showSecrets={showSecrets} />
                <div className="accountCardTags">
                  <AccountSignalBadge account={account} compact />
                  <AccountCodexPhoneTag state={phoneState} />
                  <AccountChannelTag channel={activationChannel} />
                </div>
              </RecordTop>
            </RecordMain>
            <AccountRowActions
              account={account}
              accountBusy={accountBusy}
              currentWorkflow={currentWorkflow}
              busy={busy}
              refreshingAccessToken={refreshingAccessToken}
              onOpenWorkflow={onOpenWorkflow}
              onCancelWorkflow={onCancelWorkflow}
              onRegister={onRegister}
              onCodexOAuthAddPhone={onCodexOAuthAddPhone}
              onCodexOAuthProtocol={onCodexOAuthProtocol}
              onGoPayPayment={onGoPayPayment}
              onRefreshAccessToken={onRefreshAccessToken}
              onSubmitOTP={onSubmitOTP}
              onResendOTP={onResendOTP}
              onConfirmManualPayment={onConfirmManualPayment}
              onSelectAddBalance={onSelectAddBalance}
              onConfirmAddBalance={onConfirmAddBalance}
            />
          </RecordCard>
        );
      })}
    </RecordList>
  );
}

function AccountCardIdentity({ account, showSecrets }: {
  account: Account;
  showSecrets: boolean;
}) {
  const email = account.email || '-';
  const displayEmail = showSecrets ? email : maskEmail(email);
  return (
    <RecordIdentity
      icon={<OpenAIIcon size={15} />}
      title={<span className="accountCardEmail" title={displayEmail}>{displayEmail}</span>}
    />
  );
}

function AccountRowActions({ account, accountBusy, currentWorkflow, busy, refreshingAccessToken, onOpenWorkflow, onCancelWorkflow, onRegister, onCodexOAuthAddPhone, onCodexOAuthProtocol, onGoPayPayment, onRefreshAccessToken, onSubmitOTP, onResendOTP, onConfirmManualPayment, onSelectAddBalance, onConfirmAddBalance }: {
  account: Account;
  accountBusy: boolean;
  currentWorkflow?: Job;
  busy: boolean;
  refreshingAccessToken: boolean;
  onOpenWorkflow: (job: Job) => void;
  onCancelWorkflow: (jobId: string) => Promise<void>;
  onRegister: (a: Account) => void;
  onCodexOAuthAddPhone: (a: Account) => void;
  onCodexOAuthProtocol: (a: Account) => void;
  onGoPayPayment: (a: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onRefreshAccessToken: (a: Account) => Promise<void>;
  onSubmitOTP: (jobId: string, otp: string) => Promise<void>;
  onResendOTP: (jobId: string) => Promise<void>;
  onConfirmManualPayment: (jobId: string) => Promise<void>;
  onSelectAddBalance: (jobId: string, method: ConcreteGoPayAddBalanceMethod) => Promise<void>;
  onConfirmAddBalance: (jobId: string) => Promise<void>;
}) {
  if (accountBusy && currentWorkflow && !isUserAlreadyExistsAccount(account)) {
    return (
      <RecordActions className="rowActions">
        <AccountRunningWorkflowActions job={currentWorkflow} busy={busy} onOpenWorkflow={onOpenWorkflow} onCancelWorkflow={onCancelWorkflow} onSubmitOTP={onSubmitOTP} onResendOTP={onResendOTP} onConfirmManualPayment={onConfirmManualPayment} onSelectAddBalance={onSelectAddBalance} onConfirmAddBalance={onConfirmAddBalance} />
      </RecordActions>
    );
  }

  const actions: RowActionDescriptor[] = [];
  if (canRegister(account)) actions.push({ label: '注册账号', icon: <Play size={14} />, onClick: () => onRegister(account), disabled: busy, kind: 'primary' });
  if (canRefreshAccessToken(account)) actions.push({ label: refreshingAccessToken ? '获取中' : '获取 Access', icon: <KeyRound size={14} />, onClick: () => void onRefreshAccessToken(account), disabled: busy || refreshingAccessToken, kind: actions.length ? 'secondary' : 'primary' });
  if (canLoginSession(account)) {
    actions.push({ label: '生成 auth.json', icon: <FileKey size={14} />, onClick: () => onCodexOAuthAddPhone(account), disabled: busy, kind: 'secondary' });
    actions.push({ label: '协议 auth.json', icon: <FileKey size={14} />, onClick: () => onCodexOAuthProtocol(account), disabled: busy, kind: 'secondary' });
  }

  const paymentActions: RowActionDescriptor[] = canGoPayPayment(account) ? GO_PAY_PAYMENT_CHANNELS.filter((channel) => channel !== 'wa').map((channel) => ({
    label: goPayPaymentActionLabel(channel),
    icon: <span className="activationPaymentIcon"><Zap size={13} /><PaymentChannelIcon channel={channel} /></span>,
    onClick: () => onGoPayPayment(account, channel),
    disabled: busy,
    kind: 'secondary' as const,
    className: 'paymentIconAction activationAction'
  })) : [];
  const primary = actions.find((action) => action.kind === 'primary' && !action.disabled) || actions.find((action) => !action.disabled) || actions[0];
  const leftActions = paymentActions;
  const rightActions = primary ? [primary, ...actions.filter((action) => action !== primary)] : actions;
  return (
    <RecordActions className="rowActions">
      <div className="rowActionsMain splitRowActions">
        <div className="rowActionsLeft"><RecordActionButtons actions={leftActions} /></div>
        <div className="rowActionsRight"><RecordActionButtons actions={rightActions} /></div>
      </div>
    </RecordActions>
  );
}
