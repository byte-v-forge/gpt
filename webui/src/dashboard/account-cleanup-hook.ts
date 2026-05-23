import { useState } from 'react';
import { cleanInvalidGptAccounts, deleteGptAccount, invalidAccountsForCleanup } from './account-cleanup-actions';
import type { GptAccountData } from './account-data';
import type { Account } from './types';

type AccountToast = { showOK: (message: string) => void; showError: (error: unknown) => void };

export function useGptAccountCleanupActions(data: GptAccountData, setSelectedAccountID: (value: string | ((prev: string) => string)) => void, toast: AccountToast) {
  const [cleaningInvalidAccounts, setCleaningInvalidAccounts] = useState(false);

  async function deleteAccount(account: Account) {
    if (!window.confirm(`删除账号 ${account.email || account.account_id}？`)) return;
    await deleteGptAccount(account.account_id);
    setSelectedAccountID((prev) => prev === account.account_id ? '' : prev);
    toast.showOK('账号已删除');
    await data.invalidate();
  }

  async function cleanInvalidAccounts() {
    const loadedCount = invalidAccountsForCleanup(data.accounts).length;
    const hint = loadedCount ? `当前列表有 ${loadedCount} 个失效账号` : '将扫描并清理状态为 DEACTIVATED 的账号';
    if (!window.confirm(`${hint}，确认清理？`)) return;
    setCleaningInvalidAccounts(true);
    try {
      const deleted = await cleanInvalidGptAccounts();
      const deletedIds = new Set(deleted.map((account) => account.account_id));
      setSelectedAccountID((prev) => deletedIds.has(prev) ? '' : prev);
      toast.showOK(deleted.length ? `已清理 ${deleted.length} 个失效账号` : '没有需要清理的失效账号');
      await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setCleaningInvalidAccounts(false);
    }
  }

  return { cleaningInvalidAccounts, cleanInvalidAccounts, deleteAccount };
}
