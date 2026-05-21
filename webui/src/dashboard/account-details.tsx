import { Inbox, KeyRound, Search } from 'lucide-react';
import { ActionButtonGroup, KVList } from '@/dashboard/module-kit';
import type { ActionButtonDescriptor, KVDescriptor } from '@/dashboard/module-kit';
import { MailboxOtpPanel, maskEmail } from '@/dashboard/modules/mailbox/sdk';
import {
  formatUnix,
  mask
} from '@/dashboard/module-kit';
import { AccountSignalBadge } from './account-badges';
import { ActivationChannelEditor, TokenEditor } from './account-detail-editors';
import { accountInboxHint } from './account-mail-utils';
import { accountSignalText, canLoginSession, canProbeAccount, canRefreshAccessToken, loginActionHint, loginActionLabel, probeAccountHint } from './account-utils';
import type { Account, AccountMailboxContext, LatestOtp } from './types';

export function AccountDetails({ account, showSecrets, busy, inboxLoading, refreshingAccessToken, mailboxContext, latestOtp, activationChannel, onCopy, onFetchInbox, onSessionSave, onAccessSave, onActivationChannelSave, onProbeAccount, onLogin, onRefreshAccessToken }: {
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
  onRefreshAccessToken: (account: Account) => Promise<void>;
}) {
  const accountActions: ActionButtonDescriptor[] = [{
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
