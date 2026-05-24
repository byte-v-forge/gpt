import { useEffect, useState } from 'react';
import { WalletCards } from 'lucide-react';
import QRCode from 'qrcode';
import { Button, buttonHint } from '@/dashboard/module-kit';
import { addBalanceMethodLabel, canConfirmManualAddBalance, canSelectGoPayAddBalance, manualAddBalanceView } from './gopay-utils';
import type { ConcreteGoPayAddBalanceMethod, Job, WorkflowProgress } from './types';

const ADD_BALANCE_METHODS: ConcreteGoPayAddBalanceMethod[] = ['manual_transfer', 'envelope', 'rekberinaja'];

export function hasGoPayAddBalanceActions(job?: Job, progress?: WorkflowProgress | null) {
  if (!job) return false;
  const balance = manualAddBalanceView(job);
  return canSelectGoPayAddBalance(job, progress || null, balance) || canConfirmManualAddBalance(job, progress || null, balance);
}

export function GoPayAddBalanceActions({ job, progress, busy, onSelect, onConfirm }: {
  job?: Job;
  progress?: WorkflowProgress | null;
  busy: boolean;
  onSelect: (jobId: string, method: ConcreteGoPayAddBalanceMethod) => Promise<void>;
  onConfirm: (jobId: string) => Promise<void>;
}) {
  const [pending, setPending] = useState('');
  if (!job) return null;
  const balance = manualAddBalanceView(job);
  const canSelect = canSelectGoPayAddBalance(job, progress || null, balance);
  const canConfirm = canConfirmManualAddBalance(job, progress || null, balance);
  if (!canSelect && !canConfirm) return null;
  const disabled = busy || !!pending;

  async function runSelect(method: ConcreteGoPayAddBalanceMethod) {
    if (!job) return;
    setPending(method);
    try {
      await onSelect(job.job_id, method);
    } finally {
      setPending('');
    }
  }

  async function runConfirm() {
    if (!job) return;
    setPending('confirm');
    try {
      await onConfirm(job.job_id);
    } finally {
      setPending('');
    }
  }

  return (
    <div className="flex items-center justify-end gap-2" onClick={(event) => event.stopPropagation()}>
      {canSelect && ADD_BALANCE_METHODS.map((method) => (
        <Button key={method} className="h-8 px-2 text-xs" type="button" {...buttonHint(`选择${addBalanceMethodLabel(method)}`)} disabled={disabled} onClick={() => void runSelect(method)}>
          <WalletCards size={13} />{pending === method ? '选择中' : addBalanceMethodLabel(method)}
        </Button>
      ))}
      {canConfirm && balance && (
        <ManualTransferConfirm balance={balance} disabled={disabled} pending={pending === 'confirm'} onConfirm={() => void runConfirm()} />
      )}
    </div>
  );
}

function ManualTransferConfirm({ balance, disabled, pending, onConfirm }: {
  balance: NonNullable<ReturnType<typeof manualAddBalanceView>>;
  disabled: boolean;
  pending: boolean;
  onConfirm: () => void;
}) {
  const payload = balance.transfer.qr_payload;
  const dataUrl = useQRCodeDataURL(payload);
  if (!payload) {
    return (
      <Button className="h-8 px-2 text-xs" type="button" {...buttonHint('确认手动转账已完成')} disabled={disabled} onClick={onConfirm}>
        <WalletCards size={13} />{pending ? '确认中' : '确认加余额'}
      </Button>
    );
  }
  return (
    <div className="flex items-center gap-2 rounded-xl border bg-background p-2 shadow-sm">
      {dataUrl ? <img src={dataUrl} alt="GoPay 加余额码" className="h-24 w-24 rounded bg-white p-1" /> : <div className="flex h-24 w-24 items-center justify-center rounded bg-muted text-xs">QR</div>}
      <div className="grid min-w-0 gap-1 text-left text-xs">
        <strong>扫码给 GoPay 加余额</strong>
        <span className="text-muted-foreground">{balance.transfer.amount || 1} {balance.transfer.currency || 'IDR'}</span>
        {balance.transfer.instructions && <span className="max-w-[180px] truncate text-muted-foreground">{balance.transfer.instructions}</span>}
        <code className="max-w-[180px] truncate rounded bg-muted px-1 py-0.5 text-[10px]">{qrIDFromPayload(payload) || payload}</code>
        <Button className="h-7 px-2 text-xs" type="button" {...buttonHint('确认手动转账已完成')} disabled={disabled} onClick={onConfirm}>
          <WalletCards size={13} />{pending ? '确认中' : '我已转账，继续'}
        </Button>
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
