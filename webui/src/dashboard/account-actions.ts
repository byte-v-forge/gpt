import { useEffect, useMemo } from 'react';
import { ACCOUNT_CREDENTIAL_KIND_MAILBOX, accountCarrierCredentialUpdatedAtUnix, accountMailboxContextForEmail, api, formatUnix, mask, mutateAccount, submitAccountWorkflowAction, useAsyncActionRunner, useQuery, useQueryClient, useToastMessage } from '@byte-v-forge/common-ui';
import { latestOtpForEmail, maskEmail, normalizeUiEmail } from '@byte-v-forge/common-ui';
import { type GptActionCatalog, type GptActionID, useGptActionCatalog } from './action-catalog';
import { isInvalidGptAccount } from './account-utils';
import { useGptAccountCleanupActions } from './account-cleanup-hook';
import { accountAuthQueryPrefix } from './account-auth-query';
import { accountInboxQueryKey, loadAccountMailboxProjection } from './account-inbox-query';
import type { GptAccountData } from './account-data';
import type { Account, FetchAccountMailboxResponse, InboxResponse } from './types';
import type { GetAccountResponse, UpdateAccountResponse } from '../proto/gpt_account';
import type { UpdateAccountAuthRequest } from '../proto/orchestrator_account';
import { accountCarrierID, accountCarrierEmail } from '@byte-v-forge/common-ui';

