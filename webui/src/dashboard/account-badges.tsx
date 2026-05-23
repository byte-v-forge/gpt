import { MessageSquareText, Phone, PhoneOff } from 'lucide-react';
import { accountSignalText, accountSignalTone } from './account-utils';
import type { AccountCodexPhoneState } from './account-utils';
import { BrandIcon, GoPayWalletIcon } from './brand-icons';
import type { Account, ConcreteGoPayPaymentChannel } from './types';

export function PaymentChannelIcon({ channel }: { channel: ConcreteGoPayPaymentChannel }) {
  return (
    <span className="paymentChannelIcon" aria-hidden="true">
      <GoPayWalletIcon size={15} />
      {channel === 'sms' ? <MessageSquareText size={15} /> : <span className="whatsAppPaymentIcon"><BrandIcon brand="whatsapp" size={15} /></span>}
    </span>
  );
}

export function AccountSignalBadge({ account, compact }: { account: Account; compact?: boolean }) {
  const signal = accountSignalText(account);
  const tone = accountSignalTone(account);
  return (
    <span className={`accountSignalBadge accountStatePill ${tone}${compact ? ' compact' : ''}`} title={signal}>
      {signal}
    </span>
  );
}

export function AccountChannelTag({ channel }: { channel: string }) {
  if (!channel || channel === '-') return null;
  return <span className="accountMetaTag accountChannelTag" title={`渠道: ${channel}`}><em>渠道</em><strong>{channel}</strong></span>;
}

export function AccountCodexPhoneTag({ state }: { state: AccountCodexPhoneState }) {
  const Icon = state.confirmed ? Phone : PhoneOff;
  return (
    <span className={`accountMetaTag accountPhoneTag ${state.confirmed ? 'good' : 'neutral'}`} title={state.title}>
      <Icon size={13} />
      <strong>{state.label}</strong>
    </span>
  );
}
