import type { CreditHealthStatus } from '../../../../shared/api/generated-query/model'

export type CreditHealthTone = 'neutral' | 'success' | 'warning' | 'danger'

export type CreditHealthPresentation = {
  label: string
  summary: string
  badgeLabel: string | null
  tone: CreditHealthTone
  iconClass: string
  iconKey: 'healthy' | 'time' | 'failed' | 'warning'
}

const presentations: Record<CreditHealthStatus, CreditHealthPresentation> = {
  unchecked: {
    label: '尚未检查',
    summary: '还没有成功的账本检查结果。',
    badgeLabel: '未检查',
    tone: 'neutral',
    iconClass: 'text-admin-text-3',
    iconKey: 'time',
  },
  healthy: {
    label: '账本健康',
    summary: '最近检查可信，当前没有待处理异常。',
    badgeLabel: null,
    tone: 'success',
    iconClass: 'text-admin-success',
    iconKey: 'healthy',
  },
  attention: {
    label: '需要关注',
    summary: '存在待处理异常，但当前没有高风险隔离。',
    badgeLabel: '关注',
    tone: 'warning',
    iconClass: 'text-admin-warning',
    iconKey: 'warning',
  },
  critical: {
    label: '账本异常',
    summary: '存在高风险异常或 Ledger Integrity Restriction。',
    badgeLabel: '异常',
    tone: 'danger',
    iconClass: 'text-admin-danger',
    iconKey: 'warning',
  },
  stale: {
    label: '检查已过期',
    summary: '最近成功检查已超过 48 小时。',
    badgeLabel: '过期',
    tone: 'warning',
    iconClass: 'text-admin-warning',
    iconKey: 'time',
  },
  failed: {
    label: '检查失败',
    summary: '最近一次检查失败，上次成功结果不再代表当前状态。',
    badgeLabel: '失败',
    tone: 'danger',
    iconClass: 'text-admin-danger',
    iconKey: 'failed',
  },
}

export function getCreditHealthPresentation(status: CreditHealthStatus) {
  return presentations[status]
}
