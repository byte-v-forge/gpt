import { Bug, Inbox, KeyRound, RefreshCw, Search, Trash2 } from 'lucide-react';
import { ActionButtonGroup } from '@/dashboard/module-kit';
import type { ActionButtonDescriptor } from '@/dashboard/module-kit';
import { PaymentChannelIcon } from './account-badges';
import { CodexIcon } from './brand-icons';
import { accountInboxHint } from './account-mail-utils';
import { canGoPayPayment, canLoginSession, canProbeAccount, canRefreshAccessToken, loginActionHint, loginActionLabel, probeAccountHint } from './account-utils';
import { GO_PAY_PAYMENT_CHANNELS, goPayPaymentActionLabel } from './gopay-utils';
import type { Account, AccountMailboxContext, ConcreteGoPayPaymentChannel } from './types';

export function AccountDetailActions({ account, showSecrets, busy, inboxLoading, refreshingAccessToken, mailboxContext, canFetchOTP, onFetchInbox, onProbeAccount, onLogin, onCodexOAuthAddPhone, onGoPayPayment, onRefreshAccessToken, onDelete }: {
  account: Account;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  refreshingAccessToken: boolean;
  mailboxContext: AccountMailboxContext | null;
  canFetchOTP: boolean;
  onFetchInbox: (account: Account) => Promise<void>;
  onProbeAccount: (account: Account) => void;
  onLogin: (account: Account) => void;
  onCodexOAuthAddPhone: (account: Account) => void;
  onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onRefreshAccessToken: (account: Account) => Promise<void>;
  onDelete: (account: Account) => Promise<void>;
}) {
  const rows = [{
    title: '账号',
    actions: accountActions(account, busy, refreshingAccessToken, onLogin, onRefreshAccessToken, onCodexOAuthAddPhone, onProbeAccount),
  }, {
    title: 'OTP',
    actions: mailboxActions(account, showSecrets, busy, inboxLoading, mailboxContext, canFetchOTP, onFetchInbox),
  }, {
    title: '渠道',
    actions: channelActions(account, busy, onGoPayPayment),
  }, {
    title: '危险',
    actions: dangerActions(account, busy, onDelete),
  }];
  return (
    <div className="detailActionRows">
      {rows.map((row) => row.actions.some((action) => action.visible !== false) && (
        <div className="detailActionRow" key={row.title}>
          <span className="detailActionLabel">{row.title}</span>
          <ActionButtonGroup className="sectionActions" actions={row.actions} />
        </div>
      ))}
    </div>
  );
}

function accountActions(account: Account, busy: boolean, refreshing: boolean, onLogin: (account: Account) => void, onRefresh: (account: Account) => Promise<void>, onAuth: (account: Account) => void, onProbe: (account: Account) => void): ActionButtonDescriptor[] {
  return [{
    id: 'login-session',
    visible: canLoginSession(account),
    label: loginActionLabel(account),
    hint: loginActionHint(account),
    icon: <KeyRound size={14} />,
    disabled: busy,
    onClick: () => onLogin(account),
  }, {
    id: 'refresh-access-token',
    visible: canRefreshAccessToken(account),
    label: refreshing ? '刷新中' : '刷新 Token',
    hint: '使用当前 Session 刷新 Access Token',
    icon: <RefreshCw size={14} />,
    disabled: busy || refreshing,
    onClick: () => void onRefresh(account),
  }, {
    id: 'codex-oauth-add-phone',
    visible: canLoginSession(account),
    label: '获取 auth.json',
    hint: '自动 OAuth 登录，必要时完成加手机号，产出 auth.json',
    icon: <CodexIcon size={14} />,
    disabled: busy,
    onClick: () => onAuth(account),
  }, {
    id: 'probe-account',
    label: '探测账号',
    hint: probeAccountHint(account),
    icon: <Search size={14} />,
    disabled: busy || !canProbeAccount(account),
    onClick: () => onProbe(account),
  }];
}

function mailboxActions(account: Account, showSecrets: boolean, busy: boolean, inboxLoading: boolean, mailboxContext: AccountMailboxContext | null, canFetchOTP: boolean, onFetchInbox: (account: Account) => Promise<void>): ActionButtonDescriptor[] {
  return [{
    id: 'fetch-otp',
    visible: canFetchOTP,
    label: inboxLoading ? '拉取中' : '拉取 OTP',
    hint: accountInboxHint(account.email, mailboxContext, showSecrets),
    icon: <Inbox size={14} />,
    disabled: busy || inboxLoading,
    onClick: () => void onFetchInbox(account),
  }];
}

function channelActions(account: Account, busy: boolean, onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void): ActionButtonDescriptor[] {
  if (!canGoPayPayment(account)) return [];
  return GO_PAY_PAYMENT_CHANNELS.map((channel) => ({
    id: `gopay-payment-${channel}`,
    label: goPayPaymentActionLabel(channel),
    hint: channel === 'wa' ? 'Debug：只走 GoPay WA 链接支付' : 'GoPay 激活支付渠道',
    icon: channel === 'wa' ? <Bug size={14} /> : <span className="activationPaymentIcon"><PaymentChannelIcon channel={channel} /></span>,
    className: channel === 'wa' ? 'debugAction' : 'activationAction',
    disabled: busy,
    onClick: () => onGoPayPayment(account, channel),
  }));
}

function dangerActions(account: Account, busy: boolean, onDelete: (account: Account) => Promise<void>): ActionButtonDescriptor[] {
  return [{
    id: 'delete-account',
    label: '删除账号',
    hint: '删除当前账号记录',
    icon: <Trash2 size={14} />,
    variant: 'destructive',
    disabled: busy,
    onClick: () => void onDelete(account),
  }];
}
