import { accountCarrierEmail, maskAccountRecordEmail, requireAccountRecord } from '@byte-v-forge/common-ui';
import type { AccountRecord as CommonAccountRecord } from '@byte-v-forge/common-ui';
import type { Account } from './types';

export function gptAccountRecord(account: Account, showSecrets: boolean): CommonAccountRecord {
  const record = requireAccountRecord(account, 'GPT account projection is required');
  return showSecrets ? record : maskAccountRecordEmail(record, accountCarrierEmail(account));
}
