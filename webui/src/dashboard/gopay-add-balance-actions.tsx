import { useState } from 'react';
import { WalletCards } from 'lucide-react';
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
      {canConfirm && (
        <Button className="h-8 px-2 text-xs" type="button" {...buttonHint('确认手动转账已完成')} disabled={disabled} onClick={() => void runConfirm()}>
          <WalletCards size={13} />{pending === 'confirm' ? '确认中' : '确认加余额'}
        </Button>
      )}
    </div>
  );
}
