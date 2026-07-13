export type UserCreditsInvalidationEvent =
  | { source: 'task-hold'; taskId: string }
  | {
      source: 'task-settlement'
      taskId: string
      status: 'completed' | 'failed' | 'cancelled'
    }

export type UserCreditsInvalidationListener = (event: UserCreditsInvalidationEvent) => void

const listeners = new Set<UserCreditsInvalidationListener>()

export function notifyUserCreditsInvalidation(event: UserCreditsInvalidationEvent): void {
  for (const listener of listeners) {
    try {
      listener(event)
    } catch (error) {
      console.error('[credits-invalidation] listener failed', { event, error })
    }
  }
}

export function subscribeUserCreditsInvalidation(
  listener: UserCreditsInvalidationListener,
): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
