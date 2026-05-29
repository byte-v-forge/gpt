import { numberValue, objectValue, stringValue } from '@byte-v-forge/common-ui';
import { stepDetailData } from './job-utils';
import type { ConcreteGoPayAddBalanceMethod, ConcreteGoPayPaymentChannel, Job, WorkflowProgress } from './types';

type PaymentChannelDescriptor = {
  value: ConcreteGoPayPaymentChannel;
  canonical: '' | 'gopay_sms' | 'gopay_wa';
  requestChannel: 'sms' | 'wa';
  label: string;
  actionLabel?: string;
  aliases: string[];
  match?: (value: string) => boolean;
  pureWA?: boolean;
};

const PAYMENT_CHANNELS: PaymentChannelDescriptor[] = [{
  value: 'sms',
  canonical: 'gopay_sms',
  requestChannel: 'sms',
  label: 'Gopay-SMS',
  aliases: ['sms', 'gopay_sms', 'gopay-sms'],
  match: (value) => value.includes('sms') && value.includes('gopay')
}, {
  value: 'app_wa',
  canonical: '',
  requestChannel: 'wa',
  label: 'Gopay App-WA',
  aliases: ['app_wa', 'gopay_app_wa', 'gopay-app-wa']
}, {
  value: 'wa',
  canonical: 'gopay_wa',
  requestChannel: 'wa',
  label: 'Gopay-WA',
  actionLabel: '纯WA支付',
  aliases: ['wa', 'whatsapp', 'gopay_wa', 'gopay-wa'],
  match: (value) => (value.includes('wa') || value.includes('whatsapp')) && value.includes('gopay'),
  pureWA: true
}];

type AddBalanceDescriptor = {
  value: ConcreteGoPayAddBalanceMethod;
  label: string;
  aliases: string[];
  payload: Record<string, Record<string, never>>;
  match?: (value: string) => boolean;
};

const ADD_BALANCE_METHODS: AddBalanceDescriptor[] = [{
  value: 'manual_transfer',
  label: '手动转账',
  aliases: ['manual_transfer', 'manual-transfer'],
  payload: { manualTransfer: {} },
  match: (value) => value.includes('manual_transfer') || value.includes('手动转账')
}, {
  value: 'envelope',
  label: '红包',
  aliases: ['envelope', 'claim_envelope'],
  payload: { envelope: {} },
  match: (value) => value.includes('envelope') || value.includes('红包')
}];

export const GO_PAY_PAYMENT_CHANNELS: ConcreteGoPayPaymentChannel[] = PAYMENT_CHANNELS.map((item) => item.value);

export function paymentChannelValue(value: string): '' | 'gopay_sms' | 'gopay_wa' {
  const normalized = String(value || '').trim().toLowerCase();
  if (!normalized) return '';
  return paymentChannelDescriptor(normalized)?.canonical || '';
}

export function goPayPaymentChannelLabel(value: string) {
  const normalized = String(value || '').trim().toLowerCase();
  return paymentChannelDescriptor(normalized)?.label || '-';
}

export function goPayPaymentActionLabel(channel: ConcreteGoPayPaymentChannel) {
  const descriptor = paymentChannelDescriptor(channel);
  return descriptor?.actionLabel || descriptor?.label || '-';
}

export function goPayPaymentRequestChannel(channel: ConcreteGoPayPaymentChannel): 'sms' | 'wa' {
  return paymentChannelDescriptor(channel)?.requestChannel || 'sms';
}

export function isPureGoPayWAPaymentChannel(channel: ConcreteGoPayPaymentChannel) {
  return paymentChannelDescriptor(channel)?.pureWA === true;
}

export function goPayAddBalancePayload(method: ConcreteGoPayAddBalanceMethod) {
  return addBalanceDescriptor(method)?.payload || { manualTransfer: {} };
}

export function addBalanceMethodValue(value: string) {
  return addBalanceDescriptor(value)?.value || '';
}

export function addBalanceMethodLabel(value: string) {
  return addBalanceDescriptor(value)?.label || '';
}

export function goPayPaymentUserId(job: Job) {
  return stringValue(objectValue(job.result).user_id) || 'local';
}

export function manualAddBalanceView(job: Job) {
  const data = stepDetailData((job.steps || []).find((item) => item.step_name === 'gopay_app_ensure_balance'));
  if (!data) return null;
  const transfer = objectValue(data.manual_transfer);
  return {
    method: stringValue(data.method),
    status: stringValue(data.status),
    transfer: {
      qr_payload: stringValue(transfer.qr_payload),
      instructions: stringValue(transfer.instructions),
      amount: numberValue(transfer.amount),
      currency: stringValue(transfer.currency) || 'IDR'
    }
  };
}

export function manualGoPayPaymentView(job: Job) {
  const data = stepDetailData((job.steps || []).find((item) => item.step_name === 'gopay_payment'));
  if (!data) return null;
  const confirmation = objectValue(data.manual_payment_confirmation);
  const complete = objectValue(data.payment_complete);
  const required = confirmation.required === true || complete.awaiting_manual_confirmation === true;
  if (!required) return null;
  const qrValue = stringValue(complete.qr_string) || stringValue(data.qr_string) || stringValue(complete.qr_code_url);
  const qrCodeUrl = stringValue(complete.qr_code_url);
  return {
    required,
    confirmed: confirmation.confirmed === true,
    charge_ref: stringValue(complete.charge_ref),
    qr_payload: isHttpURL(qrValue) ? '' : qrValue,
    qr_url: isHttpURL(qrCodeUrl) ? qrCodeUrl : '',
    deeplink_url: stringValue(complete.deeplink_url)
  };
}

export function canConfirmManualGoPayPayment(job: Job, progress: WorkflowProgress | null, payment: ReturnType<typeof manualGoPayPaymentView>) {
  return !!payment && !payment.confirmed && job.status === 'RUNNING' &&
    (progress?.step_name === 'gopay_payment' || job.last_step === 'gopay_payment');
}

export function canConfirmManualAddBalance(job: Job, progress: WorkflowProgress | null, balance: ReturnType<typeof manualAddBalanceView>) {
  return !!balance && job.status === 'RUNNING' && balance.method === 'manual_transfer' &&
    (progress?.step_name === 'gopay_app_ensure_balance_confirm' || progress?.step_name === 'gopay_app_ensure_balance');
}

export function canSelectGoPayAddBalance(job: Job, progress: WorkflowProgress | null, balance: ReturnType<typeof manualAddBalanceView>) {
  return job.status === 'RUNNING' &&
    (progress?.step_name === 'gopay_app_ensure_balance' || job.last_step === 'gopay_app_ensure_balance') &&
    (!balance?.method || balance.status === 'awaiting_selection');
}

function isHttpURL(value: string) {
  return /^https?:\/\//i.test(String(value || '').trim());
}

function paymentChannelDescriptor(value: string) {
  const normalized = String(value || '').trim().toLowerCase();
  return PAYMENT_CHANNELS.find((item) => item.aliases.includes(normalized) || item.match?.(normalized));
}

function addBalanceDescriptor(value: string) {
  const normalized = String(value || '').trim().toLowerCase();
  return ADD_BALANCE_METHODS.find((item) => item.aliases.includes(normalized) || item.match?.(normalized));
}
