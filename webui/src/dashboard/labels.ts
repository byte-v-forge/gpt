import type { DisplayLabelMap } from './types';

const accountStatusLabels: DisplayLabelMap = {
  UNREGISTERED: '未注册',
  REGISTERED: '已注册',
  ACTIVATED: '已激活',
  DEACTIVATED: '已停用',
  USER_ALREADY_EXISTS: '用户已存在',
  EMAIL_ALREADY_EXISTS: '用户已存在',
  REGISTER_FAILED: '注册失败',
  PAYMENT_FAILED: '支付失败'
};

const jobStatusLabels: DisplayLabelMap = {
  RUNNING: '运行中',
  SUCCEEDED: '成功',
  CANCELED: '已取消',
  FAILED_RETRYABLE: '失败',
  FAILED_RECOVERABLE: '失败，需处理',
  FAILED_FINAL: '最终失败'
};

const emailAllocationStatusLabels: DisplayLabelMap = {
  AVAILABLE: '可用',
  ASSIGNED: '已分配',
  REGISTERED: '已注册',
  USER_ALREADY_EXISTS: '用户已存在',
  REGISTRATION_FAILED: '注册失败',
  BLOCKED: '停止分配'
};

const actionLabels: DisplayLabelMap = {
  REGISTER: '注册账号',
  REGISTER_PROTOCOL: '协议注册账号',
  LOGIN_SESSION: '登录取 Token',
  LOGIN_SESSION_PROTOCOL: '协议登录取 Token',
  CODEX_OAUTH: '浏览器 auth.json',
  CODEX_OAUTH_PROTOCOL: '协议 auth.json',
  CODEX_OAUTH_ADD_PHONE: 'Codex OAuth 加手机号',
  ACTIVATE: '激活支付',
  AUTOPAY: '自动支付',
  GOPAY_APP: 'GoPay App',
  GOPAY_PAYMENT: 'GoPay 支付',
  GOPAY_QRIS_PAYMENT_ACTIVATE: 'QRIS激活',
  GOPAY_WA_PAYMENT: '纯 GoPay-WA 支付',
  GOPAY_PAYMENT_REBIND: 'GoPay 支付换绑',
  REGISTER_AND_ACTIVATE: '注册并激活',
  PROBE_ACCOUNT: '探测账号'
};

export function statusText(status: string) {
  return accountStatusLabels[status] || jobStatusLabels[status] || emailAllocationStatusLabels[status] || status || '-';
}

export function actionText(action: string) {
  return actionLabels[action] || action || '-';
}
