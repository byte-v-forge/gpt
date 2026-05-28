import { useEffect, useState } from 'react';
import type React from 'react';
import { CheckCircle2, KeyRound, LogIn, RefreshCcw, Repeat, Save, Send, UserPlus, WalletCards, XCircle } from 'lucide-react';
import { Button, DashboardField, Input, api, useQuery } from '@byte-v-forge/common-ui';
import type { GoPayUserWAPhoneResponse } from '../proto/orchestrator_gopay_app';
import { GoPayPhoneCheck } from './gopay-phone-check';
import type { Job } from './types';

const USER_ID = 'local';

type ActionResult = {
  success?: boolean;
  error_message?: string;
};

type WorkflowResult = {
  job_id?: string;
  started?: boolean;
  error_message?: string;
};

type Props = {
  currentJob?: Job;
  onDone: (message: string, error?: boolean) => void;
  onCancelWorkflow: (jobId: string) => Promise<void>;
  onRefreshState: () => Promise<void> | void;
  onRefreshJobs: () => Promise<void> | void;
};

export function GoPayActionsPanel({ currentJob, onDone, onCancelWorkflow, onRefreshState, onRefreshJobs }: Props) {
  const profile = useQuery({ queryKey: ['gpt', 'gopay', 'profile', USER_ID], queryFn: loadProfile });
  const [waPhone, setWaPhone] = useState('');
  const [pin, setPin] = useState('');
  const [countryCode, setCountryCode] = useState('+62');
  const [phone, setPhone] = useState('');
  const [otp, setOtp] = useState('');
  const [busy, setBusy] = useState('');

  useEffect(() => {
    if (!profile.data) return;
    setWaPhone(profile.data.wa_phone || '');
    setPin(profile.data.pin || '');
    if (!phone && profile.data.wa_phone) setPhone(profile.data.wa_phone);
  }, [phone, profile.data]);

  const disabled = busy !== '';
  const primaryPhone = phone || waPhone;

  return (
    <section className="goPayActions">
      <ActionGroup title="用户资料">
        <DashboardField className="goPayActionField" label="WA 手机号"><Input value={waPhone} placeholder="812..." onChange={(event) => setWaPhone(event.target.value)} /></DashboardField>
        <DashboardField className="goPayActionField" label="PIN"><Input value={pin} type="password" onChange={(event) => setPin(event.target.value)} /></DashboardField>
        <DashboardField className="goPayActionField" label="区号"><Input value={countryCode} placeholder="+62" onChange={(event) => setCountryCode(event.target.value)} /></DashboardField>
        <Button onClick={saveProfile} disabled={disabled}><Save size={15} />保存</Button>
      </ActionGroup>
      <ActionGroup title="手机号检测">
        <GoPayPhoneCheck defaultPhone={primaryPhone} disabled={disabled} onDone={onDone} />
      </ActionGroup>
      <ActionGroup title="需要登录的检测">
        <Button onClick={() => startAppWorkflow('检测余额', 'check_balance')} disabled={disabled}><WalletCards size={15} />检测余额</Button>
        <Button onClick={() => startAppWorkflow('检测PIN', 'check_pin')} disabled={disabled}><CheckCircle2 size={15} />检测PIN</Button>
      </ActionGroup>
      <ActionGroup title="流程">
        <Button onClick={() => startAppWorkflow('登录', 'login')} disabled={disabled}><LogIn size={15} />登录</Button>
        <Button onClick={() => startAppWorkflow('注册', 'signup')} disabled={disabled}><UserPlus size={15} />注册</Button>
        <Button onClick={() => startAppWorkflow('设置PIN', 'ensure_pin_setup')} disabled={disabled}><KeyRound size={15} />设置PIN</Button>
        <Button onClick={() => startAppWorkflow('换绑', 'change_phone')} disabled={disabled}><Repeat size={15} />换绑</Button>
      </ActionGroup>
      <ActionGroup title="手动 OTP">
        <DashboardField className="goPayActionField" label="OTP"><Input value={otp} placeholder="123456" onChange={(event) => setOtp(event.target.value)} /></DashboardField>
        <Button onClick={submitOTP} disabled={disabled || !currentJob}><Send size={15} />提交当前流程</Button>
        <Button variant="destructive" onClick={cancelWorkflow} disabled={disabled || !currentJob}><XCircle size={15} />取消当前流程</Button>
        <Button onClick={() => void refreshAll('已刷新')} disabled={disabled}><RefreshCcw size={15} />刷新</Button>
      </ActionGroup>
    </section>
  );

  async function saveProfile() {
    await run('保存配置', '/api/gpt/gopay/profile', { wa_phone: waPhone, pin }, false);
    await profile.refetch();
  }

  async function startAppWorkflow(label: string, operation: string) {
    await startWorkflow(label, '/api/gpt/workflows/gopay-app', {
      operation,
      user_id: USER_ID,
      phone: primaryPhone,
      country_code: countryCode,
      pin,
      otp_channel: 'wa'
    });
  }

  async function submitOTP() {
    if (!currentJob?.job_id) return onDone('没有运行中的 GoPay 流程', true);
    if (!otp.trim()) return onDone('OTP 不能为空', true);
    await run('提交 OTP', `/api/gpt/jobs/${currentJob.job_id}/otp`, { otp }, true);
    setOtp('');
  }

  async function cancelWorkflow() {
    if (!currentJob?.job_id) return onDone('没有运行中的 GoPay 流程', true);
    setBusy('取消流程');
    try {
      await onCancelWorkflow(currentJob.job_id);
      await refreshAll();
    } finally {
      setBusy('');
    }
  }

  async function run(label: string, path: string, body: Record<string, unknown>, refresh: boolean) {
    setBusy(label);
    try {
      const resp = await api<ActionResult>(path, { method: 'POST', body: JSON.stringify({ user_id: USER_ID, ...body }) });
      onDone(resultText(label, resp), !!resp.error_message);
      if (refresh) await refreshAll();
    } catch (err) {
      onDone(`${label}: ${err instanceof Error ? err.message : String(err)}`, true);
    } finally {
      setBusy('');
    }
  }

  async function startWorkflow(label: string, path: string, body: Record<string, unknown>) {
    setBusy(label);
    try {
      const resp = await api<WorkflowResult>(path, { method: 'POST', body: JSON.stringify(body) });
      onDone(resp.error_message ? `${label}: ${resp.error_message}` : `${label}流程已启动: ${resp.job_id || 'ok'}`, !!resp.error_message);
      await refreshAll();
    } catch (err) {
      onDone(`${label}: ${err instanceof Error ? err.message : String(err)}`, true);
    } finally {
      setBusy('');
    }
  }

  async function refreshAll(message?: string) {
    await Promise.all([onRefreshState(), onRefreshJobs()]);
    if (message) onDone(message);
  }
}

function ActionGroup({ title, children }: { title: string; children: React.ReactNode }) {
  return <div className="goPayActionGroup"><h3>{title}</h3><div>{children}</div></div>;
}


function loadProfile() {
  return api<GoPayUserWAPhoneResponse>(`/api/gpt/gopay/profile?user_id=${USER_ID}`);
}

function resultText(label: string, resp: ActionResult) {
  if (resp.error_message) return `${label}: ${resp.error_message}`;
  return `${label}: ${resp.success === false ? '失败' : '完成'}`;
}
