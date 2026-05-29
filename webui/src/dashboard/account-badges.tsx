import { MessageSquareText, Phone, PhoneOff, QrCode } from 'lucide-react';
import { accountSignalText, accountSignalTone } from './account-utils';
import type { AccountCodexPhoneState } from './account-job-semantics';
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
  return <span className="accountMetaTag accountChannelTag iconOnly" title={`渠道: ${channel}`} aria-label={`渠道: ${channel}`}>{channelIcon(channel)}</span>;
}

export function AccountCodexPhoneTag({ state }: { state: AccountCodexPhoneState }) {
  const Icon = state.confirmed || state.tone === 'bad' ? Phone : PhoneOff;
  return (
    <span className={`accountMetaTag accountPhoneTag iconOnly ${state.tone}`} title={state.title} aria-label={state.label}>
      <Icon size={13} />
    </span>
  );
}

function channelIcon(channel: string) {
  const normalized = channel.toLowerCase();
  if (normalized.includes('qris')) return <QrCode size={14} />;
  if (normalized.includes('sms')) return <PaymentChannelIcon channel="sms" />;
  if (normalized.includes('app') && normalized.includes('wa')) return <PaymentChannelIcon channel="app_wa" />;
  if (normalized.includes('wa') || normalized.includes('whatsapp')) return <PaymentChannelIcon channel="wa" />;
  return <GoPayWalletIcon size={15} />;
}
