import { sanitizeApiErrorMessage } from '../../../../../shared/utils/errorMessages'
import { i18n } from '../../../../../shared/i18n'

export type TaskWaitFailureKind = 'pending' | 'timeout'

/** Transport-level facts only: the feedback boundary translates `kind` to copy. */
export class TaskResultPendingError extends Error {
  readonly kind: TaskWaitFailureKind

  constructor(kind: TaskWaitFailureKind = 'pending', message = '') {
    super(message)
    this.name = 'TaskResultPendingError'
    this.kind = kind
  }
}

export class TaskCancelledError extends Error {
  readonly kind = 'cancelled' as const

  constructor(message = '') {
    super(message)
    this.name = 'TaskCancelledError'
  }
}

export function isTaskResultPendingError(error: unknown): error is TaskResultPendingError {
  return error instanceof TaskResultPendingError
}

type TaskFailurePayload = {
  error_msg?: string | undefined
  execution_metrics?:
    | {
        failure_stage?: string | undefined
        failure_category?: string | undefined
      }
    | null
    | undefined
}

function normalizeTaskFailureValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function isResultFinalizationFailure(task: TaskFailurePayload): boolean {
  const metrics = task.execution_metrics
  const failureStage = normalizeTaskFailureValue(metrics?.failure_stage)
  const failureCategory = normalizeTaskFailureValue(metrics?.failure_category)
  if (
    failureStage === 'finalizing' &&
    (failureCategory === 'storage_error' || failureCategory === 'finalize_error')
  ) {
    return true
  }

  return false
}

export function createTaskResultFailureError(
  task: TaskFailurePayload,
  fallbackMessage = i18n.global.t('canvas.feedback.imageGenerationFailed'),
): Error {
  if (isResultFinalizationFailure(task)) {
    return new TaskResultPendingError('pending')
  }
  const msg = task.error_msg?.trim() || fallbackMessage
  return new Error(sanitizeApiErrorMessage(msg, fallbackMessage))
}

export function createTaskCancelledError(
  task?: Pick<TaskFailurePayload, 'error_msg'> & { cancel_reason?: string | undefined },
): TaskCancelledError {
  const detail = task?.cancel_reason?.trim() || task?.error_msg?.trim() || ''
  // Backend reason text stays as debug detail; the feedback boundary shows copy for kind='cancelled'.
  return new TaskCancelledError(sanitizeApiErrorMessage(detail, ''))
}
