import { useState } from 'react';
import { accountMailboxContextForEmail, latestOtpForEmail, ToastMessage, WorkspaceTabbedPanel } from '@byte-v-forge/common-ui';
import { AccountDetails } from './account-details';
import { useGptActionCatalog, type GptActionCatalog } from './action-catalog';
import { useGptAccountActions } from './account-actions';
import { useGptAccountData } from './account-data';
import { useGptAccountEventCache } from './account-events';
import { accountActivationChannel, accountCodexPhoneState } from './account-job-semantics';
import { GPTSettingsPage } from './gpt-settings-page';
import { GptAccountsView } from './view';
import { accountCarrierID, accountCarrierEmail } from '@byte-v-forge/common-ui';

type GptPageTab = 'accounts' | 'settings';

export function GptAccountsPage() {
  const [tab, setTab] = useState<GptPageTab>('accounts');
  const actionCatalog = useGptActionCatalog();
  return (
    <WorkspaceTabbedPanel<GptPageTab>
      title="GPT"
      value={tab}
      onValueChange={setTab}
      tabsListVariant="line"
      tabs={[
        {
          value: 'accounts',
          label: '账号',
          content: <GptAccountsTab actionCatalog={actionCatalog.data} />,
          contentClassName: 'flex flex-col overflow-hidden'
        },
        {
          value: 'settings',
          label: '设置',
          content: <GPTSettingsPage />,
          contentClassName: 'flex flex-col overflow-hidden'
        }
      ]}
    />
  );
}

function GptAccountsTab({ actionCatalog }: { actionCatalog?: GptActionCatalog }) {
  const [selectedAccountID, setSelectedAccountID] = useState('');
  const [showSecrets, setShowSecrets] = useState(true);
  const data = useGptAccountData(selectedAccountID, setSelectedAccountID);
  const actions = useGptAccountActions(data, showSecrets, setSelectedAccountID, actionCatalog);
  const busy = actions.working || data.busy;
  const selectedPhoneState = data.selected ? accountCodexPhoneState(data.selected, data.jobs, actions.actionCatalog) : null;
  useGptAccountEventCache();
  const selectedDetails = data.selected && selectedPhoneState ? (
    <AccountDetails
      account={data.selected}
      jobs={data.jobs}
      actionCatalog={actions.actionCatalog}
      showSecrets={showSecrets}
      busy={busy}
      inboxLoading={actions.inboxLoading}
      mailboxContext={accountMailboxContextForEmail(data.mailboxes, data.allocations, { email: accountCarrierEmail(data.selected), primary_mailbox_email: data.selected.primary_mailbox_email })}
      latestOtp={latestOtpForEmail(actions.inbox, data.mailboxes, accountCarrierEmail(data.selected))}
      activationChannel={accountActivationChannel(data.selected, data.jobs, actions.actionCatalog)}
      codexPhoneState={selectedPhoneState}
      updatingWebAccessToken={actions.updatingWebAccessTokens.has(accountCarrierID(data.selected))}
      onCopy={actions.toast.copyValue}
      onFetchInbox={actions.fetchInbox}
      onSessionSave={(account, sessionToken) => actions.updateAccount(account, { session_token: sessionToken }, '认证信息已更新')}
      onAccessSave={(account, accessToken) => actions.updateAccount(account, { access_token: accessToken }, '认证信息已更新')}
      runWorkflow={actions.runWorkflow}
      onWorkflowChanged={data.invalidate}
      onWorkflowMessage={actions.toast.showToast}
      onWorkflowError={actions.toast.showError}
      onUpdateWebAccessToken={actions.updateWebAccessToken}
      onDelete={actions.deleteAccount}
    />
  ) : null;

  return (
    <>
      <ToastMessage toast={actions.toast.toast} />
      <GptAccountsView
        accounts={data.accounts}
        jobs={data.jobs}
        selectedAccountId={selectedAccountID}
        actionCatalog={actions.actionCatalog}
        showSecrets={showSecrets}
        busy={busy}
        cleaningInvalidAccounts={actions.cleaningInvalidAccounts}
        runningAccountIds={data.runningIds}
        runningWorkflowByAccountID={data.runningByAccount}
        accountsPagination={data.accountsPagination}
        selectedAccount={data.selected}
        detail={selectedDetails}
        onCreateDone={async (message) => {
          actions.toast.showOK(message);
          await data.invalidate();
        }}
        onError={actions.toast.showError}
        onToggleSecrets={() => setShowSecrets((value) => !value)}
        onCleanInvalidAccounts={actions.cleanInvalidAccounts}
        onSelectAccount={(account) => setSelectedAccountID(accountCarrierID(account))}
        onCloseDetails={() => setSelectedAccountID('')}
        runWorkflow={actions.runWorkflow}
        runBulkWorkflow={actions.runBulkWorkflow}
        onDeleteAccount={actions.deleteAccount}
      />
    </>
  );
}
