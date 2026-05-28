import { Bug, Copy, RefreshCw, Trash2 } from 'lucide-react';
import { ActionButtonGroup, Button, buttonHint, formatUnix, mask } from '@byte-v-forge/common-ui';
import type { ActionButtonDescriptor } from '@byte-v-forge/common-ui';
import { PaymentChannelIcon } from './account-badges';
import { accountInboxHint } from './account-mail-utils';
import { canGoPayPayment } from './account-utils';
import { GO_PAY_PAYMENT_CHANNELS, goPayPaymentActionLabel } from './gopay-utils';
import type { Account, AccountMailboxContext, ConcreteGoPayPaymentChannel, LatestOtp } from './types';

export function AccountDetailActions({ account, showSecrets, busy, inboxLoading, mailboxContext, latestOtp, canFetchOTP, onCopy, onFetchInbox, onGoPayPayment }: {
  account: Account;
  showSecrets: boolean;
  busy: boolean;
  inboxLoading: boolean;
  mailboxContext: AccountMailboxContext | null;
  latestOtp: LatestOtp | null;
  canFetchOTP: boolean;
  onCopy: (label: string, value: string) => void;
  onFetchInbox: (account: Account) => Promise<void>;
  onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void;
}) {
  const channelRow = channelActions(account, busy, onGoPayPayment);
  return (
    <div className="detailActionRows">
      {(canFetchOTP || latestOtp) && (
        <div className="detailActionRow">
          <span className="detailActionLabel">OTP</span>
          <div className="detailActionContent">
            <LatestOTPValue latestOtp={latestOtp} showSecrets={showSecrets} onCopy={onCopy} />
            {canFetchOTP && (
              <Button
                className="copyButton detailOtpRefresh"
                {...buttonHint(accountInboxHint(account.email, mailboxContext, showSecrets))}
                disabled={busy || inboxLoading}
                onClick={() => void onFetchInbox(account)}
              >
                <RefreshCw size={14} />
              </Button>
            )}
          </div>
        </div>
      )}
      {hasVisibleAction(channelRow) && <ActionRow label="激活" actions={channelRow} />}
    </div>
  );
}

export function AccountDangerActions({ account, busy, onDelete }: {
  account: Account;
  busy: boolean;
  onDelete: (account: Account) => Promise<void>;
}) {
  return (
    <div className="detailActionRows bottomActionRows">
      <ActionRow label="危险" actions={dangerActions(account, busy, onDelete)} />
    </div>
  );
}

function ActionRow({ label, actions }: { label?: string; actions: ActionButtonDescriptor[] }) {
  return (
    <div className={`detailActionRow${label ? '' : ' unlabeled'}`}>
      {label && <span className="detailActionLabel">{label}</span>}
      <ActionButtonGroup className="sectionActions" actions={actions} />
    </div>
  );
}

function LatestOTPValue({ latestOtp, showSecrets, onCopy }: {
  latestOtp: LatestOtp | null;
  showSecrets: boolean;
  onCopy: (label: string, value: string) => void;
}) {
  const code = latestOtp?.otp || '';
  return (
    <span className={`detailOtpCode${code ? '' : ' empty'}`}>
      <strong>{code ? (showSecrets ? code : mask(code)) : '暂无 OTP'}</strong>
      {code && <em>{formatUnix(latestOtp?.received_at_unix || 0)}</em>}
      <Button className="copyButton detailOtpCopy" {...buttonHint('复制 OTP')} disabled={!code} onClick={() => onCopy('OTP', code)}>
        <Copy size={14} />
      </Button>
    </span>
  );
}

function hasVisibleAction(actions: ActionButtonDescriptor[]) {
  return actions.some((action) => action.visible !== false);
}

function channelActions(account: Account, busy: boolean, onGoPayPayment: (account: Account, channel: ConcreteGoPayPaymentChannel) => void): ActionButtonDescriptor[] {
  if (!canGoPayPayment(account)) return [];
  return GO_PAY_PAYMENT_CHANNELS.map((channel) => ({
    id: `gopay-payment-${channel}`,
    label: goPayPaymentActionLabel(channel),
    hint: channel === 'wa' ? 'Debug：只走 GoPay WA 链接支付' : 'GoPay 激活支付渠道',
    icon: channel === 'wa' ? <Bug size={14} /> : <span className="activationPaymentIcon"><PaymentChannelIcon channel={channel} /></span>,
    className: channel === 'wa' ? 'debugAction' : 'activationAction',
    disabled: busy,
    onClick: () => onGoPayPayment(account, channel),
  }));
}

function dangerActions(account: Account, busy: boolean, onDelete: (account: Account) => Promise<void>): ActionButtonDescriptor[] {
  return [{
    id: 'delete-account',
    label: '删除账号',
    hint: '删除当前账号记录',
    icon: <Trash2 size={14} />,
    variant: 'destructive',
    disabled: busy,
    onClick: () => void onDelete(account),
  }];
}
