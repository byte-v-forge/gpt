import { numberValue, objectValue, stringValue } from '@/dashboard/module-kit';
import { stepDetailData } from '@/dashboard/modules/workflow/sdk';
import type { ConcreteGoPayAddBalanceMethod, ConcreteGoPayPaymentChannel, Job, WorkflowProgress } from './types';

export const GO_PAY_PAYMENT_CHANNELS: ConcreteGoPayPaymentChannel[] = ['sms', 'app_wa', 'wa'];

export function paymentChannelValue(value: string): '' | 'gopay_sms' | 'gopay_wa' {
  const normalized = String(value || '').trim().toLowerCase();
  if (!normalized) return '';
  if (normalized === 'sms' || normalized === 'gopay_sms' || normalized === 'gopay-sms') return 'gopay_sms';
  if (normalized === 'wa' || normalized === 'whatsapp' || normalized === 'gopay_wa' || normalized === 'gopay-wa') return 'gopay_wa';
  if (normalized.includes('sms') && normalized.includes('gopay')) return 'gopay_sms';
  if ((normalized.includes('wa') || normalized.includes('whatsapp')) && normalized.includes('gopay')) return 'gopay_wa';
  return '';
}

export function goPayPaymentChannelLabel(value: string) {
  const normalized = String(value || '').trim().toLowerCase();
  if (normalized === 'app_wa' || normalized === 'gopay_app_wa' || normalized === 'gopay-app-wa') return 'Gopay App-WA';
  const channel = paymentChannelValue(value);
  if (channel === 'gopay_sms') return 'Gopay-SMS';
  if (channel === 'gopay_wa') return 'Gopay-WA';
  return '-';
}

export function goPayPaymentActionLabel(channel: ConcreteGoPayPaymentChannel) {
  if (channel === 'wa') return '纯WA支付';
  return goPayPaymentChannelLabel(channel);
}

export function goPayPaymentRequestChannel(channel: ConcreteGoPayPaymentChannel): 'sms' | 'wa' {
  return channel === 'app_wa' ? 'wa' : channel;
}

export function isPureGoPayWAPaymentChannel(channel: ConcreteGoPayPaymentChannel) {
  return channel === 'wa';
}

export function goPayAddBalancePayload(method: ConcreteGoPayAddBalanceMethod) {
  if (method === 'rekberinaja') return { rekberinaja: {} };
  if (method === 'envelope') return { envelope: {} };
  return { manualTransfer: {} };
}

export function addBalanceMethodValue(value: string) {
  if (isRekberinajaActivation(value)) return 'rekberinaja';
  if (isEnvelopeActivation(value)) return 'envelope';
  if (isManualTransferActivation(value)) return 'manual_transfer';
  return '';
}

export function addBalanceMethodLabel(value: string) {
  const method = addBalanceMethodValue(value);
  if (method === 'rekberinaja') return 'R平台';
  if (method === 'envelope') return '红包';
  if (method === 'manual_transfer') return '手动转账';
  return '';
}

export function canRetryGoPayPaymentRebind(job: Job) {
  const result = objectValue(job.result);
  if (job.action === 'GOPAY_PAYMENT_REBIND') return job.status === 'FAILED_RETRYABLE' || job.status === 'FAILED_RECOVERABLE';
  if (job.action !== 'GOPAY_PAYMENT') return false;
  const paymentCompleted = result.payment_completed === true || String(result.payment_completed || '').toLowerCase() === 'true';
  const hasPayment = !!(stringValue(result.charge_ref) || stringValue(result.snap_token));
  const changePhone = objectValue(result.change_phone);
  const changeComplete = result.change_phone_complete === true || changePhone.change_phone_complete === true;
  return paymentCompleted && hasPayment && !changeComplete && ['SUCCEEDED', 'FAILED_RECOVERABLE', 'FAILED_RETRYABLE'].includes(job.status);
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
  return !!payment && !payment.confirmed && job.status === 'RUNNING' && ['GOPAY_QRIS_PAYMENT_ACTIVATE', 'GOPAY_PAYMENT'].includes(job.action || '') &&
    (progress?.step_name === 'gopay_payment' || job.last_step === 'gopay_payment');
}

export function canConfirmManualAddBalance(job: Job, progress: WorkflowProgress | null, balance: ReturnType<typeof manualAddBalanceView>) {
  return !!balance && job.status === 'RUNNING' && ['GOPAY_PAYMENT', 'GOPAY_QRIS_PAYMENT_ACTIVATE'].includes(job.action || '') && balance.method === 'manual_transfer' &&
    (progress?.step_name === 'gopay_app_ensure_balance_confirm' || progress?.step_name === 'gopay_app_ensure_balance');
}

export function canSelectGoPayAddBalance(job: Job, progress: WorkflowProgress | null, balance: ReturnType<typeof manualAddBalanceView>) {
  return job.status === 'RUNNING' && ['GOPAY_PAYMENT', 'GOPAY_QRIS_PAYMENT_ACTIVATE'].includes(job.action || '') &&
    (progress?.step_name === 'gopay_app_ensure_balance' || job.last_step === 'gopay_app_ensure_balance') &&
    (!balance?.method || balance.status === 'awaiting_selection');
}

function isHttpURL(value: string) {
  return /^https?:\/\//i.test(String(value || '').trim());
}

function isManualTransferActivation(value: string) {
  const normalized = String(value || '').trim().toLowerCase();
  return normalized === 'manual_transfer' || normalized === 'manual-transfer' || normalized.includes('manual_transfer') || normalized.includes('手动转账');
}

function isRekberinajaActivation(value: string) {
  const normalized = String(value || '').trim().toLowerCase();
  return normalized === 'rekberinaja' || normalized === 'r_platform' || normalized.includes('rekberinaja') || normalized.includes('r平台');
}

function isEnvelopeActivation(value: string) {
  const normalized = String(value || '').trim().toLowerCase();
  return normalized === 'envelope' || normalized === 'claim_envelope' || normalized.includes('envelope') || normalized.includes('红包');
}
