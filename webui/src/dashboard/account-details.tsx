import { KVList } from '@/dashboard/module-kit';
import type { KVDescriptor } from '@/dashboard/module-kit';
import { maskEmail } from '@/dashboard/modules/mailbox/sdk';
import {
  formatUnix,
  mask
} from '@/dashboard/module-kit';
import { AccountChannelTag, AccountCodexPhoneTag, AccountSignalBadge } from './account-badges';
import { AccountPrimaryActions } from './account-auth-action-groups';
import { AccountDangerActions, AccountDetailActions } from './account-detail-actions';
import { TokenEditor } from './account-detail-editors';
import { canFetchAccountInbox } from './account-mail-utils';
import type { AccountCodexPhoneState } from './account-utils';
import type { Account, AccountMailboxContext, ConcreteGoPayPaymentChannel, LatestOtp } from './types';

export function AccountDetails({ account, showSecrets, busy, inboxLoading, refreshingAccessToken, mailboxContext, latestOtp, activationChannel, codexPhoneState, onCopy, onFetchInbox, onSessionSave, onAccessSave, onProbeAccount, onRegister, onRegisterProtocol, onLogin, onLoginProtocol, onCodexOAuthAddPhone, onCodexOAuthProtocol, onGoPayPayment, onRefreshAccessToken, onDelete }: {
  account: Account;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  refreshingAccessToken: boolean;
  mailboxContext: AccountMailboxContext | null;
  latestOtp: LatestOtp | null;
  activationChannel: string;
  codexPhoneState: AccountCodexPhoneState;
  onCopy: (label: string, value: string) => void;
  onFetchInbox: (account: Account) => Promise<void>;
  onSessionSave: (account: Account, sessionToken: string) => Promise<void>;
  onAccessSave: (account: Account, accessToken: string) => Promise<void>;
  onProbeAccount: (account: Account) => void;
  onRegister: (account: Account) => void;
  onRegisterProtocol: (account: Account) => void;
  onLogin: (account: Account) => void;
  onLoginProtocol: (account: Account) => void;
  onCodexOAuthAddPhone: (account: Account) => void;
  onCodexOAuthProtocol: (account: Account) => void;
  onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void;
  onRefreshAccessToken: (account: Account) => Promise<void>;
  onDelete: (account: Account) => Promise<void>;
}) {
  const canFetchOTP = canFetchAccountInbox(account, mailboxContext);
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
  const codexAuthFields: KVDescriptor[] = [{
    id: 'codex-auth-json',
    label: 'auth.json',
    value: showSecrets ? account.codex_auth_json : mask(account.codex_auth_json),
    copyValue: account.codex_auth_json,
    copyDisabled: !account.codex_auth_json,
    copyHint: '暂无 auth.json',
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
  }, {
    id: 'codex-auth-updated-at',
    label: 'auth.json更新时间',
    value: formatUnix(account.codex_auth_updated_at_unix),
    visible: !!account.codex_auth_updated_at_unix,
  }];

  return (
    <div className="details">
      <section>
        <div className="sectionTitle accountSectionTitle">
          <h3>账号 <AccountSignalBadge account={account} compact /><AccountCodexPhoneTag state={codexPhoneState} /><AccountChannelTag channel={activationChannel} /></h3>
          <AccountPrimaryActions account={account} busy={busy} refreshingAccessToken={refreshingAccessToken} onProbeAccount={onProbeAccount} onRegister={onRegister} onRegisterProtocol={onRegisterProtocol} onLogin={onLogin} onLoginProtocol={onLoginProtocol} onCodexOAuthAddPhone={onCodexOAuthAddPhone} onCodexOAuthProtocol={onCodexOAuthProtocol} onRefreshAccessToken={onRefreshAccessToken} />
        </div>
        <AccountDetailActions account={account} showSecrets={showSecrets} busy={busy} inboxLoading={inboxLoading} mailboxContext={mailboxContext} latestOtp={latestOtp} canFetchOTP={canFetchOTP} onCopy={onCopy} onFetchInbox={onFetchInbox} onGoPayPayment={onGoPayPayment} />
        <KVList items={credentialFields} onCopy={onCopy} />
        <TokenEditor label="Session" field="session_token" account={account} showSecrets={showSecrets} onCopy={onCopy} onSave={onSessionSave} />
        <TokenEditor label="Access" field="access_token" account={account} showSecrets={showSecrets} onCopy={onCopy} onSave={onAccessSave} />
        <KVList items={codexAuthFields} onCopy={onCopy} />
        <KVList items={timeFields} onCopy={onCopy} />
        <AccountDangerActions account={account} busy={busy} onDelete={onDelete} />
      </section>
    </div>
  );
}
