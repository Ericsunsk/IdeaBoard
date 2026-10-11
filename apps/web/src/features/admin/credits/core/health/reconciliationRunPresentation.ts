import { calcDurationSeconds } from '../../../../../core/utils/dateTime'
import type { AdminCreditRun } from '../../session/health/runHistoryResource'

export function formatReconciliationRunDuration(run: AdminCreditRun): string {
  if (!run.started_at) return '—'
  if (!run.completed_at) return '进行中'
  const totalSeconds = calcDurationSeconds(run.started_at, run.completed_at)
  if (totalSeconds == null) return '—'
  if (totalSeconds < 60) return `${totalSeconds} 秒`
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return seconds === 0 ? `${minutes} 分钟` : `${minutes} 分 ${seconds} 秒`
}

export function formatReconciliationRunResult(run: AdminCreditRun): string {
  if (run.status === 'queued') return '等待执行'
  if (run.status === 'running') return '检查中'
  return `${run.finding_count} 检查发现（含业务追踪） · ${run.critical_count} 严重 Finding · ${run.isolated_wallet_count} Wallet 已受限`
}

export function getReconciliationTriggerLabel(run: AdminCreditRun): string {
  return run.trigger === 'scheduled' ? '自动调度' : '人工触发'
}
