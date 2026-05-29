import { GPT_ACTIONS, type GptActionCatalog } from './action-catalog';
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
  [GPT_ACTIONS.register]: '注册账号',
  [GPT_ACTIONS.registerProtocol]: '协议注册账号',
  [GPT_ACTIONS.loginSession]: '登录更新认证',
  [GPT_ACTIONS.loginSessionProtocol]: '协议登录更新认证',
  [GPT_ACTIONS.codexOAuth]: '浏览器 auth.json',
  [GPT_ACTIONS.codexOAuthProtocol]: '协议 auth.json',
  [GPT_ACTIONS.codexOAuthAddPhone]: 'Codex OAuth 加手机号',
  [GPT_ACTIONS.goPayApp]: 'GoPay App',
  [GPT_ACTIONS.goPayPayment]: 'GoPay 支付',
  [GPT_ACTIONS.goPayQRISPaymentActivate]: 'QRIS激活',
  [GPT_ACTIONS.goPayWAPayment]: '纯 GoPay-WA 支付',
  [GPT_ACTIONS.goPayPaymentRebind]: 'GoPay 支付换绑',
  [GPT_ACTIONS.probeAccount]: '探测账号'
};

export function statusText(status: string) {
  return accountStatusLabels[status] || jobStatusLabels[status] || emailAllocationStatusLabels[status] || status || '-';
}

export function actionText(action: string, catalog?: GptActionCatalog) {
  return catalog?.actions.find((item) => item.action_id === action)?.display_name || actionLabels[action] || action || '-';
}
