import { Copy, KeyRound, Play, Search, ShieldCheck, Trash2 } from 'lucide-react';
import {
  IconActionButton,
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
  canGoPayPayment,
  canLoginSession,
  canProbeAccount,
  canRefreshAccessToken,
  canRegister,
  isUserAlreadyExistsAccount,
  loginActionLabel
} from './account-utils';
import { AccountChannelTag, AccountSignalBadge, PaymentChannelIcon } from './account-badges';
import { AccountRunningWorkflowActions } from './account-otp-actions';
import { OpenAIIcon } from './brand-icons';
import { GO_PAY_PAYMENT_CHANNELS, goPayPaymentChannelLabel } from './gopay-utils';
import type { Account, ConcreteGoPayPaymentChannel, Job } from './types';

export function AccountTable({ accounts, jobs, selected, showSecrets, runningAccountIds, runningWorkflowByAccountID, refreshingAccessTokenIds, busy, onSelect, onOpenWorkflow, onRegister, onLogin, onGoPayPayment, onProbeAccount, onRegisterActivate, onRefreshAccessToken, onDelete, onCopy, onSubmitOTP, onResendOTP }: {
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
  onRegister: (a: Account) => void;
  onLogin: (a: Account) => void;
  onGoPayPayment: (a: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onProbeAccount: (a: Account) => void;
  onRegisterActivate: (a: Account) => void;
  onRefreshAccessToken: (a: Account) => Promise<void>;
  onDelete: (a: Account) => void;
  onCopy: (label: string, value: string) => void;
  onSubmitOTP: (jobId: string, otp: string) => Promise<void>;
  onResendOTP: (jobId: string) => Promise<void>;
}) {
  return (
    <RecordList className="accountsList" emptyText="暂无账号。可以先创建账号，或切换为全部状态查看。">
      {accounts.map((account) => {
        const accountBusy = runningAccountIds.has(account.account_id);
        const currentWorkflow = runningWorkflowByAccountID.get(account.account_id);
        const refreshingAccessToken = refreshingAccessTokenIds.has(account.account_id);
        const activationChannel = accountActivationChannel(account, jobs);
        return (
          <RecordCard key={account.account_id} selected={selected === account.account_id} onClick={() => onSelect(account)}>
            <RecordMain>
              <RecordTop>
                <AccountCardIdentity account={account} showSecrets={showSecrets} onCopy={onCopy} />
                <div className="accountCardTags">
                  <AccountSignalBadge account={account} compact />
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
              onRegister={onRegister}
              onLogin={onLogin}
              onGoPayPayment={onGoPayPayment}
              onProbeAccount={onProbeAccount}
              onRegisterActivate={onRegisterActivate}
              onRefreshAccessToken={onRefreshAccessToken}
              onDelete={onDelete}
              onSubmitOTP={onSubmitOTP}
              onResendOTP={onResendOTP}
            />
          </RecordCard>
        );
      })}
    </RecordList>
  );
}

function AccountCardIdentity({ account, showSecrets, onCopy }: {
  account: Account;
  showSecrets: boolean;
  onCopy: (label: string, value: string) => void;
}) {
  const email = account.email || '-';
  const displayEmail = showSecrets ? email : maskEmail(email);
  return (
    <RecordIdentity
      icon={<OpenAIIcon size={15} />}
      title={(
        <span className="accountCardEmail">
          <span title={displayEmail}>{displayEmail}</span>
          <IconActionButton
            className="inlineCopyAction"
            label="复制邮箱"
            icon={<Copy size={13} />}
            disabled={!account.email}
            onClick={(event) => {
              event.stopPropagation();
              onCopy('邮箱', account.email);
            }}
          />
        </span>
      )}
    />
  );
}

function AccountRowActions({ account, accountBusy, currentWorkflow, busy, refreshingAccessToken, onOpenWorkflow, onRegister, onLogin, onGoPayPayment, onProbeAccount, onRegisterActivate, onRefreshAccessToken, onDelete, onSubmitOTP, onResendOTP }: {
  account: Account;
  accountBusy: boolean;
  currentWorkflow?: Job;
  busy: boolean;
  refreshingAccessToken: boolean;
  onOpenWorkflow: (job: Job) => void;
  onRegister: (a: Account) => void;
  onLogin: (a: Account) => void;
  onGoPayPayment: (a: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onProbeAccount: (a: Account) => void;
  onRegisterActivate: (a: Account) => void;
  onRefreshAccessToken: (a: Account) => Promise<void>;
  onDelete: (a: Account) => void;
  onSubmitOTP: (jobId: string, otp: string) => Promise<void>;
  onResendOTP: (jobId: string) => Promise<void>;
}) {
  if (accountBusy && currentWorkflow && !isUserAlreadyExistsAccount(account)) {
    return (
      <RecordActions className="rowActions">
        <AccountRunningWorkflowActions job={currentWorkflow} busy={busy} onOpenWorkflow={onOpenWorkflow} onSubmitOTP={onSubmitOTP} onResendOTP={onResendOTP} />
      </RecordActions>
    );
  }

  const actions: RowActionDescriptor[] = [];
  if (canRegister(account)) actions.push({ label: '注册账号', icon: <Play size={14} />, onClick: () => onRegister(account), disabled: busy, kind: 'primary' });
  if (canRefreshAccessToken(account)) actions.push({ label: refreshingAccessToken ? '获取中' : '获取 Access', icon: <KeyRound size={14} />, onClick: () => void onRefreshAccessToken(account), disabled: busy || refreshingAccessToken, kind: actions.length ? 'secondary' : 'primary' });
  if (canLoginSession(account)) actions.push({ label: loginActionLabel(account), icon: <KeyRound size={14} />, onClick: () => onLogin(account), disabled: busy, kind: actions.length ? 'secondary' : 'primary' });
  if (canProbeAccount(account)) actions.push({ label: '探测账号', icon: <Search size={14} />, onClick: () => onProbeAccount(account), disabled: busy, kind: 'secondary' });
  if (canRegister(account)) actions.push({ label: '注册并激活', icon: <ShieldCheck size={14} />, onClick: () => onRegisterActivate(account), disabled: busy, kind: 'secondary' });
  actions.push({ label: '删除账号', icon: <Trash2 size={14} />, onClick: () => onDelete(account), disabled: busy, kind: 'danger' });

  const paymentActions: RowActionDescriptor[] = canGoPayPayment(account) ? GO_PAY_PAYMENT_CHANNELS.map((channel) => ({
    label: goPayPaymentChannelLabel(channel),
    icon: <PaymentChannelIcon channel={channel} />,
    onClick: () => onGoPayPayment(account, channel),
    disabled: busy,
    kind: 'secondary' as const,
    className: 'paymentIconAction'
  })) : [];
  const primary = actions.find((action) => action.kind === 'primary' && !action.disabled) || actions.find((action) => !action.disabled) || actions[0];
  const orderedActions = primary ? [primary, ...actions.filter((action) => action !== primary), ...paymentActions] : [...actions, ...paymentActions];
  return (
    <RecordActions className="rowActions">
      <div className="rowActionsMain">
        <RecordActionButtons actions={orderedActions} />
      </div>
    </RecordActions>
  );
}
