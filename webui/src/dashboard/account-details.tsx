import { FileKey, Inbox, KeyRound, Search, Trash2, Zap } from 'lucide-react';
import { ActionButtonGroup, KVList } from '@/dashboard/module-kit';
import type { ActionButtonDescriptor, KVDescriptor } from '@/dashboard/module-kit';
import { MailboxOtpPanel, maskEmail } from '@/dashboard/modules/mailbox/sdk';
import {
  formatUnix,
  mask
} from '@/dashboard/module-kit';
import { AccountSignalBadge, PaymentChannelIcon } from './account-badges';
import { ActivationChannelEditor, TokenEditor } from './account-detail-editors';
import { accountInboxHint } from './account-mail-utils';
import { accountSignalText, canGoPayPayment, canLoginSession, canProbeAccount, canRefreshAccessToken, loginActionHint, loginActionLabel, probeAccountHint } from './account-utils';
import type { Account, AccountMailboxContext, ConcreteGoPayPaymentChannel, LatestOtp } from './types';

export function AccountDetails({ account, showSecrets, busy, inboxLoading, refreshingAccessToken, mailboxContext, latestOtp, activationChannel, onCopy, onFetchInbox, onSessionSave, onAccessSave, onActivationChannelSave, onProbeAccount, onLogin, onCodexOAuthAddPhone, onGoPayPayment, onRefreshAccessToken, onDelete }: {
  account: Account;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  refreshingAccessToken: boolean;
  mailboxContext: AccountMailboxContext | null;
  latestOtp: LatestOtp | null;
  activationChannel: string;
  onCopy: (label: string, value: string) => void;
  onFetchInbox: (account: Account) => Promise<void>;
  onSessionSave: (account: Account, sessionToken: string) => Promise<void>;
  onAccessSave: (account: Account, accessToken: string) => Promise<void>;
  onActivationChannelSave: (account: Account, activationChannel: string) => Promise<void>;
  onProbeAccount: (account: Account) => void;
  onLogin: (account: Account) => void;
  onCodexOAuthAddPhone: (account: Account) => void;
  onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onRefreshAccessToken: (account: Account) => Promise<void>;
  onDelete: (account: Account) => Promise<void>;
}) {
  const accountActions: ActionButtonDescriptor[] = [{
    id: 'gopay-wa-payment',
    visible: canGoPayPayment(account),
    label: '纯WA支付',
    hint: '只走 GoPay WA 链接支付，不执行 GoPay App 注册/登录/加余额',
    icon: <span className="activationPaymentIcon"><Zap size={13} /><PaymentChannelIcon channel="wa" /></span>,
    className: 'activationAction',
    disabled: busy,
    onClick: () => onGoPayPayment(account, 'wa'),
  }, {
    id: 'codex-oauth-add-phone',
    visible: canLoginSession(account),
    label: '生成 auth.json',
    hint: '自动 OAuth 登录，必要时完成加手机号，产出 auth.json',
    icon: <FileKey size={14} />,
    disabled: busy,
    onClick: () => onCodexOAuthAddPhone(account),
  }, {
    id: 'refresh-access-token',
    visible: canRefreshAccessToken(account),
    label: refreshingAccessToken ? '获取中' : '自动获取 Access Token',
    hint: '使用当前 Session 自动获取 Access Token',
    icon: <KeyRound size={14} />,
    disabled: busy || refreshingAccessToken,
    onClick: () => void onRefreshAccessToken(account),
  }, {
    id: 'login-session',
    visible: canLoginSession(account),
    label: loginActionLabel(account),
    hint: loginActionHint(account),
    icon: <KeyRound size={14} />,
    disabled: busy,
    onClick: () => onLogin(account),
  }, {
    id: 'probe-account',
    label: '探测账号',
    hint: probeAccountHint(account),
    icon: <Search size={14} />,
    disabled: busy || !canProbeAccount(account),
    onClick: () => onProbeAccount(account),
  }, {
    id: 'fetch-otp',
    label: inboxLoading ? '拉取中' : '拉取 OTP',
    hint: accountInboxHint(account.email, mailboxContext, showSecrets),
    icon: <Inbox size={14} />,
    disabled: busy || inboxLoading || !account.email,
    onClick: () => void onFetchInbox(account),
  }, {
    id: 'delete-account',
    label: '删除账号',
    hint: '删除当前账号记录',
    icon: <Trash2 size={14} />,
    variant: 'destructive',
    disabled: busy,
    onClick: () => void onDelete(account),
  }];
  const summaryFields: KVDescriptor[] = [{
    id: 'account-status',
    label: '账号结果',
    value: accountSignalText(account),
    copyValue: account.status || '-',
  }];
  const credentialFields: KVDescriptor[] = [{
    id: 'email',
    label: '邮箱',
    value: showSecrets ? account.email : maskEmail(account.email),
    copyValue: account.email,
    copyDisabled: !account.email,
    masked: !showSecrets,
  }, {
    id: 'password',
    label: '密码',
    value: showSecrets ? account.password : mask(account.password),
    copyValue: account.password,
    copyDisabled: !account.password,
    masked: !showSecrets,
    mono: true,
  }];
  const timeFields: KVDescriptor[] = [{
    id: 'created-at',
    label: '创建时间',
    value: formatUnix(account.created_at),
  }, {
    id: 'updated-at',
    label: '更新时间',
    value: formatUnix(account.updated_at),
  }];

  return (
    <div className="details">
      <section>
        <div className="sectionTitle">
          <h3>账号</h3>
          <ActionButtonGroup className="sectionActions" actions={accountActions} />
        </div>
        <MailboxOtpPanel latestOtp={latestOtp} showSecrets={showSecrets} loading={inboxLoading} onCopy={onCopy} />
        <AccountSignalBadge account={account} />
        <KVList items={summaryFields} onCopy={onCopy} />
        <ActivationChannelEditor account={account} activationChannel={activationChannel} onSave={onActivationChannelSave} />
        <KVList items={credentialFields} onCopy={onCopy} />
        <TokenEditor label="Session" field="session_token" account={account} showSecrets={showSecrets} onCopy={onCopy} onSave={onSessionSave} />
        <TokenEditor label="Access" field="access_token" account={account} showSecrets={showSecrets} onCopy={onCopy} onSave={onAccessSave} />
        <KVList items={timeFields} onCopy={onCopy} />
      </section>
    </div>
  );
}
