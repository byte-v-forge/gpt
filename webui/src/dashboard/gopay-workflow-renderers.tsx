import { useEffect, useState } from 'react';
import { Badge, KVList, numberValue, objectValue, stringValue } from '@/dashboard/module-kit';
import { registerWorkflowStepRenderers, stepDetailData } from '@/dashboard/modules/workflow/sdk';
import type { WorkflowStepRendererProps } from '@/dashboard/modules/workflow/sdk';
import QRCode from 'qrcode';

let registered = false;

export function registerGoPayWorkflowRenderers() {
  if (registered) return;
  registered = true;
  registerWorkflowStepRenderers([
    {
      id: 'gpt.gopay.ensure_balance',
      stepNames: ['gopay_app_ensure_balance'],
      jobActions: ['GOPAY_PAYMENT', 'GOPAY_QRIS_PAYMENT_ACTIVATE'],
      label: 'GoPay 确保余额',
      render: (props) => <GoPayAddBalanceStep {...props} />
    },
    {
      id: 'gpt.gopay.pin',
      stepNames: ['gopay_app_ensure_pin_setup'],
      label: 'GoPay 确认 PIN'
    }
  ]);
}

function GoPayAddBalanceStep({ step }: WorkflowStepRendererProps) {
  const data = stepDetailData(step) || {};
  const transfer = objectValue(data.manual_transfer);
  const status = stringValue(data.status);
  const method = stringValue(data.method);
  const payload = stringValue(transfer.qr_payload);
  if (!payload) return <AddBalanceSummary status={status} method={method} data={data} />;
  return <ManualTransferCode payload={payload} transfer={transfer} status={status} />;
}

function AddBalanceSummary({ status, method, data }: {
  status: string;
  method: string;
  data: Record<string, any>;
}) {
  const methods = Array.isArray(data.methods) ? data.methods.join(' / ') : '';
  return (
    <div className="goPayWorkflowStep">
      <Badge variant="outline">{status || 'awaiting_selection'}</Badge>
      <KVList items={[
        { label: '方式', value: method || '待选择' },
        { label: '可选方式', value: methods || '-', visible: !!methods }
      ]} />
    </div>
  );
}

function ManualTransferCode({ payload, transfer, status }: {
  payload: string;
  transfer: Record<string, any>;
  status: string;
}) {
  const dataUrl = useQRCodeDataURL(payload);
  const amount = numberValue(transfer.amount) || 1;
  const currency = stringValue(transfer.currency) || 'IDR';
  const qrID = qrIDFromPayload(payload);
  return (
    <div className="goPayWorkflowStep goPayManualTransferStep">
      <div className="goPayManualTransferQR">
        {dataUrl ? <img src={dataUrl} alt="GoPay 手动转账码" /> : <span>QR</span>}
      </div>
      <div className="goPayManualTransferInfo">
        <div className="goPayManualTransferHead">
          <strong>手动转账码</strong>
          <Badge variant="outline">{status || 'awaiting_manual_confirmation'}</Badge>
        </div>
        <KVList items={[
          { label: 'QR ID', value: qrID || '-', copyValue: qrID, mono: true },
          { label: '转账码', value: payload, copyValue: payload, mono: true },
          { label: '金额', value: `${amount} ${currency}` },
          { label: '说明', value: stringValue(transfer.instructions) || '-', visible: !!stringValue(transfer.instructions) }
        ]} />
      </div>
    </div>
  );
}

function useQRCodeDataURL(payload: string) {
  const [dataUrl, setDataUrl] = useState('');
  useEffect(() => {
    let alive = true;
    if (!payload) { setDataUrl(''); return; }
    QRCode.toDataURL(payload, { errorCorrectionLevel: 'M', margin: 1, width: 176 })
      .then((value) => { if (alive) setDataUrl(value); })
      .catch(() => { if (alive) setDataUrl(''); });
    return () => { alive = false; };
  }, [payload]);
  return dataUrl;
}

function qrIDFromPayload(payload: string) {
  try {
    const parsed = JSON.parse(payload) as { qr_id?: unknown };
    return typeof parsed.qr_id === 'string' ? parsed.qr_id : '';
  } catch {
    return '';
  }
}
