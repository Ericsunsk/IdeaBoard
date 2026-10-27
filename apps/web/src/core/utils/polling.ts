/** Shared polling primitives — bounded waits and capped exponential backoff. */

export type PollDelayPolicy = {
  initialDelayMs: number
  maxDelayMs: number
  /** Multiplier applied per step; `1` (default) keeps a fixed cadence. */
  backoffFactor?: number | undefined
}

/**
 * Advance a poll delay under the policy. The `+1` floor keeps a zero initial
 * delay from stalling forever.
 */
export function advancePollDelay(
  currentDelayMs: number,
  policy: Pick<PollDelayPolicy, 'maxDelayMs' | 'backoffFactor'>,
): number {
  const factor = policy.backoffFactor ?? 1
  return Math.min(
    policy.maxDelayMs,
    Math.max(currentDelayMs + 1, Math.ceil(currentDelayMs * factor)),
  )
}

export type PollDelayIterator = {
  /** Current delay without advancing. */
  current: () => number
  /** Return the current delay, then advance under the policy. */
  next: () => number
  /** Restore the initial delay (e.g. after a successful attempt). */
  reset: () => void
}

export function createPollDelay(policy: PollDelayPolicy): PollDelayIterator {
  const initial = Math.max(0, policy.initialDelayMs)
  let current = initial
  return {
    current: () => current,
    next() {
      const value = current
      current = advancePollDelay(current, policy)
      return value
    },
    reset() {
      current = initial
    },
  }
}

export function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  signal?.throwIfAborted()
  return new Promise((resolve, reject) => {
    const onAbort = () => {
      globalThis.clearTimeout(timer)
      reject(signal?.reason)
    }
    const timer = globalThis.setTimeout(
      () => {
        signal?.removeEventListener('abort', onAbort)
        resolve()
      },
      Math.max(0, ms),
    )
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}
