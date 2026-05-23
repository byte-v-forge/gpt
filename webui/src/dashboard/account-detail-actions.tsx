import { Bug, Copy, Inbox, KeyRound, RefreshCw, Search, Trash2 } from 'lucide-react';
import { ActionButtonGroup, Button, buttonHint, formatUnix, mask } from '@/dashboard/module-kit';
import type { ActionButtonDescriptor } from '@/dashboard/module-kit';
import { PaymentChannelIcon } from './account-badges';
import { CodexIcon } from './brand-icons';
import { accountInboxHint } from './account-mail-utils';
import { canGoPayPayment, canLoginSession, canProbeAccount, canRefreshAccessToken, loginActionHint, loginActionLabel, probeAccountHint } from './account-utils';
import { GO_PAY_PAYMENT_CHANNELS, goPayPaymentActionLabel } from './gopay-utils';
import type { Account, AccountMailboxContext, ConcreteGoPayPaymentChannel, LatestOtp } from './types';

export function AccountPrimaryActions({ account, busy, refreshingAccessToken, onProbeAccount, onLogin, onCodexOAuthAddPhone, onRefreshAccessToken }: {
  account: Account;
  busy: boolean;
  refreshingAccessToken: boolean;
  onProbeAccount: (account: Account) => void;
  onLogin: (account: Account) => void;
  onCodexOAuthAddPhone: (account: Account) => void;
  onRefreshAccessToken: (account: Account) => Promise<void>;
}) {
  return (
    <ActionButtonGroup
      className="sectionActions accountHeaderActions"
      actions={accountActions(account, busy, refreshingAccessToken, onLogin, onRefreshAccessToken, onCodexOAuthAddPhone, onProbeAccount)}
    />
  );
}

export function AccountDetailActions({ account, showSecrets, busy, inboxLoading, mailboxContext, latestOtp, canFetchOTP, onCopy, onFetchInbox, onGoPayPayment, onDelete }: {
  account: Account;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  mailboxContext: AccountMailboxContext | null;
  latestOtp: LatestOtp | null;
  canFetchOTP: boolean;
  onCopy: (label: string, value: string) => void;
  onFetchInbox: (account: Account) => Promise<void>;
  onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onDelete: (account: Account) => Promise<void>;
}) {
  const otpRow = mailboxActions(account, showSecrets, busy, inboxLoading, mailboxContext, canFetchOTP, onFetchInbox);
  const channelRow = channelActions(account, busy, onGoPayPayment);
  const dangerRow = dangerActions(account, busy, onDelete);
  return (
    <div className="detailActionRows">
      {(canFetchOTP || latestOtp) && (
        <div className="detailActionRow">
          <span className="detailActionLabel">OTP</span>
          <div className="detailActionContent">
            <LatestOTPValue latestOtp={latestOtp} showSecrets={showSecrets} onCopy={onCopy} />
            <ActionButtonGroup className="sectionActions" actions={otpRow} />
          </div>
        </div>
      )}
      {hasVisibleAction(channelRow) && <ActionRow label="激活" actions={channelRow} />}
      {hasVisibleAction(dangerRow) && <ActionRow label="危险" actions={dangerRow} />}
    </div>
  );
}

function ActionRow({ label, actions }: { label?: string; actions: ActionButtonDescriptor[] }) {
  return (
    <div className={`detailActionRow${label ? '' : ' unlabeled'}`}>
      {label && <span className="detailActionLabel">{label}</span>}
      <ActionButtonGroup className="sectionActions" actions={actions} />
    </div>
  );
}

function LatestOTPValue({ latestOtp, showSecrets, onCopy }: {
  latestOtp: LatestOtp | null;
  showSecrets: boolean;
  onCopy: (label: string, value: string) => void;
}) {
  const code = latestOtp?.otp || '';
  return (
    <span className={`detailOtpCode${code ? '' : ' empty'}`}>
      <strong>{code ? (showSecrets ? code : mask(code)) : '暂无 OTP'}</strong>
      {code && <em>{formatUnix(latestOtp?.received_at_unix || 0)}</em>}
      <Button className="copyButton detailOtpCopy" {...buttonHint('复制 OTP')} disabled={!code} onClick={() => onCopy('OTP', code)}>
        <Copy size={14} />
      </Button>
    </span>
  );
}

function hasVisibleAction(actions: ActionButtonDescriptor[]) {
  return actions.some((action) => action.visible !== false);
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
