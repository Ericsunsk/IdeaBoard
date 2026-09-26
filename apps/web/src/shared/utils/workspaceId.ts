/**
 * Shared workspace-id primitive (issue #835).
 *
 * The single fail-closed workspace resolver for the frontend: a pure
 * `ws_...` assert plus a generic require that throws a caller-supplied
 * message (or error factory) when unset. Feature modules keep one-line
 * wrappers that supply their own copy/typed errors; `shared/` never names
 * feature locale keys.
 */

/** Pure `ws_...` validation: trims and returns the id, throws otherwise. */
export function assertWorkspaceId(value: string): string {
  const trimmed = value.trim()
  if (!/^ws_[A-Za-z0-9_-]+$/.test(trimmed)) {
    throw new Error('Invalid workspace id: expected ws_...')
  }
  return trimmed
}

/**
 * Fail-closed workspace resolution: returns the trimmed explicit value, or
 * throws when blank. The message (or error factory) is caller-supplied so
 * each feature keeps its own guidance copy/typed error.
 */
export function requireWorkspaceId(
  value: string | null | undefined,
  onMissing: string | (() => Error),
): string {
  const trimmed = value?.trim() ?? ''
  if (trimmed) return trimmed
  if (typeof onMissing === 'function') throw onMissing()
  throw new Error(onMissing)
}