export function useGptAccountActions(data: GptAccountData, showSecrets: boolean, setSelectedAccountID: (value: string | ((prev: string) => string)) => void, providedActionCatalog?: GptActionCatalog) {
  const toast = useToastMessage();
  const queryClient = useQueryClient();
  const selectedAccountID = accountCarrierID(data.selected);
  const mailboxContext = data.selected ? accountMailboxContextForEmail(data.mailboxes, data.allocations, data.selected) : null;
  const primaryMailboxEmail = normalizeUiEmail(mailboxContext?.primary_email || '');
  const selectedInboxVersion = accountCarrierCredentialUpdatedAtUnix(data.selected, ACCOUNT_CREDENTIAL_KIND_MAILBOX);
  const selectedInboxKey = useMemo(() => accountInboxQueryKey(selectedAccountID, primaryMailboxEmail, selectedInboxVersion), [selectedAccountID, primaryMailboxEmail, selectedInboxVersion]);
  const inboxQuery = useQuery<InboxResponse | null>({
    queryKey: selectedInboxKey,
    queryFn: () => loadAccountMailboxProjection(selectedAccountID),
    enabled: !!data.selected && !!primaryMailboxEmail,
    refetchOnMount: 'always',
    initialData: null
  });
  const actionCatalogQuery = useGptActionCatalog();
  const actionCatalog = providedActionCatalog ?? actionCatalogQuery.data;
  const actionsRunner = useAsyncActionRunner();
  const inboxRunner = useAsyncActionRunner();
  const cleanup = useGptAccountCleanupActions(data, toast);

  useEffect(() => {
    if (data.loadError) toast.showError(data.loadError);
  }, [data.loadError, toast.showError]);

  async function runWorkflow(actionID: GptActionID, account: Account, payload: Record<string, unknown> = {}) {
    if (!canMutateAccount(account)) return;
    await actionsRunner.run(`workflow:${actionID}:${accountCarrierID(account)}`, () => submitAccountWorkflowAction({ catalog: actionCatalog, actionID, pathPrefix: '/api/gpt', payload: { account_id: accountCarrierID(account), ...payload }, toast, onSuccess: () => data.invalidate() }));
  }

  async function runBulkWorkflow(actionID: GptActionID, accounts: Account[], fallbackLabel = actionID) {
    if (!accounts.length) {
      toast.showError('没有可操作账号');
      return;
    }
    await actionsRunner.run(`bulk-workflow:${actionID}`, async () => {
      const accountIds = accounts.map((account) => accountCarrierID(account)).filter(Boolean);
      if (!accountIds.length) {
        toast.showError('没有可操作账号');
        return;
      }
      await submitAccountWorkflowAction({
        catalog: actionCatalog,
        actionID,
        fallbackLabel,
        pathPrefix: '/api/gpt',
        payload: { account_ids: accountIds },
        toast,
        onSuccess: () => data.invalidate(),
      });
    });
  }

  async function updateAccount(account: Account, payload: Partial<UpdateAccountAuthRequest>, successText: string) {
    if (!canMutateAccount(account)) return;
    await data.runAccountMutation('update-account', account, () => (
      mutateAccount<Account, UpdateAccountResponse>(`/api/gpt/accounts/${accountCarrierID(account)}`, { method: 'PATCH', body: JSON.stringify(payload) })
    ), {
      onSuccess: async (updated) => {
        if (!updated) return;
        setSelectedAccountID(accountCarrierID(updated));
        toast.showOK(successText);
        if (payload.session_token || payload.access_token) await queryClient.invalidateQueries({ queryKey: accountAuthQueryPrefix });
      },
      onError: toast.showError,
    });
  }

  async function updateWebAccessToken(account: Account) {
    if (!canMutateAccount(account)) return;
    await data.runAccountMutation('refresh-access-token', account, () => (
      mutateAccount<Account, GetAccountResponse>(`/api/gpt/accounts/${accountCarrierID(account)}/access-token`, { method: 'POST', body: '{}' })
    ), {
      onSuccess: () => toast.showOK('Web AT 已更新'),
      onError: toast.showError,
    });
  }

  async function fetchInbox(account: Account) {
    if (!canMutateAccount(account)) return;
    await inboxRunner.tryRun(`fetch-inbox:${accountCarrierID(account)}`, async () => {
      const resp = await api<FetchAccountMailboxResponse>(`/api/gpt/accounts/${accountCarrierID(account)}/mailbox/inbox`, { method: 'POST', body: JSON.stringify({ limit_per_mailbox: 10 }) });
      if (resp.account) {
        data.cacheAccount(resp.account);
        setSelectedAccountID(accountCarrierID(resp.account));
      }
      if (resp.inbox) queryClient.setQueryData(selectedInboxKey, resp.inbox);
      await queryClient.invalidateQueries({ queryKey: selectedInboxKey });
      const latest = resp.inbox ? latestOtpForEmail(resp.inbox, data.mailboxes, accountCarrierEmail(account)) : null;
      const mailbox = account.primary_mailbox_email || accountCarrierEmail(account);
      toast.showToast(resp.error_message ? 'error' : 'ok', `${showSecrets ? mailbox : maskEmail(mailbox)} 重新读取 OTP 缓存${latest ? `，OTP ${showSecrets ? latest.otp : mask(latest.otp)}，${formatUnix(latest.received_at_unix)}` : ''}${resp.error_message ? `，${resp.error_message}` : ''}`);
      if (resp.account) await data.invalidate();
    }, { onError: toast.showError });
  }

  function canMutateAccount(account: Account) {
    if (!isInvalidGptAccount(account)) return true;
    toast.showError('失效账号只能删除');
    return false;
  }

  const updatingWebAccessTokens = new Set(
    data.accounts
      .filter((account) => data.isAccountActionActive('refresh-access-token', account))
      .map((account) => accountCarrierID(account))
  );

  return { toast, actionCatalog, inbox: inboxQuery.data ?? null, inboxQueryKey: selectedInboxKey, working: actionsRunner.busy, inboxLoading: inboxRunner.busy, cleaningInvalidAccounts: cleanup.cleaningInvalidAccounts, updatingWebAccessTokens, runWorkflow, runBulkWorkflow, updateAccount, updateWebAccessToken, fetchInbox, cleanInvalidAccounts: cleanup.cleanInvalidAccounts, deleteAccount: cleanup.deleteAccount };
}
