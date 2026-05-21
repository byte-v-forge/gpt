import { numberValue, objectValue, stringValue } from '@/dashboard/module-kit';
import { stepDetailData } from '@/dashboard/modules/workflow/sdk';
import type { ConcreteGoPayAddBalanceMethod, ConcreteGoPayPaymentChannel, Job, WorkflowProgress } from './types';

export const GO_PAY_PAYMENT_CHANNELS: ConcreteGoPayPaymentChannel[] = ['sms', 'wa'];

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
  const channel = paymentChannelValue(value);
  if (channel === 'gopay_sms') return 'Gopay-SMS';
  if (channel === 'gopay_wa') return 'Gopay-WA';
  return '-';
}

export function goPayAddBalancePayload(method: ConcreteGoPayAddBalanceMethod) {
  if (method === 'rekberinaja') return { rekberinaja: {} };
  if (method === 'envelope') return { envelope: {} };
  return { manual_transfer: {} };
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
  const data = stepDetailData((job.steps || []).find((item) => item.step_name === 'gopay_app_add_balance'));
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

export function canConfirmManualAddBalance(job: Job, progress: WorkflowProgress | null, balance: ReturnType<typeof manualAddBalanceView>) {
  return !!balance && job.status === 'RUNNING' && job.action === 'GOPAY_PAYMENT' && balance.method === 'manual_transfer' &&
    (progress?.step_name === 'gopay_app_add_balance_confirm' || progress?.step_name === 'gopay_app_add_balance');
}

export function canSelectGoPayAddBalance(job: Job, progress: WorkflowProgress | null, balance: ReturnType<typeof manualAddBalanceView>) {
  return job.status === 'RUNNING' && job.action === 'GOPAY_PAYMENT' &&
    (progress?.step_name === 'gopay_app_add_balance' || job.last_step === 'gopay_app_add_balance') &&
    (!balance?.method || balance.status === 'awaiting_selection');
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
