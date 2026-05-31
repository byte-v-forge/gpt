import { cleanInvalidGptAccounts, deleteGptAccount, invalidAccountsForCleanup } from './account-cleanup-actions';
import type { GptAccountData } from './account-data';
import type { Account } from './types';
import { accountCarrierID, clearSelectedAccountID, useAsyncActionRunner } from '@byte-v-forge/common-ui';

type AccountToast = { showOK: (message: string) => void; showError: (error: unknown) => void };

export function useGptAccountCleanupActions(data: GptAccountData, toast: AccountToast) {
  const runner = useAsyncActionRunner();

  async function deleteAccount(account: Account) {
    if (await data.deleteAccount(account, deleteGptAccount, { onError: toast.showError })) toast.showOK('账号已删除');
  }

  async function cleanInvalidAccounts() {
    const loadedCount = invalidAccountsForCleanup(data.accounts).length;
    const hint = loadedCount ? `当前列表有 ${loadedCount} 个失效账号` : '将扫描并清理状态为 DEACTIVATED 的账号';
    if (!window.confirm(`${hint}，确认清理？`)) return;
    await runner.tryRun('clean-invalid-accounts', async () => {
      const deleted = await cleanInvalidGptAccounts();
      const deletedIds = new Set(deleted.map((account) => accountCarrierID(account)));
      clearSelectedAccountID(data.setSelectedAccountID, deletedIds);
      toast.showOK(deleted.length ? `已清理 ${deleted.length} 个失效账号` : '没有需要清理的失效账号');
      await data.invalidate();
    }, { onError: toast.showError });
  }

  return { cleaningInvalidAccounts: runner.busy, cleanInvalidAccounts, deleteAccount };
}
