import { SESSION_FRESH_THRESHOLD_MS } from '../../../shared/config/timing'

// Auth owns the trust window and predicate shared by the router and session cache.
export const SESSION_TRUST_WINDOW_MS = SESSION_FRESH_THRESHOLD_MS

// The TTL boundary and never-validated sessions are stale.
export function isSessionFresh(validatedAt: number, now: number = Date.now()): boolean {
  if (!Number.isFinite(validatedAt) || validatedAt <= 0) return false
  if (!Number.isFinite(now)) return false
  return now - validatedAt < SESSION_TRUST_WINDOW_MS
}
