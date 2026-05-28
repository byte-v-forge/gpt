import { useEffect, useState } from 'react';
import { Phone, Search } from 'lucide-react';
import { Badge, Button, DashboardField, Input, api } from '@byte-v-forge/common-ui';
import type { GoPayUserCheckPhoneResponse } from '../proto/orchestrator_gopay_app';

const USER_ID = 'local';

type Props = {
  defaultPhone?: string;
  disabled?: boolean;
  onDone: (message: string, error?: boolean) => void;
};

export function GoPayPhoneCheck({ defaultPhone, disabled, onDone }: Props) {
  const [countryCode, setCountryCode] = useState('+62');
  const [phone, setPhone] = useState('');
  const [result, setResult] = useState<GoPayUserCheckPhoneResponse | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!phone && defaultPhone) setPhone(defaultPhone);
  }, [defaultPhone, phone]);

  async function checkPhone() {
    const target = phone.trim();
    if (!target) {
      onDone('检测手机号: 手机号不能为空', true);
      return;
    }
    setBusy(true);
    try {
      const resp = await api<GoPayUserCheckPhoneResponse>('/api/gpt/gopay/user/check-phone', {
        method: 'POST',
        body: JSON.stringify({ user_id: USER_ID, country_code: countryCode, phone: target })
      });
      setResult(resp);
      onDone(phoneCheckToast(resp), !!resp.error_message);
    } catch (err) {
      onDone(`检测手机号: ${err instanceof Error ? err.message : String(err)}`, true);
    } finally {
      setBusy(false);
    }
  }

  const summary = result ? phoneCheckSummary(result) : null;

  return (
    <div className="goPayStandaloneCheck">
      <div className="goPayCheckFields">
        <DashboardField className="goPayActionField" label="区号"><Input value={countryCode} placeholder="+62" onChange={(event) => setCountryCode(event.target.value)} /></DashboardField>
        <DashboardField className="goPayActionField" label="手机号"><Input value={phone} placeholder="812..." onChange={(event) => setPhone(event.target.value)} /></DashboardField>
        <Button onClick={checkPhone} disabled={disabled || busy}><Search size={15} />检测是否注册</Button>
      </div>
      {summary && (
        <div className={`goPayCheckResult ${summary.tone}`}>
          <Phone size={15} />
          <div>
            <strong>{summary.title}</strong>
            <span>{summary.detail}</span>
          </div>
          <Badge className={`badge ${summary.tone}`}>{summary.status}</Badge>
        </div>
      )}
    </div>
  );
}


function phoneCheckToast(resp: GoPayUserCheckPhoneResponse) {
  if (resp.error_message) return `检测手机号: ${resp.error_message}`;
  return `检测手机号: ${phoneCheckSummary(resp).title}`;
}

function phoneCheckSummary(resp: GoPayUserCheckPhoneResponse) {
  const status = (resp.status || '').trim().toLowerCase();
  if (resp.error_message || status === 'error') {
    return { tone: 'bad', status: status || 'error', title: '检测失败', detail: resp.error_message || 'provider 未返回可用判断。' };
  }
  if (status === 'registered' || (!resp.available && resp.success)) {
    return { tone: 'mid', status: status || 'registered', title: '已注册', detail: '这个号码已有 GoPay 用户，适合走登录或换绑。' };
  }
  if (status === 'available' || resp.available) {
    return { tone: 'good', status: status || 'available', title: '未注册，可注册', detail: '这个号码没有 GoPay 用户，适合走注册。' };
  }
  return { tone: 'bad', status: status || 'unknown', title: '无法判断', detail: '返回状态无法映射，需要看 provider 原始状态。' };
}
