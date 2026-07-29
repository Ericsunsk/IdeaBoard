import type { AdminCreditRun } from './adminCreditRunHistoryResource'

export function formatReconciliationRunDuration(run: AdminCreditRun): string {
  if (!run.started_at) return '—'
  if (!run.completed_at) return '进行中'
  const startedAt = new Date(run.started_at).getTime()
  const completedAt = new Date(run.completed_at).getTime()
  if (!Number.isFinite(startedAt) || !Number.isFinite(completedAt) || completedAt < startedAt) {
    return '—'
  }
  const totalSeconds = Math.round((completedAt - startedAt) / 1000)
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
