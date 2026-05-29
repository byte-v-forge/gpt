import { ContentTabs, KVList, useQuery } from '@byte-v-forge/common-ui';
import type { KVDescriptor } from '@byte-v-forge/common-ui';
import { maskEmail } from '@byte-v-forge/common-ui';
import { formatUnix, mask } from '@byte-v-forge/common-ui';
import { AccountChannelTag, AccountCodexPhoneTag, AccountSignalBadge } from './account-badges';
import type { GptActionCatalog } from './action-catalog';
import { AccountFingerprintPanel } from './account-fingerprint-panel';
import { AccountPrimaryActions } from './account-auth-action-groups';
import { accountAuthQueryKey, loadAccountAuthTokens } from './account-auth-query';
import { AccountDangerActions, AccountDetailActions } from './account-detail-actions';
import { TokenEditor } from './account-detail-editors';
import { canFetchAccountInbox } from './account-mail-utils';
import { AccountProxyHistoryPanel } from './account-proxy-history-panel';
import { AccountWorkflowTimelinePanel } from './account-workflow-timeline-panel';
import { isInvalidGptAccount } from './account-utils';
import type { AccountCodexPhoneState } from './account-job-semantics';
import type { Account, AccountMailboxContext, ConcreteGoPayPaymentChannel, Job, LatestOtp } from './types';

export function AccountDetails({
  account,
  jobs,
  actionCatalog,
  showSecrets,
  busy,
  inboxLoading,
  updatingWebAccessToken,
  mailboxContext,
  latestOtp,
  activationChannel,
  codexPhoneState,
  onCopy,
  onFetchInbox,
  onSessionSave,
  onAccessSave,
  onProbeAccount,
  onRegister,
  onRegisterProtocol,
  onLogin,
  onLoginProtocol,
  onCodexOAuthAddPhone,
  onCodexOAuthProtocol,
  onGoPayPayment,
  onUpdateWebAccessToken,
  onDelete
}: {
  account: Account;
  jobs: Job[];
  actionCatalog?: GptActionCatalog;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  updatingWebAccessToken: boolean;
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
  onUpdateWebAccessToken: (account: Account) => Promise<void>;
  onDelete: (account: Account) => Promise<void>;
}) {
  const invalid = isInvalidGptAccount(account);
  const canFetchOTP = !invalid && canFetchAccountInbox(account, mailboxContext);
  const authQuery = useQuery({
    queryKey: accountAuthQueryKey(account.account_id),
    queryFn: () => loadAccountAuthTokens(account.account_id),
    enabled: !invalid && showSecrets,
    refetchOnMount: 'always'
  });
  const auth = showSecrets ? authQuery.data : null;
  const credentialFields: KVDescriptor[] = [
    {
      id: 'email',
      label: '邮箱',
      value: showSecrets ? account.email : maskEmail(account.email),
      copyValue: account.email,
      copyDisabled: !account.email,
      masked: !showSecrets
    },
    {
      id: 'password',
      label: '密码',
      value: showSecrets ? account.password : mask(account.password),
      copyValue: account.password,
      copyDisabled: !account.password,
      masked: !showSecrets,
      mono: true
    }
  ];
  const timeFields: KVDescriptor[] = [
    {
      id: 'created-at',
      label: '创建时间',
      value: formatUnix(account.created_at)
    },
    {
      id: 'updated-at',
      label: '更新时间',
      value: formatUnix(account.updated_at)
    }
  ];

  const overview = (
    <section>
      <div className="sectionTitle accountSectionTitle">
        <h3>
          账号 <AccountSignalBadge account={account} compact />
          <AccountCodexPhoneTag state={codexPhoneState} />
          <AccountChannelTag channel={activationChannel} />
        </h3>
      </div>
      {!invalid && (
        <div className="accountDetailActionStack">
          <AccountPrimaryActions account={account} actionCatalog={actionCatalog} busy={busy} updatingWebAccessToken={updatingWebAccessToken} onProbeAccount={onProbeAccount} onRegister={onRegister} onRegisterProtocol={onRegisterProtocol} onLogin={onLogin} onLoginProtocol={onLoginProtocol} onCodexOAuthAddPhone={onCodexOAuthAddPhone} onCodexOAuthProtocol={onCodexOAuthProtocol} onUpdateWebAccessToken={onUpdateWebAccessToken} />
          <AccountDetailActions account={account} actionCatalog={actionCatalog} showSecrets={showSecrets} busy={busy} inboxLoading={inboxLoading} mailboxContext={mailboxContext} latestOtp={latestOtp} canFetchOTP={canFetchOTP} onCopy={onCopy} onFetchInbox={onFetchInbox} onGoPayPayment={onGoPayPayment} />
        </div>
      )}
      <KVList items={credentialFields} onCopy={onCopy} />
      {!invalid && <TokenEditor label="Session Token" field="session_token" account={account} token={auth?.session_token || ''} expiresAtUnix={auth?.session_token_expires_at_unix || 0} loading={authQuery.isLoading} showSecrets={showSecrets} onCopy={onCopy} onSave={onSessionSave} />}
      {!invalid && <TokenEditor label="Web AT" field="access_token" account={account} token={auth?.access_token || ''} expiresAtUnix={auth?.access_token_expires_at_unix || 0} loading={authQuery.isLoading} showSecrets={showSecrets} onCopy={onCopy} onSave={onAccessSave} />}
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
          {
            value: 'overview',
            label: '基础信息',
            content: overview,
            contentClassName: 'accountDetailTabContent'
          },
          {
            value: 'workflows',
            label: '流程',
            content: <AccountWorkflowTimelinePanel accountID={account.account_id} jobs={jobs} actionCatalog={actionCatalog} />,
            contentClassName: 'accountDetailTabContent'
          },
          {
            value: 'fingerprint',
            label: '指纹',
            content: <AccountFingerprintPanel accountID={account.account_id} onCopy={onCopy} />,
            contentClassName: 'accountDetailTabContent'
          },
          {
            value: 'ip-history',
            label: 'IP 历史',
            content: <AccountProxyHistoryPanel accountID={account.account_id} />,
            contentClassName: 'accountDetailTabContent'
          }
        ]}
      />
    </div>
  );
}
