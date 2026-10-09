import type {
  AdminMonitorDetailModel,
  AdminMonitorExecutionAttempt,
} from '../infrastructure/resource'

// Pure execution-attempt presentation labels for the task result dialog.
// No component state, no i18n plumbing: same input, same string.
export function getDetailExecutionProviderLabel(task: AdminMonitorDetailModel) {
  return task.latest_execution?.provider || '-'
}

export function getDetailExecutionModelLabel(task: AdminMonitorDetailModel) {
  const execution = task.latest_execution
  if (!execution) return '-'
  return execution.resolved_model || '-'
}

export function getAttemptProviderLabel(attempt: AdminMonitorExecutionAttempt) {
  return attempt.provider || `模型 ${attempt.resolved_model || '未知'}`
}

export function getAttemptStatusLabel(status: string) {
  return (
    {
      running: '执行中',
      completed: '已完成',
      failed: '失败',
      requeued: '已重试',
      cancelled: '已取消',
    }[status] || status
  )
}

export function getAttemptStatusVariant(
  status: string,
): 'success' | 'danger' | 'warning' | 'neutral' | 'info' {
  const variantByStatus: Record<string, 'success' | 'danger' | 'warning' | 'neutral' | 'info'> = {
    running: 'info',
    completed: 'success',
    failed: 'danger',
    requeued: 'warning',
    cancelled: 'neutral',
  }
  return variantByStatus[status] || 'neutral'
}

export function getAttemptStageLabel(stage: string) {
  return (
    {
      queued: '排队中',
      assigned: '已分配',
      provider: 'Provider',
      finalizing: '结果落盘',
      completed: '已完成',
      failed: '失败',
      cancelled: '已取消',
    }[stage] || stage
  )
}

export function getAttemptFailureLabel(category: string) {
  if (!category) return '-'
  return (
    {
      input_invalid: '输入无效',
      auth: '鉴权',
      quota: '配额',
      timeout: '超时',
      provider_error: 'Provider 错误',
      storage_error: '存储错误',
      finalize_error: '结果处理错误',
      cancelled: '已取消',
      unknown: '未知',
    }[category] || category
  )
}

// Retry-layer presentation labels (#852, ADR-0012 rule 7): gateway
// request retries, model fallback attempts, and River/task retries stay
// distinguishable in Admin views.
export function getAttemptRetryLayerLabel(layer: string | undefined) {
  if (!layer) return '-'
  return (
    {
      gateway: '网关请求',
      model_fallback: '模型兜底',
      river_task: '任务重投',
    }[layer] || layer
  )
}

export function getDetailFinalModelLabel(task: AdminMonitorDetailModel) {
  return task.final_resolved_model || getDetailExecutionModelLabel(task)
}

export function formatAttemptLatency(milliseconds: number) {
  if (milliseconds <= 0) return '-'
  if (milliseconds < 1000) return `${milliseconds} ms`
  return `${(milliseconds / 1000).toFixed(milliseconds < 10000 ? 1 : 0)} s`
}

export function getBillingSettlementLabel(state: string) {
  return (
    {
      held: '待结算',
      free: '免费',
      captured: '已扣费',
      released: '已释放',
    }[state] || state
  )
}
