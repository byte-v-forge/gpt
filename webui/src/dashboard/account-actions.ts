import { useEffect, useState } from 'react';
import { api, formatUnix, mask, useQuery, useQueryClient, useToastMessage } from '@/dashboard/module-kit';
import { latestOtpForEmail, maskEmail } from '@/dashboard/modules/mailbox/sdk';
import { goPayPaymentChannelLabel } from './gopay-utils';
import type { GptAccountData } from './account-data';
import type { GoPayUserWAPhoneResponse } from '@/proto/orchestrator_gopay_app';
import type { Account, ConcreteGoPayPaymentChannel, FetchAccountMailboxResponse, InboxResponse, SyncAccountMailboxesResponse } from './types';

const accountInboxQueryKey = ['gpt', 'inbox'] as const;
const GO_PAY_USER_ID = 'local';

export function useGptAccountActions(data: GptAccountData, showSecrets: boolean, setSelectedAccountID: (value: string | ((prev: string) => string)) => void) {
  const toast = useToastMessage();
  const queryClient = useQueryClient();
  const inboxQuery = useQuery<InboxResponse | null>({ queryKey: accountInboxQueryKey, queryFn: async () => null, enabled: false, initialData: null });
  const goPayProfile = useQuery({ queryKey: ['gpt', 'gopay', 'profile', GO_PAY_USER_ID], queryFn: loadGoPayProfile });
  const [working, setWorking] = useState(false);
  const [inboxLoading, setInboxLoading] = useState(false);
  const [syncingMailboxes, setSyncingMailboxes] = useState(false);
  const [refreshing, setRefreshing] = useState<Set<string>>(new Set());

  useEffect(() => { if (data.loadError) toast.showError(data.loadError); }, [data.loadError, toast.showError]);

  async function runWorkflow(label: string, path: string, account: Account, payload: Record<string, any> = {}) {
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

  async function submitJobOTP(jobId: string, otp: string) {
    const resp = await api<{ success?: boolean; error_message?: string }>(`/api/jobs/${jobId}/otp`, {
      method: 'POST',
      body: JSON.stringify({ otp })
    });
    toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || 'OTP 已提交');
    if (!resp.error_message) await data.invalidate();
  }

  async function resendJobOTP(jobId: string) {
    const resp = await api<{ success?: boolean; error_message?: string }>(`/api/jobs/${jobId}/otp/resend`, {
      method: 'POST',
      body: '{}'
    });
    toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || 'OTP 重发已触发');
    if (!resp.error_message) await data.invalidate();
  }

  async function runGoPayPayment(account: Account, otpChannel: ConcreteGoPayPaymentChannel) {
    setWorking(true);
    try {
      const resp = await api<{ job_id?: string; error_message?: string }>('/api/workflows/gopay-payment', {
        method: 'POST',
        body: JSON.stringify({ account_id: account.account_id, user_id: GO_PAY_USER_ID, otp_channel: otpChannel, ...goPayAppInput() })
      });
      toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || `${goPayPaymentChannelLabel(otpChannel)} 支付已提交: ${resp.job_id || 'ok'}`);
      if (!resp.error_message) await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setWorking(false);
    }
  }

  async function updateAccount(account: Account, payload: Record<string, string>, successText: string) {
    setWorking(true);
    try {
      const updated = await api<Account>(`/api/accounts/${account.account_id}`, { method: 'PATCH', body: JSON.stringify(payload) });
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
    setRefreshing((prev) => new Set(prev).add(account.account_id));
    try {
      const updated = await api<Account>(`/api/accounts/${account.account_id}/access-token`, { method: 'POST', body: '{}' });
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
    setInboxLoading(true);
    try {
      const resp = await api<FetchAccountMailboxResponse>(`/api/accounts/${account.account_id}/mailbox/inbox`, { method: 'POST', body: JSON.stringify({ limit_per_mailbox: 10 }) });
      if (resp.account) {
        data.cacheAccount(resp.account);
        setSelectedAccountID(resp.account.account_id);
      }
      if (resp.inbox) queryClient.setQueryData(accountInboxQueryKey, resp.inbox);
      const latest = resp.inbox ? latestOtpForEmail(resp.inbox, data.mailboxes, account.email) : null;
      const mailbox = account.primary_mailbox_email || account.email;
      toast.showToast(resp.error_message ? 'error' : 'ok', `${showSecrets ? mailbox : maskEmail(mailbox)} 收信完成：${resp.inbox?.message_count || 0} 封邮件${latest ? `，OTP ${showSecrets ? latest.otp : mask(latest.otp)}，${formatUnix(latest.received_at_unix)}` : ''}${resp.error_message ? `，${resp.error_message}` : ''}`);
      if (resp.account) await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setInboxLoading(false);
    }
  }

  async function syncMailboxes() {
    setSyncingMailboxes(true);
    try {
      const resp = await api<SyncAccountMailboxesResponse>('/api/accounts/mailbox/sync', {
        method: 'POST',
        body: JSON.stringify({ limit_per_mailbox: 25, account_limit: 500 })
      });
      toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || `邮箱同步完成：${resp.synced_count || 0}/${resp.account_count || 0} 个账号，${resp.message_count || 0} 封新邮件`);
      await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setSyncingMailboxes(false);
    }
  }

  async function deleteAccount(account: Account) {
    if (!window.confirm(`删除账号 ${account.email || account.account_id}？`)) return;
    await api(`/api/accounts/${account.account_id}`, { method: 'DELETE' });
    setSelectedAccountID((prev) => prev === account.account_id ? '' : prev);
    toast.showOK('账号已删除');
    await data.invalidate();
  }

  function goPayAppInput() {
    return { phone: goPayProfile.data?.wa_phone || '', country_code: '+62', pin: goPayProfile.data?.pin || '' };
  }

  function goPayActivationInput() {
    const input = goPayAppInput();
    return { gopay_phone: input.phone, gopay_country_code: input.country_code, gopay_pin: input.pin };
  }

  async function runRegisterActivate(account: Account) {
    await runWorkflow('注册并激活', '/api/workflows/register-and-activate', account, goPayActivationInput());
  }

  return { toast, inbox: inboxQuery.data ?? null, inboxQueryKey: accountInboxQueryKey, working, inboxLoading, syncingMailboxes, refreshing, runWorkflow, runGoPayPayment, runRegisterActivate, submitJobOTP, resendJobOTP, updateAccount, refreshAccessToken, fetchInbox, syncMailboxes, deleteAccount };
}

function loadGoPayProfile() {
  return api<GoPayUserWAPhoneResponse>(`/api/gopay/profile?user_id=${GO_PAY_USER_ID}`);
}
