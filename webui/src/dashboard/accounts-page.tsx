import { useState } from 'react';
import { DetailDrawer, ToastMessage } from '@/dashboard/module-kit';
import { latestOtpForEmail } from '@/dashboard/modules/mailbox/sdk';
import { AccountDetails } from './account-details';
import { useGptAccountActions } from './account-actions';
import { useGptAccountData } from './account-data';
import { useGptAccountEventCache } from './account-events';
import { latestOtpFromAccount, mailboxContextForEmail } from './account-mail-utils';
import { accountActivationChannel, loginActionLabel } from './account-utils';
import { GptAccountsView } from './view';

export function GptAccountsPage() {
  const [selectedAccountID, setSelectedAccountID] = useState('');
  const [showSecrets, setShowSecrets] = useState(true);
  const data = useGptAccountData(selectedAccountID);
  const actions = useGptAccountActions(data, showSecrets, setSelectedAccountID);
  const busy = actions.working || data.busy;
  useGptAccountEventCache();

  return (
    <>
      <ToastMessage toast={actions.toast.toast} />
      <GptAccountsView accounts={data.accounts} jobs={data.jobs} mailboxDomains={data.domains} selectedAccountId={selectedAccountID} showSecrets={showSecrets} busy={busy} mailboxSyncing={actions.syncingMailboxes} runningAccountIds={data.runningIds} runningWorkflowByAccountID={data.runningByAccount} refreshingAccessTokenIds={actions.refreshing} onCreateDone={async (message) => { actions.toast.showOK(message); await data.invalidate(); }} onError={actions.toast.showError} onToggleSecrets={() => setShowSecrets((value) => !value)} onSyncMailboxes={actions.syncMailboxes} onSelectAccount={(account) => setSelectedAccountID(account.account_id)} onOpenWorkflow={() => actions.toast.showOK('请在工作流页查看任务详情')} onRegister={(account) => actions.runWorkflow('注册账号', '/api/workflows/register', account)} onLogin={(account) => actions.runWorkflow(loginActionLabel(account), '/api/workflows/login', account)} onGoPayPayment={(account, channel) => void actions.runGoPayPayment(account, channel)} onProbeAccount={(account) => actions.runWorkflow('探测账号', '/api/workflows/probe', account)} onRegisterActivate={actions.runRegisterActivate} onRefreshAccessToken={actions.refreshAccessToken} onDeleteAccount={actions.deleteAccount} onSubmitOTP={actions.submitJobOTP} onResendOTP={actions.resendJobOTP} onCopy={actions.toast.copyValue} />
      <DetailDrawer open={!!data.selected} title="GPT账号详情" onClose={() => setSelectedAccountID('')}>
        {data.selected && <AccountDetails account={data.selected} showSecrets={showSecrets} busy={busy} inboxLoading={actions.inboxLoading} mailboxContext={mailboxContextForEmail(data.mailboxes, data.allocations, data.selected)} latestOtp={latestOtpForEmail(actions.inbox, data.mailboxes, data.selected.email) || latestOtpFromAccount(data.selected)} activationChannel={accountActivationChannel(data.selected, data.jobs)} refreshingAccessToken={actions.refreshing.has(data.selected.account_id)} onCopy={actions.toast.copyValue} onFetchInbox={actions.fetchInbox} onSessionSave={(account, sessionToken) => actions.updateAccount(account, { session_token: sessionToken }, '认证信息已更新')} onAccessSave={(account, accessToken) => actions.updateAccount(account, { access_token: accessToken }, '认证信息已更新')} onActivationChannelSave={(account, activationChannel) => actions.updateAccount(account, { activation_channel: activationChannel }, '渠道已更新')} onProbeAccount={(account) => actions.runWorkflow('探测账号', '/api/workflows/probe', account)} onLogin={(account) => actions.runWorkflow(loginActionLabel(account), '/api/workflows/login', account)} onRefreshAccessToken={actions.refreshAccessToken} />}
      </DetailDrawer>
    </>
  );
}
