export function withSignal(
  signal: AbortSignal | null | undefined,
): { signal: AbortSignal } | Record<string, never> {
  return signal == null ? {} : { signal }
}

/** Bound the whole operation, including I/O that ignores AbortSignal. */
export async function runWithDeadline<T>(
  deadlineAt: number,
  parentSignal: AbortSignal | undefined,
  operation: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  const controller = new AbortController()
  const abort = () => controller.abort(parentSignal?.reason)
  parentSignal?.addEventListener('abort', abort, { once: true })
  if (parentSignal?.aborted) abort()
  const timeoutError = new DOMException('Request timed out', 'TimeoutError')
  const remainingMs = deadlineAt - Date.now()
  if (remainingMs <= 0) controller.abort(timeoutError)
  const timer = globalThis.setTimeout(
    () => controller.abort(timeoutError),
    Math.max(0, remainingMs),
  )
  let rejectOnAbort: () => void = () => undefined
  try {
    controller.signal.throwIfAborted()
    const aborted = new Promise<never>((_, reject) => {
      rejectOnAbort = () => reject(controller.signal.reason)
      controller.signal.addEventListener('abort', rejectOnAbort, { once: true })
    })
    return await Promise.race([operation(controller.signal), aborted])
  } finally {
    globalThis.clearTimeout(timer)
    parentSignal?.removeEventListener('abort', abort)
    controller.signal.removeEventListener('abort', rejectOnAbort)
    controller.abort()
  }
}
