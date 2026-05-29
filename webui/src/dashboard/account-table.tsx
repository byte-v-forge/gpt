import { Trash2, Zap } from 'lucide-react';
import {
  RecordActionButtons,
  RecordActions,
  RecordCard,
  RecordIdentity,
  RecordList,
  RecordMain,
  RecordTop
} from '@byte-v-forge/common-ui';
import type { RowActionDescriptor } from '@byte-v-forge/common-ui';
import { maskEmail } from '@byte-v-forge/common-ui';
import {
  canGoPayPayment,
  isInvalidGptAccount,
  isUserAlreadyExistsAccount
} from './account-utils';
import { accountActivationChannel, accountCodexPhoneState } from './account-job-semantics';
import { AccountChannelTag, AccountCodexPhoneTag, AccountSignalBadge, PaymentChannelIcon } from './account-badges';
import { GPT_ACTIONS, gptActionAvailability, type GptActionCatalog } from './action-catalog';
import { AccountRowAuthGroups } from './account-row-auth-groups';
import { OpenAIIcon } from './brand-icons';
import { GO_PAY_PAYMENT_CHANNELS, goPayPaymentActionLabel } from './gopay-utils';
import type { Account, ConcreteGoPayPaymentChannel, Job } from './types';

export function AccountTable({ accounts, jobs, selected, actionCatalog, showSecrets, runningAccountIds, runningWorkflowByAccountID, busy, onSelect, onRegisterProtocol, onGoPayPayment, onDelete }: {
  accounts: Account[];
  jobs: Job[];
  selected?: string;
  actionCatalog?: GptActionCatalog;
  showSecrets: boolean;
  runningAccountIds: Set<string>;
  runningWorkflowByAccountID: Map<string, Job>;
  busy: boolean;
  onSelect: (a: Account) => void;
  onRegisterProtocol: (a: Account) => void;
  onGoPayPayment: (a: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onDelete: (a: Account) => void | Promise<void>;
}) {
  return (
    <RecordList className="accountsList" emptyText="暂无账号。可以先创建账号，或切换为全部状态查看。">
      {accounts.map((account) => {
        const accountBusy = runningAccountIds.has(account.account_id);
        const currentWorkflow = runningWorkflowByAccountID.get(account.account_id);
        const activationChannel = accountActivationChannel(account, jobs, actionCatalog);
        const phoneState = accountCodexPhoneState(account, jobs, actionCatalog);
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
              actionCatalog={actionCatalog}
              accountBusy={accountBusy}
              currentWorkflow={currentWorkflow}
              busy={busy}
              onRegisterProtocol={onRegisterProtocol}
              onGoPayPayment={onGoPayPayment}
              onDelete={onDelete}
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

function AccountRowActions({ account, actionCatalog, accountBusy, currentWorkflow, busy, onRegisterProtocol, onGoPayPayment, onDelete }: {
  account: Account;
  actionCatalog?: GptActionCatalog;
  accountBusy: boolean;
  currentWorkflow?: Job;
  busy: boolean;
  onRegisterProtocol: (a: Account) => void;
  onGoPayPayment: (a: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onDelete: (a: Account) => void | Promise<void>;
}) {
  if (isInvalidGptAccount(account)) {
    const actions: RowActionDescriptor[] = [{
      label: '删除账号',
      icon: <Trash2 size={14} />,
      onClick: () => void onDelete(account),
      disabled: busy,
      kind: 'danger'
    }];
    return (
      <RecordActions className="rowActions">
        <div className="rowActionsMain"><RecordActionButtons actions={actions} /></div>
      </RecordActions>
    );
  }

  if (accountBusy && currentWorkflow && !isUserAlreadyExistsAccount(account)) {
    return (
      <RecordActions className="rowActions">
        <div className="rowActionsMain"><span className="accountWorkflowNotice">流程运行中，请到工作流页处理</span></div>
      </RecordActions>
    );
  }

  const payment = gptActionAvailability(actionCatalog, GPT_ACTIONS.goPayPayment, account, 'account_row');
  const paymentActions: RowActionDescriptor[] = GO_PAY_PAYMENT_CHANNELS.filter((channel) => channel !== 'wa' && payment.visible).map((channel) => ({
    label: goPayPaymentActionLabel(channel),
    icon: <span className="activationPaymentIcon"><Zap size={13} /><PaymentChannelIcon channel={channel} /></span>,
    onClick: () => onGoPayPayment(account, channel),
    disabled: busy || !payment.enabled || !canGoPayPayment(account),
    kind: 'secondary' as const,
    className: 'paymentIconAction activationAction'
  }));
  const leftActions = paymentActions;
  return (
    <RecordActions className="rowActions">
      <div className="rowActionsMain splitRowActions">
        <div className="rowActionsLeft"><RecordActionButtons actions={leftActions} /></div>
        <div className="rowActionsRight">
          <AccountRowAuthGroups account={account} actionCatalog={actionCatalog} busy={busy} onRegisterProtocol={onRegisterProtocol} />
        </div>
      </div>
    </RecordActions>
  );
}
