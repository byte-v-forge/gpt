import { useEventQueryCache } from '@/dashboard/module-kit';
import { accountQueryKeys, updateAccountCache } from './account-data';
import type { Account } from './types';

export function useGptAccountEventCache() {
  useEventQueryCache<Account>({
    url: '/api/accounts/events',
    eventName: 'account',
    targets: [{
      queryKey: accountQueryKeys.accounts,
      update: (prev, account) => account?.account_id ? updateAccountCache(prev as Account[] | undefined, account) : prev
    }]
  });
}
