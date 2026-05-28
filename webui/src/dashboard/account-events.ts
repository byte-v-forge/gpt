import { createHotStreamURL, useHotStreamInvalidation } from '@byte-v-forge/common-ui';
import { accountQueryKeys } from './account-data';
import { accountInboxQueryPrefix } from './account-inbox-query';

const accountEventTypes = ['gpt.account.updated', 'gpt.account.deleted', 'gpt.email_allocation.updated', 'gpt.account_mailbox.updated'];
const accountResourceTypes = ['gpt.account', 'gpt.email_allocation', 'gpt.account_mailbox'];

export function useGptAccountEventCache() {
  useHotStreamInvalidation({
    url: createHotStreamURL('/api/gpt', { eventTypes: accountEventTypes, resourceTypes: accountResourceTypes }),
    rules: [
      { queryKey: accountQueryKeys.accounts, eventTypes: ['gpt.account.updated', 'gpt.account.deleted', 'gpt.account_mailbox.updated'], resourceTypes: ['gpt.account', 'gpt.account_mailbox'] },
      { queryKey: accountInboxQueryPrefix, eventTypes: ['gpt.account_mailbox.updated'], resourceTypes: ['gpt.account_mailbox'] },
      { queryKey: accountQueryKeys.allocations, eventTypes: ['gpt.email_allocation.updated'], resourceTypes: ['gpt.email_allocation'] }
    ]
  });
}
