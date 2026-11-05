// Unavailable Admin operations expose a stable error code and operator guidance.
export const ADMIN_CONTROL_UNAVAILABLE_CODE = 'admin_control_unavailable'

export type AdminControlUnavailableError = Error & { code: string }

export function adminControlUnavailable(feature: string): AdminControlUnavailableError {
  const messages: Record<string, string> = {
    'task-list': '任务列表暂不可用：任务读取模型尚未上线。',
    'task-detail': '任务详情暂不可用：任务读取模型尚未上线。',
    'monitor-stream': '实时监控流暂不可用，将按轮询刷新。',
    'reward-codes': '奖励码管理暂不可用。',
    'reward-redemptions': '奖励兑换记录暂不可用。',
    'oauth-providers': 'OAuth 供应商设置暂不可用。',
    'logo-upload': 'Logo 上传暂不可用，请直接填写 Logo 地址后保存。',
    'gateway-detection': '模型探测暂不可用。',
    'gateway-preview': '路由透视暂不可用。',
    'gateway-reference': '参照价查询暂不可用。',
    'gateway-observed': '观测数据暂不可用。',
    'gateway-test': '网关连接测试暂不可用。',
  }
  const error = new Error(
    messages[feature] ?? '该管理功能暂不可用。',
  ) as AdminControlUnavailableError
  error.code = ADMIN_CONTROL_UNAVAILABLE_CODE
  return error
}
