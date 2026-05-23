import { useState } from 'react';
import { DetailDrawer, ToastMessage, api } from '@/dashboard/module-kit';
import { latestOtpForEmail } from '@/dashboard/modules/mailbox/sdk';
import { AccountDetails } from './account-details';
import { useGptAccountActions } from './account-actions';
import { useGptAccountData } from './account-data';
import { useGptAccountEventCache } from './account-events';
import { latestOtpFromAccount, mailboxContextForEmail } from './account-mail-utils';
import { accountActivationChannel, accountCodexPhoneState, loginActionLabel } from './account-utils';
import { addBalanceMethodLabel, goPayAddBalancePayload } from './gopay-utils';
import { GptAccountsView } from './view';
import type { ConcreteGoPayAddBalanceMethod } from './types';

export function GptAccountsPage() {
  const [selectedAccountID, setSelectedAccountID] = useState('');
  const [showSecrets, setShowSecrets] = useState(true);
  const data = useGptAccountData(selectedAccountID);
  const actions = useGptAccountActions(data, showSecrets, setSelectedAccountID);
  const busy = actions.working || data.busy;
  const selectedPhoneState = data.selected ? accountCodexPhoneState(data.selected, data.jobs) : null;
  useGptAccountEventCache();

  async function confirmManualPayment(jobId: string) {
    const resp = await api<{ success?: boolean; error_message?: string }>(`/api/jobs/${jobId}/gopay-payment/confirm`, { method: 'POST', body: '{}' });
    actions.toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || '已确认支付，继续后续步骤');
    if (!resp.error_message) await data.invalidate();
  }

  async function selectAddBalance(jobId: string, method: ConcreteGoPayAddBalanceMethod) {
    const resp = await api<{ success?: boolean; error_message?: string }>(`/api/jobs/${jobId}/add-balance/select`, { method: 'POST', body: JSON.stringify({ addBalance: goPayAddBalancePayload(method) }) });
    actions.toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || `已选择加余额方式：${addBalanceMethodLabel(method)}`);
    if (!resp.error_message) await data.invalidate();
  }

  async function confirmAddBalance(jobId: string) {
    const resp = await api<{ success?: boolean; error_message?: string }>(`/api/jobs/${jobId}/add-balance/confirm`, { method: 'POST', body: '{}' });
    actions.toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || '已确认加余额，继续后续步骤');
    if (!resp.error_message) await data.invalidate();
  }

  async function cancelWorkflow(jobId: string) {
    const resp = await api<{ success?: boolean; error_message?: string }>(`/api/jobs/${jobId}/cancel`, { method: 'POST', body: JSON.stringify({ reason: 'manual workflow cancel' }) });
    actions.toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || '流程已取消');
    if (!resp.error_message) await data.invalidate();
  }

  return (
    <>
      <ToastMessage toast={actions.toast.toast} />
      <GptAccountsView accounts={data.accounts} jobs={data.jobs} mailboxDomains={data.domains} selectedAccountId={selectedAccountID} showSecrets={showSecrets} busy={busy} mailboxSyncing={actions.syncingMailboxes} cleaningInvalidAccounts={actions.cleaningInvalidAccounts} runningAccountIds={data.runningIds} runningWorkflowByAccountID={data.runningByAccount} refreshingAccessTokenIds={actions.refreshing} onCreateDone={async (message) => { actions.toast.showOK(message); await data.invalidate(); }} onError={actions.toast.showError} onToggleSecrets={() => setShowSecrets((value) => !value)} onSyncMailboxes={actions.syncMailboxes} onCleanInvalidAccounts={actions.cleanInvalidAccounts} onSelectAccount={(account) => setSelectedAccountID(account.account_id)} onOpenWorkflow={() => actions.toast.showOK('请在工作流页查看任务详情')} onCancelWorkflow={cancelWorkflow} onRegister={(account) => actions.runWorkflow('注册账号', '/api/workflows/register', account)} onCodexOAuthAddPhone={(account) => actions.runWorkflow('生成 auth.json', '/api/workflows/codex-oauth', account)} onCodexOAuthBatchAddPhone={actions.runCodexOAuthBatchAddPhone} onGoPayPayment={(account, channel) => void actions.runGoPayPayment(account, channel)} onRefreshAccessToken={actions.refreshAccessToken} onSubmitOTP={actions.submitJobOTP} onResendOTP={actions.resendJobOTP} onConfirmManualPayment={confirmManualPayment} onSelectAddBalance={selectAddBalance} onConfirmAddBalance={confirmAddBalance} />
      <DetailDrawer open={!!data.selected} title="GPT账号详情" onClose={() => setSelectedAccountID('')}>
        {data.selected && <AccountDetails account={data.selected} showSecrets={showSecrets} busy={busy} inboxLoading={actions.inboxLoading} mailboxContext={mailboxContextForEmail(data.mailboxes, data.allocations, data.selected)} latestOtp={latestOtpForEmail(actions.inbox, data.mailboxes, data.selected.email) || latestOtpFromAccount(data.selected)} activationChannel={accountActivationChannel(data.selected, data.jobs)} codexPhoneState={selectedPhoneState!} refreshingAccessToken={actions.refreshing.has(data.selected.account_id)} onCopy={actions.toast.copyValue} onFetchInbox={actions.fetchInbox} onSessionSave={(account, sessionToken) => actions.updateAccount(account, { session_token: sessionToken }, '认证信息已更新')} onAccessSave={(account, accessToken) => actions.updateAccount(account, { access_token: accessToken }, '认证信息已更新')} onProbeAccount={(account) => actions.runWorkflow('探测账号', '/api/workflows/probe', account)} onLogin={(account) => actions.runWorkflow(loginActionLabel(account), '/api/workflows/login', account)} onCodexOAuthAddPhone={(account) => actions.runWorkflow('生成 auth.json', '/api/workflows/codex-oauth', account)} onGoPayPayment={(account, channel) => void actions.runGoPayPayment(account, channel)} onRefreshAccessToken={actions.refreshAccessToken} onDelete={actions.deleteAccount} />}
      </DetailDrawer>
    </>
  );
}
