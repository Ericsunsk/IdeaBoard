/**
 * Shared opaque-id alphabet + assert (issue #835 follow-up).
 *
 * Mirrors backend `contract.IsValidPublicID` (`identifiers.go`): every kernel
 * prefix carries a non-empty `[A-Za-z0-9_-]` suffix. All frontend shape
 * asserts (workspace, task, upload, asset, library, vision-operation ids)
 * build on this module so the alphabet can never drift per call site.
 * Throwing fail-closed semantics: trims, returns the id, throws otherwise.
 */

/** Payload alphabet shared by every opaque-id shape (backend-mirrored). */
export const OPAQUE_ID_SUFFIX_SOURCE = '[A-Za-z0-9_-]+'

export function opaqueIdSourceFor(prefixes: readonly string[] | string): string {
  const list = Array.isArray(prefixes) ? prefixes : [prefixes]
  return `^(?:${list.join('|')})_${OPAQUE_ID_SUFFIX_SOURCE}$`
}

/** Fresh `RegExp` for the given prefix(es); no shared mutable instance. */
export function opaqueIdPattern(prefixes: readonly string[] | string): RegExp {
  return new RegExp(opaqueIdSourceFor(prefixes))
}

/** Non-throwing shape check (no trim: callers pass the exact wire value). */
export function isOpaqueIdWithPrefix(value: unknown, prefix: string): boolean {
  return typeof value === 'string' && opaqueIdPattern(prefix).test(value)
}

/** Fail-closed assert: trims and returns the id, throws otherwise. */
export function assertOpaqueId(value: string, prefix: string, label: string): string {
  const trimmed = value.trim()
  if (!opaqueIdPattern(prefix).test(trimmed)) {
    throw new Error(`Invalid ${label}: expected ${prefix}_...`)
  }
  return trimmed
}
