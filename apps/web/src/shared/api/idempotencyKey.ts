// Task cancellation is an idempotent command keyed by its target task.
export function taskCancellationIdempotencyKey(taskID: string): string {
  return `task-cancel:${taskID}`
}
