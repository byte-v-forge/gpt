import { ContentTabs, KVList } from '@byte-v-forge/common-ui';
import type { KVDescriptor } from '@byte-v-forge/common-ui';
import { maskEmail } from '@byte-v-forge/common-ui';
import {
  formatUnix,
  mask
} from '@byte-v-forge/common-ui';
import { AccountChannelTag, AccountCodexPhoneTag, AccountSignalBadge } from './account-badges';
import { AccountFingerprintPanel } from './account-fingerprint-panel';
import { AccountPrimaryActions } from './account-auth-action-groups';
import { AccountDangerActions, AccountDetailActions } from './account-detail-actions';
import { TokenEditor } from './account-detail-editors';
import { canFetchAccountInbox } from './account-mail-utils';
import { AccountProxyHistoryPanel } from './account-proxy-history-panel';
import { isInvalidGptAccount } from './account-utils';
import type { AccountCodexPhoneState } from './account-utils';
import type { Account, AccountMailboxContext, ConcreteGoPayPaymentChannel, LatestOtp, MailboxProviderCapability } from './types';

export function AccountDetails({ account, showSecrets, busy, inboxLoading, refreshingAccessToken, mailboxContext, mailboxProviderCapabilities, latestOtp, activationChannel, codexPhoneState, onCopy, onFetchInbox, onSessionSave, onAccessSave, onProbeAccount, onRegister, onRegisterProtocol, onLogin, onLoginProtocol, onCodexOAuthAddPhone, onCodexOAuthProtocol, onGoPayPayment, onRefreshAccessToken, onDelete }: {
  account: Account;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  refreshingAccessToken: boolean;
  mailboxContext: AccountMailboxContext | null;
  mailboxProviderCapabilities: MailboxProviderCapability[];
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
  const invalid = isInvalidGptAccount(account);
  const canFetchOTP = !invalid && canFetchAccountInbox(account, mailboxContext, mailboxProviderCapabilities);
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

  const overview = (
    <section>
      <div className="sectionTitle accountSectionTitle">
        <h3>账号 <AccountSignalBadge account={account} compact /><AccountCodexPhoneTag state={codexPhoneState} /><AccountChannelTag channel={activationChannel} /></h3>
      </div>
      {!invalid && (
        <div className="accountDetailActionStack">
          <AccountPrimaryActions account={account} busy={busy} refreshingAccessToken={refreshingAccessToken} onProbeAccount={onProbeAccount} onRegister={onRegister} onRegisterProtocol={onRegisterProtocol} onLogin={onLogin} onLoginProtocol={onLoginProtocol} onCodexOAuthAddPhone={onCodexOAuthAddPhone} onCodexOAuthProtocol={onCodexOAuthProtocol} onRefreshAccessToken={onRefreshAccessToken} />
          <AccountDetailActions account={account} showSecrets={showSecrets} busy={busy} inboxLoading={inboxLoading} mailboxContext={mailboxContext} latestOtp={latestOtp} canFetchOTP={canFetchOTP} onCopy={onCopy} onFetchInbox={onFetchInbox} onGoPayPayment={onGoPayPayment} />
        </div>
      )}
      <KVList items={credentialFields} onCopy={onCopy} />
      {!invalid && <TokenEditor label="Session" field="session_token" account={account} showSecrets={showSecrets} onCopy={onCopy} onSave={onSessionSave} />}
      {!invalid && <TokenEditor label="Access" field="access_token" account={account} showSecrets={showSecrets} onCopy={onCopy} onSave={onAccessSave} />}
      <KVList items={timeFields} onCopy={onCopy} />
      <AccountDangerActions account={account} busy={busy} onDelete={onDelete} />
    </section>
  );

  return (
    <div className="details accountDetails">
      <ContentTabs
        tabsListVariant="line"
        tabsClassName="accountDetailsTabs"
        tabs={[
          { value: 'overview', label: '基础信息', content: overview, contentClassName: 'accountDetailTabContent' },
          { value: 'fingerprint', label: '指纹', content: <AccountFingerprintPanel accountID={account.account_id} onCopy={onCopy} />, contentClassName: 'accountDetailTabContent' },
          { value: 'ip-history', label: 'IP 历史', content: <AccountProxyHistoryPanel accountID={account.account_id} />, contentClassName: 'accountDetailTabContent' }
        ]}
      />
    </div>
  );
}
