import { AccountTokenEditor, ContentTabs, KVList, canFetchAccountMailboxInbox, useQuery } from '@byte-v-forge/common-ui';
import type { AccountMailboxContext, KVDescriptor } from '@byte-v-forge/common-ui';
import { maskEmail } from '@byte-v-forge/common-ui';
import { formatUnix } from '@byte-v-forge/common-ui';
import { AccountChannelTag, AccountCodexPhoneTag, AccountSignalBadge } from './account-badges';
import type { AccountWorkflowRunner } from './account-action-specs';
import type { GptActionCatalog } from './action-catalog';
import { AccountFingerprintPanel } from './account-fingerprint-panel';
import { AccountPrimaryActions } from './account-auth-action-groups';
import { accountAuthQueryKey, loadAccountAuthTokens } from './account-auth-query';
import { AccountDangerActions, AccountDetailActions } from './account-detail-actions';
import { AccountProxyHistoryPanel } from './account-proxy-history-panel';
import { AccountWorkflowTimelinePanel } from './account-workflow-timeline-panel';
import { isInvalidGptAccount } from './account-utils';
import type { WorkflowJobActionMessageKind } from './job-action-renderers';
import type { AccountCodexPhoneState } from './account-job-semantics';
import type { Account, Job, LatestOtp } from './types';
import { accountCarrierID, accountCarrierEmail, accountCarrierCreatedAtUnix, accountCarrierUpdatedAtUnix } from '@byte-v-forge/common-ui';

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
  runWorkflow,
  onWorkflowChanged,
  onWorkflowMessage,
  onWorkflowError,
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
  runWorkflow: AccountWorkflowRunner;
  onWorkflowChanged?: () => void | Promise<void>;
  onWorkflowMessage?: (kind: WorkflowJobActionMessageKind, message: string) => void;
  onWorkflowError?: (error: unknown) => void;
  onUpdateWebAccessToken: (account: Account) => Promise<void>;
  onDelete: (account: Account) => Promise<void>;
}) {
  const invalid = isInvalidGptAccount(account);
  const canFetchOTP = !invalid && canFetchAccountMailboxInbox(account, mailboxContext);
  const authQuery = useQuery({
    queryKey: accountAuthQueryKey(accountCarrierID(account)),
    queryFn: () => loadAccountAuthTokens(accountCarrierID(account)),
    enabled: !invalid && showSecrets,
    refetchOnMount: 'always'
  });
  const auth = showSecrets ? authQuery.data : null;
  const credentialFields: KVDescriptor[] = [
    {
      id: 'email',
      label: '邮箱',
      value: showSecrets ? accountCarrierEmail(account) : maskEmail(accountCarrierEmail(account)),
      copyValue: accountCarrierEmail(account),
      copyDisabled: !accountCarrierEmail(account),
      masked: !showSecrets
    },
    {
      id: 'password',
      label: '密码',
      value: showSecrets ? auth?.password || '-' : '••••••••',
      copyValue: auth?.password || '',
      copyDisabled: !auth?.password,
      masked: !showSecrets,
      visible: !invalid
    }
  ];
  const timeFields: KVDescriptor[] = [
    {
      id: 'created-at',
      label: '创建时间',
      value: formatUnix(accountCarrierCreatedAtUnix(account))
    },
    {
      id: 'updated-at',
      label: '更新时间',
      value: formatUnix(accountCarrierUpdatedAtUnix(account))
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
          <AccountPrimaryActions account={account} actionCatalog={actionCatalog} busy={busy} updatingWebAccessToken={updatingWebAccessToken} runWorkflow={runWorkflow} onUpdateWebAccessToken={onUpdateWebAccessToken} />
          <AccountDetailActions account={account} actionCatalog={actionCatalog} showSecrets={showSecrets} busy={busy} inboxLoading={inboxLoading} mailboxContext={mailboxContext} latestOtp={latestOtp} canFetchOTP={canFetchOTP} runWorkflow={runWorkflow} onCopy={onCopy} onFetchInbox={onFetchInbox} />
        </div>
      )}
      <KVList items={credentialFields} onCopy={onCopy} />
      {!invalid && <AccountTokenEditor label="Session Token" field="session_token" account={account} token={auth?.session_token || ''} expiresAtUnix={auth?.session_token_expires_at_unix || 0} loading={authQuery.isLoading} showSecrets={showSecrets} onCopy={onCopy} onSave={onSessionSave} />}
      {!invalid && <AccountTokenEditor label="Web AT" field="access_token" account={account} token={auth?.access_token || ''} expiresAtUnix={auth?.access_token_expires_at_unix || 0} loading={authQuery.isLoading} showSecrets={showSecrets} onCopy={onCopy} onSave={onAccessSave} />}
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
            content: (
              <AccountWorkflowTimelinePanel
                accountID={accountCarrierID(account)}
                jobs={jobs}
                actionCatalog={actionCatalog}
                onChanged={onWorkflowChanged}
                onMessage={onWorkflowMessage}
                onError={onWorkflowError}
              />
            ),
            contentClassName: 'accountDetailTabContent'
          },
          {
            value: 'fingerprint',
            label: '指纹',
            content: <AccountFingerprintPanel accountID={accountCarrierID(account)} onCopy={onCopy} />,
            contentClassName: 'accountDetailTabContent'
          },
          {
            value: 'ip-history',
            label: 'IP 历史',
            content: <AccountProxyHistoryPanel accountID={accountCarrierID(account)} />,
            contentClassName: 'accountDetailTabContent'
          }
        ]}
      />
    </div>
  );
}
