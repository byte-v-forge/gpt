import { useState } from 'react';
import { DetailDrawer, latestOtpForEmail, ToastMessage, WorkspaceTabbedPanel } from '@byte-v-forge/common-ui';
import { AccountDetails } from './account-details';
import { useGptAccountActions } from './account-actions';
import { useGptAccountData } from './account-data';
import { useGptAccountEventCache } from './account-events';
import { mailboxContextForEmail } from './account-mail-utils';
import { accountActivationChannel, accountCodexPhoneState, loginActionLabel } from './account-utils';
import { GoPayLabPage } from './gopay-page';
import { GPTSettingsPage } from './gpt-settings-page';
import { GptAccountsView } from './view';

type GptPageTab = 'accounts' | 'gopay' | 'settings';

export function GptAccountsPage() {
  const [tab, setTab] = useState<GptPageTab>('accounts');
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
          content: <GptAccountsTab />,
          contentClassName: 'flex flex-col overflow-hidden'
        },
        {
          value: 'gopay',
          label: 'GoPay',
          content: <GoPayLabPage />,
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

function GptAccountsTab() {
  const [selectedAccountID, setSelectedAccountID] = useState('');
  const [showSecrets, setShowSecrets] = useState(true);
  const data = useGptAccountData(selectedAccountID);
  const actions = useGptAccountActions(data, showSecrets, setSelectedAccountID);
  const busy = actions.working || data.busy;
  const selectedPhoneState = data.selected ? accountCodexPhoneState(data.selected, data.jobs) : null;
  useGptAccountEventCache();

  return (
    <>
      <ToastMessage toast={actions.toast.toast} />
      <GptAccountsView accounts={data.accounts} jobs={data.jobs} mailboxes={data.mailboxes} allocations={data.allocations} mailboxDomains={data.domains} mailboxProviderCapabilities={data.providerCapabilities} selectedAccountId={selectedAccountID} showSecrets={showSecrets} busy={busy} cleaningInvalidAccounts={actions.cleaningInvalidAccounts} runningAccountIds={data.runningIds} runningWorkflowByAccountID={data.runningByAccount} onCreateDone={async (message) => { actions.toast.showOK(message); await data.invalidate(); }} onError={actions.toast.showError} onToggleSecrets={() => setShowSecrets((value) => !value)} onCleanInvalidAccounts={actions.cleanInvalidAccounts} onSelectAccount={(account) => setSelectedAccountID(account.account_id)} onRegisterProtocol={(account) => actions.runWorkflow('协议注册', '/api/gpt/workflows/register-protocol', account)} onCodexOAuthBatchAddPhone={actions.runCodexOAuthBatchAddPhone} onGoPayPayment={(account, channel) => void actions.runGoPayPayment(account, channel)} onDeleteAccount={actions.deleteAccount} />
      <DetailDrawer open={!!data.selected} title="GPT账号详情" size="wide" onClose={() => setSelectedAccountID('')}>
        {data.selected && <AccountDetails account={data.selected} showSecrets={showSecrets} busy={busy} inboxLoading={actions.inboxLoading} mailboxContext={mailboxContextForEmail(data.mailboxes, data.allocations, data.selected)} mailboxProviderCapabilities={data.providerCapabilities} latestOtp={latestOtpForEmail(actions.inbox, data.mailboxes, data.selected.email)} activationChannel={accountActivationChannel(data.selected, data.jobs)} codexPhoneState={selectedPhoneState!} refreshingAccessToken={actions.refreshing.has(data.selected.account_id)} onCopy={actions.toast.copyValue} onFetchInbox={actions.fetchInbox} onSessionSave={(account, sessionToken) => actions.updateAccount(account, { session_token: sessionToken }, '认证信息已更新')} onAccessSave={(account, accessToken) => actions.updateAccount(account, { access_token: accessToken }, '认证信息已更新')} onProbeAccount={(account) => actions.runWorkflow('探测账号', '/api/gpt/workflows/probe', account)} onRegister={(account) => actions.runWorkflow('浏览器注册', '/api/gpt/workflows/register', account)} onRegisterProtocol={(account) => actions.runWorkflow('协议注册', '/api/gpt/workflows/register-protocol', account)} onLogin={(account) => actions.runWorkflow(loginActionLabel(account), '/api/gpt/workflows/login', account)} onLoginProtocol={(account) => actions.runWorkflow('协议' + loginActionLabel(account), '/api/gpt/workflows/login-protocol', account)} onCodexOAuthAddPhone={(account) => actions.runWorkflow('生成 auth.json', '/api/gpt/workflows/codex-oauth', account)} onCodexOAuthProtocol={(account) => actions.runWorkflow('协议 auth.json', '/api/gpt/workflows/codex-oauth-protocol', account)} onGoPayPayment={(account, channel) => void actions.runGoPayPayment(account, channel)} onRefreshAccessToken={actions.refreshAccessToken} onDelete={actions.deleteAccount} />}
      </DetailDrawer>
    </>
  );
}
