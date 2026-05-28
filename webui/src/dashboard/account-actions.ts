import { useEffect, useMemo, useState } from 'react';
import { api, formatUnix, mask, useQuery, useQueryClient, useToastMessage } from '@byte-v-forge/common-ui';
import { latestOtpForEmail, maskEmail, normalizeUiEmail } from '@byte-v-forge/common-ui';
import { goPayPaymentActionLabel, goPayPaymentRequestChannel, isPureGoPayWAPaymentChannel } from './gopay-utils';
import { mailboxContextForEmail } from './account-mail-utils';
import { isInvalidGptAccount } from './account-utils';
import { useGptAccountCleanupActions } from './account-cleanup-hook';
import { accountInboxQueryKey, loadAccountMailboxProjection } from './account-inbox-query';
import type { GptAccountData } from './account-data';
import type { GoPayUserWAPhoneResponse } from '../proto/orchestrator_gopay_app';
import type { Account, ConcreteGoPayPaymentChannel, FetchAccountMailboxResponse, InboxResponse } from './types';

const GO_PAY_USER_ID = 'local';

export function useGptAccountActions(data: GptAccountData, showSecrets: boolean, setSelectedAccountID: (value: string | ((prev: string) => string)) => void) {
  const toast = useToastMessage();
  const queryClient = useQueryClient();
  const mailboxContext = data.selected ? mailboxContextForEmail(data.mailboxes, data.allocations, data.selected) : null;
  const primaryMailboxEmail = normalizeUiEmail(mailboxContext?.primary_email || '');
  const selectedInboxVersion = data.selected?.mailbox_last_message_at_unix || 0;
  const selectedInboxKey = useMemo(() => accountInboxQueryKey(data.selected?.account_id || '', primaryMailboxEmail, selectedInboxVersion), [data.selected?.account_id, primaryMailboxEmail, selectedInboxVersion]);
  const inboxQuery = useQuery<InboxResponse | null>({
    queryKey: selectedInboxKey,
    queryFn: () => loadAccountMailboxProjection(data.selected?.account_id || ''),
    enabled: !!data.selected && !!primaryMailboxEmail,
    refetchOnMount: 'always',
    initialData: null
  });
  const goPayProfile = useQuery({ queryKey: ['gpt', 'gopay', 'profile', GO_PAY_USER_ID], queryFn: loadGoPayProfile });
  const [working, setWorking] = useState(false);
  const [inboxLoading, setInboxLoading] = useState(false);
  const [refreshing, setRefreshing] = useState<Set<string>>(new Set());
  const cleanup = useGptAccountCleanupActions(data, setSelectedAccountID, toast);

  useEffect(() => { if (data.loadError) toast.showError(data.loadError); }, [data.loadError, toast.showError]);

  async function runWorkflow(label: string, path: string, account: Account, payload: Record<string, any> = {}) {
    if (!canMutateAccount(account)) return;
    setWorking(true);
    try {
      const resp = await api<{ job_id?: string; error_message?: string }>(path, { method: 'POST', body: JSON.stringify({ account_id: account.account_id, ...payload }) });
      toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || `${label} 已提交: ${resp.job_id || 'ok'}`);
      if (!resp.error_message) await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setWorking(false);
    }
  }

  async function runGoPayPayment(account: Account, otpChannel: ConcreteGoPayPaymentChannel) {
    if (!canMutateAccount(account)) return;
    setWorking(true);
    try {
      const input = goPayAppInput();
      const waOnly = isPureGoPayWAPaymentChannel(otpChannel);
      const apiChannel = goPayPaymentRequestChannel(otpChannel);
      const appInput = { user_id: GO_PAY_USER_ID, wa_phone: input.phone, country_code: input.country_code, pin: input.pin };
      const body = waOnly ? { account_id: account.account_id, ...appInput } : { account_id: account.account_id, otp_channel: apiChannel, ...appInput };
      const resp = await api<{ job_id?: string; error_message?: string }>(waOnly ? '/api/gpt/workflows/gopay-wa-payment' : '/api/gpt/workflows/gopay-payment', { method: 'POST', body: JSON.stringify(body) });
      toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || `${goPayPaymentActionLabel(otpChannel)} 已提交: ${resp.job_id || 'ok'}`);
      if (!resp.error_message) await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setWorking(false);
    }
  }

  async function runCodexOAuthBatchAddPhone(accounts: Account[]) {
    if (!accounts.length) {
      toast.showError('没有未加手机账号');
      return;
    }
    setWorking(true);
    try {
      const accountIds = accounts.map((account) => account.account_id).filter(Boolean);
      const resp = await api<{ job_id?: string; error_message?: string }>('/api/gpt/workflows/codex-oauth-add-phone/batch', {
        method: 'POST',
        body: JSON.stringify({ account_ids: accountIds })
      });
      toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || `add phone 已提交: ${resp.job_id || 'ok'}`);
      if (!resp.error_message) await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setWorking(false);
    }
  }
  async function updateAccount(account: Account, payload: Record<string, string>, successText: string) {
    if (!canMutateAccount(account)) return;
    setWorking(true);
    try {
      const updated = await api<Account>(`/api/gpt/accounts/${account.account_id}`, { method: 'PATCH', body: JSON.stringify(payload) });
      data.cacheAccount(updated);
      setSelectedAccountID(updated.account_id);
      toast.showOK(successText);
      await data.invalidate();
    } catch (err) {
      toast.showError(err);
      throw err;
    } finally {
      setWorking(false);
    }
  }

  async function refreshAccessToken(account: Account) {
    if (!canMutateAccount(account)) return;
    setRefreshing((prev) => new Set(prev).add(account.account_id));
    try {
      const updated = await api<Account>(`/api/gpt/accounts/${account.account_id}/access-token`, { method: 'POST', body: '{}' });
      data.cacheAccount(updated);
      toast.showOK('Access Token 已自动获取');
      await data.invalidate();
    } finally {
      setRefreshing((prev) => {
        const next = new Set(prev);
        next.delete(account.account_id);
        return next;
      });
    }
  }

  async function fetchInbox(account: Account) {
    if (!canMutateAccount(account)) return;
    setInboxLoading(true);
    try {
      const resp = await api<FetchAccountMailboxResponse>(`/api/gpt/accounts/${account.account_id}/mailbox/inbox`, { method: 'POST', body: JSON.stringify({ limit_per_mailbox: 10 }) });
      if (resp.account) {
        data.cacheAccount(resp.account);
        setSelectedAccountID(resp.account.account_id);
      }
      if (resp.inbox) queryClient.setQueryData(selectedInboxKey, resp.inbox);
      await queryClient.invalidateQueries({ queryKey: selectedInboxKey });
      const latest = resp.inbox ? latestOtpForEmail(resp.inbox, data.mailboxes, account.email) : null;
      const mailbox = account.primary_mailbox_email || account.email;
      toast.showToast(resp.error_message ? 'error' : 'ok', `${showSecrets ? mailbox : maskEmail(mailbox)} 读取 GPT 邮件投影${latest ? `，OTP ${showSecrets ? latest.otp : mask(latest.otp)}，${formatUnix(latest.received_at_unix)}` : ''}${resp.error_message ? `，${resp.error_message}` : ''}`);
      if (resp.account) await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setInboxLoading(false);
    }
  }


  function goPayAppInput() { return { phone: goPayProfile.data?.wa_phone || '', country_code: '+62', pin: goPayProfile.data?.pin || '' }; }

  function canMutateAccount(account: Account) { if (!isInvalidGptAccount(account)) return true; toast.showError('失效账号只能删除'); return false; }

  return { toast, inbox: inboxQuery.data ?? null, inboxQueryKey: selectedInboxKey, working, inboxLoading, cleaningInvalidAccounts: cleanup.cleaningInvalidAccounts, refreshing, runWorkflow, runCodexOAuthBatchAddPhone, runGoPayPayment, updateAccount, refreshAccessToken, fetchInbox, cleanInvalidAccounts: cleanup.cleanInvalidAccounts, deleteAccount: cleanup.deleteAccount };
}

function loadGoPayProfile() {
  return api<GoPayUserWAPhoneResponse>(`/api/gpt/gopay/profile?user_id=${GO_PAY_USER_ID}`);
}
