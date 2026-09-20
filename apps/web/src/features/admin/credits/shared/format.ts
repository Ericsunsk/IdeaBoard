import type {
  CreditLedgerTransaction,
  CreditReconciliationRun,
} from '../../../../shared/api/generated-query/model'
import { sanitizeApiErrorMessage } from '../../../../shared/utils/errorMessages'

const dateTimeFormat = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
})

/**
 * Admin-local fixed `zh-CN` timestamp with seconds.
 * Named distinctly from `shared/utils/dateTime.formatAdminDateTime`
 * (locale-aware, no seconds) to avoid cross-module misuse.
 */
export function formatAdminDateTimeWithSeconds(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : dateTimeFormat.format(date)
}

const commandLabels: Record<CreditLedgerTransaction['command_type'], string> = {
  grant: '赠送',
  deduction: '扣减',
  hold: '冻结',
  capture: '消费',
  release: '解冻',
  refund: '退款',
  correction: '修正',
}

export function getCreditCommandLabel(command: CreditLedgerTransaction['command_type']) {
  return commandLabels[command]
}

export function getCreditCommandSign(command: CreditLedgerTransaction['command_type']) {
  return command === 'grant' || command === 'release' || command === 'refund' ? '+' : '-'
}

export function getCreditCommandSeverity(command: CreditLedgerTransaction['command_type']) {
  if (command === 'grant' || command === 'release' || command === 'refund') return 'success'
  if (command === 'deduction' || command === 'capture') return 'danger'
  return 'warn'
}

export function getRunStatusLabel(status: CreditReconciliationRun['status']) {
  return { queued: '排队中', running: '执行中', completed: '已完成', failed: '失败' }[status]
}

export function getRunStatusSeverity(status: CreditReconciliationRun['status']) {
  if (status === 'completed') return 'success'
  if (status === 'failed') return 'danger'
  return 'warn'
}

export function formatReconciliationRunError(message?: string) {
  return sanitizeApiErrorMessage(message, '账本检查失败，请查看服务日志并重试')
}
