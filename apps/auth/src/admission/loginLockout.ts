import { APIError } from 'better-auth/api'
import {
  DEFAULT_LOGIN_LOCKOUT_DURATION_SECONDS,
  DEFAULT_LOGIN_LOCKOUT_THRESHOLD,
  MAX_LOGIN_LOCKOUT_DURATION_SECONDS,
  MAX_LOGIN_LOCKOUT_THRESHOLD,
  MIN_LOGIN_LOCKOUT_DURATION_SECONDS,
  MIN_LOGIN_LOCKOUT_THRESHOLD,
  type AuthGuardSettings,
} from './authGuardSettings.js'
import { normalizeCanonicalEmail } from '../infrastructure/authUtils.js'

// Email+password brute-force protection for POST /sign-in/email.
//
// Memory is the fast path; the Better Auth `verification` table is the
// durable path so a restart cannot reset the counter. No new table or
// top-level concept is introduced: lockout rows reuse the existing
// key-value verification seam with a `login-lockout:` identifier prefix
// (Better Auth OTP flows use their own identifiers and never touch this
// prefix; `cleanupUnverifiedUsers` only deletes `user` rows).
//
// Semantics: consecutive failures per canonical email. Reaching the
// threshold locks the email for the configured duration; any successful
// sign-in clears the counter. Both the counting window and the lock
// lifetime equal the duration, so failures older than the duration stop
// counting and an expired lock restarts from zero. Every method is total:
// store or settings failures degrade to fail-open defaults and never
// reject, so a lockout dependency outage cannot block all logins.

export const LOGIN_LOCKOUT_COPY = '账户已暂时锁定，请稍后再试'

const LOGIN_LOCKOUT_IDENTIFIER_PREFIX = 'login-lockout:'
const LOGIN_LOCKOUT_ROW_VERSION = 1
const LOGIN_LOCKOUT_MEMORY_ENTRY_LIMIT = 10_000
const DEFAULT_LOCKED_REVALIDATE_AFTER_MS = 30_000

export type LoginLockoutStatus = {
  locked: boolean
  retryAfterSeconds: number
}

export type LoginLockoutPolicy = {
  threshold: number
  durationSeconds: number
}

type LoginLockoutDatabase = {
  query(text: string, values?: unknown[]): Promise<{ rows: unknown[] }>
  connect?: () => Promise<LoginLockoutConnection>
}

type LoginLockoutConnection = Pick<LoginLockoutDatabase, 'query'> & {
  release(): void
}

export type LoginLockoutGuardOptions = {
  now?: () => number
  onError?: (error: unknown) => void
  revalidateLockedAfterMs?: number
}

export type LoginLockoutGuard = {
  check(email: string): Promise<LoginLockoutStatus>
  recordFailure(email: string): Promise<LoginLockoutStatus>
  recordSuccess(email: string): Promise<void>
}

type MemoryEntry = {
  failures: number
  lockedUntilMs: number
  observedAtMs: number
}

type StoredLockout = {
  failures: number
  lockedUntilMs: number
  expiresAtMs: number
}

export function resolveLoginLockoutPolicy(
  settings?: Pick<AuthGuardSettings, 'lockoutThreshold' | 'lockoutDurationSeconds'> | null,
): LoginLockoutPolicy {
  return {
    threshold: normalizePolicyValue(
      settings?.lockoutThreshold,
      MIN_LOGIN_LOCKOUT_THRESHOLD,
      MAX_LOGIN_LOCKOUT_THRESHOLD,
      DEFAULT_LOGIN_LOCKOUT_THRESHOLD,
    ),
    durationSeconds: normalizePolicyValue(
      settings?.lockoutDurationSeconds,
      MIN_LOGIN_LOCKOUT_DURATION_SECONDS,
      MAX_LOGIN_LOCKOUT_DURATION_SECONDS,
      DEFAULT_LOGIN_LOCKOUT_DURATION_SECONDS,
    ),
  }
}

export function createLoginLockoutGuard(
  database: LoginLockoutDatabase,
  getSettings: () => Promise<AuthGuardSettings>,
  options: LoginLockoutGuardOptions = {},
): LoginLockoutGuard {
  const memory = new Map<string, MemoryEntry>()
  const mutations = new Map<string, Promise<void>>()
  const now = options.now ?? Date.now
  const revalidateLockedAfterMs =
    options.revalidateLockedAfterMs ?? DEFAULT_LOCKED_REVALIDATE_AFTER_MS
  const onError =
    options.onError ??
    ((error: unknown) => {
      console.error('[auth] Login lockout store failed', error)
    })

  async function readPolicy(): Promise<LoginLockoutPolicy> {
    try {
      return resolveLoginLockoutPolicy(await getSettings())
    } catch (error) {
      onError(error)
      return resolveLoginLockoutPolicy(null)
    }
  }

  async function readStored(client: Pick<LoginLockoutDatabase, 'query'>, key: string) {
    const result = await client.query(
      'SELECT value, "expiresAt" FROM "verification" WHERE identifier = $1 ORDER BY "updatedAt" DESC LIMIT 1',
      [lockoutIdentifier(key)],
    )
    const row = result.rows[0] as { value?: unknown; expiresAt?: unknown } | undefined
    return row ? parseStoredLockout(row.value, row.expiresAt) : null
  }

  async function readStoredResult(
    key: string,
  ): Promise<{ stored: StoredLockout | null; readable: boolean }> {
    try {
      return { stored: await readStored(database, key), readable: true }
    } catch (error) {
      onError(error)
      return { stored: null, readable: false }
    }
  }

  async function writeStored(
    client: Pick<LoginLockoutDatabase, 'query'>,
    key: string,
    failures: number,
    lockedUntilMs: number,
    durationMs: number,
    nowMs: number,
  ): Promise<void> {
    const expiresAt = new Date(lockedUntilMs > nowMs ? lockedUntilMs : nowMs + durationMs)
    await client.query('DELETE FROM "verification" WHERE identifier = $1', [lockoutIdentifier(key)])
    await client.query(
      'INSERT INTO "verification" (id, identifier, value, "expiresAt", "createdAt", "updatedAt") VALUES (gen_random_uuid()::text, $1, $2, $3, now(), now())',
      [
        lockoutIdentifier(key),
        JSON.stringify({
          v: LOGIN_LOCKOUT_ROW_VERSION,
          failures,
          lockedUntilMs,
        }),
        expiresAt,
      ],
    )
  }

  async function withStoredMutation<T>(
    key: string,
    action: (client: LoginLockoutConnection) => Promise<T>,
  ): Promise<T> {
    if (!database.connect) throw new Error('login lockout store requires a transaction connection')
    const client = await database.connect()
    try {
      await client.query('BEGIN')
      // Acquire the lock before a separate SELECT so READ COMMITTED sees the previous writer.
      await client.query('SELECT pg_advisory_xact_lock(hashtextextended($1, 0))', [
        lockoutIdentifier(key),
      ])
      const result = await action(client)
      await client.query('COMMIT')
      return result
    } catch (error) {
      try {
        await client.query('ROLLBACK')
      } catch {
        // Preserve the original store error.
      }
      throw error
    } finally {
      client.release()
    }
  }

  async function serializeMutation<T>(key: string, action: () => Promise<T>): Promise<T> {
    // Keep this instance's memory updates in the same order as its durable writes.
    const result = (mutations.get(key) ?? Promise.resolve()).then(action)
    const pending = result.then(
      () => {},
      () => {},
    )
    mutations.set(key, pending)
    try {
      return await result
    } finally {
      if (mutations.get(key) === pending) mutations.delete(key)
    }
  }

  function pruneMemory(nowMs: number): void {
    if (memory.size <= LOGIN_LOCKOUT_MEMORY_ENTRY_LIMIT) return
    // Under memory pressure drop unlocked entries first; their counters
    // survive in the verification table and reload on the next failure.
    for (const [key, entry] of memory) {
      if (entry.lockedUntilMs <= nowMs) memory.delete(key)
      if (memory.size <= LOGIN_LOCKOUT_MEMORY_ENTRY_LIMIT) return
    }
  }

  return {
    async check(email: string): Promise<LoginLockoutStatus> {
      const key = normalizeCanonicalEmail(email)
      if (!key) return { locked: false, retryAfterSeconds: 0 }
      const nowMs = now()
      const entry = memory.get(key)
      if (entry && entry.lockedUntilMs > nowMs) {
        if (nowMs - entry.observedAtMs < revalidateLockedAfterMs) {
          return lockedStatus(entry.lockedUntilMs, nowMs)
        }
        // A lock observed long ago is revalidated against the durable
        // row so a clear written by another process (for example a
        // successful sign-in on a sibling instance) propagates within
        // the interval. An unreadable store keeps the lock (fail-closed
        // for the locked verdict); only an explicitly absent or expired
        // row clears it.
        const revalidated = await readStoredResult(key)
        if (!revalidated.readable) {
          return lockedStatus(entry.lockedUntilMs, nowMs)
        }
        if (revalidated.stored && revalidated.stored.lockedUntilMs > nowMs) {
          memory.set(key, {
            failures: revalidated.stored.failures,
            lockedUntilMs: revalidated.stored.lockedUntilMs,
            observedAtMs: nowMs,
          })
          return lockedStatus(revalidated.stored.lockedUntilMs, nowMs)
        }
        memory.delete(key)
        return { locked: false, retryAfterSeconds: 0 }
      }
      if (entry) memory.delete(key)
      const { stored } = await readStoredResult(key)
      if (stored && stored.lockedUntilMs > nowMs) {
        memory.set(key, {
          failures: stored.failures,
          lockedUntilMs: stored.lockedUntilMs,
          observedAtMs: nowMs,
        })
        return lockedStatus(stored.lockedUntilMs, nowMs)
      }
      return { locked: false, retryAfterSeconds: 0 }
    },

    async recordFailure(email: string): Promise<LoginLockoutStatus> {
      const key = normalizeCanonicalEmail(email)
      if (!key) return { locked: false, retryAfterSeconds: 0 }
      return serializeMutation(key, async () => {
        const policy = await readPolicy()
        try {
          const entry = await withStoredMutation(key, async (client) => {
            const nowMs = now()
            const stored = await readStored(client, key)
            const failures = (stored && stored.expiresAtMs > nowMs ? stored.failures : 0) + 1
            const durationMs = policy.durationSeconds * 1_000
            const lockedUntilMs = failures >= policy.threshold ? nowMs + durationMs : 0
            await writeStored(client, key, failures, lockedUntilMs, durationMs, nowMs)
            return { failures, lockedUntilMs, observedAtMs: nowMs }
          })
          pruneMemory(now())
          memory.set(key, entry)
          return entry.lockedUntilMs > now()
            ? lockedStatus(entry.lockedUntilMs, now())
            : { locked: false, retryAfterSeconds: 0 }
        } catch (error) {
          onError(error)
          const entry = memory.get(key)
          return entry && entry.lockedUntilMs > now()
            ? lockedStatus(entry.lockedUntilMs, now())
            : { locked: false, retryAfterSeconds: 0 }
        }
      })
    },

    async recordSuccess(email: string): Promise<void> {
      const key = normalizeCanonicalEmail(email)
      if (!key) return
      await serializeMutation(key, async () => {
        try {
          await withStoredMutation(key, async (client) => {
            await client.query('DELETE FROM "verification" WHERE identifier = $1', [
              lockoutIdentifier(key),
            ])
          })
        } catch (error) {
          onError(error)
        } finally {
          memory.delete(key)
        }
      })
    },
  }
}

export function extractLockoutEmail(body: unknown): string | null {
  if (!body || typeof body !== 'object') return null
  const email = normalizeCanonicalEmail((body as Record<string, unknown>).email)
  return email || null
}

export async function throwIfLocked(guard: LoginLockoutGuard, email: string | null): Promise<void> {
  if (!email) return
  const status = await guard.check(email)
  if (status.locked) {
    throw new APIError('TOO_MANY_REQUESTS', {
      code: 'ACCOUNT_TEMPORARILY_LOCKED',
      message: LOGIN_LOCKOUT_COPY,
    })
  }
}

export async function recordSignInOutcome(
  guard: LoginLockoutGuard,
  email: string | null,
  returned: unknown,
): Promise<void> {
  if (!email) return
  if (isSignInFailure(returned)) {
    await guard.recordFailure(email)
    return
  }
  await guard.recordSuccess(email)
}

function isSignInFailure(returned: unknown): boolean {
  if (returned instanceof APIError) return true
  if (returned && typeof returned === 'object') {
    const statusCode = (returned as { statusCode?: unknown }).statusCode
    if (typeof statusCode === 'number' && Number.isInteger(statusCode) && statusCode >= 400) {
      return true
    }
  }
  return false
}

function lockoutIdentifier(canonicalEmail: string): string {
  return `${LOGIN_LOCKOUT_IDENTIFIER_PREFIX}${canonicalEmail}`
}

function lockedStatus(lockedUntilMs: number, nowMs: number): LoginLockoutStatus {
  return {
    locked: true,
    retryAfterSeconds: Math.max(1, Math.ceil((lockedUntilMs - nowMs) / 1_000)),
  }
}

function parseStoredLockout(value: unknown, expiresAt: unknown): StoredLockout | null {
  if (typeof value !== 'string') return null
  let parsed: unknown
  try {
    parsed = JSON.parse(value)
  } catch {
    return null
  }
  if (!parsed || typeof parsed !== 'object') return null
  const record = parsed as { v?: unknown; failures?: unknown; lockedUntilMs?: unknown }
  if (record.v !== LOGIN_LOCKOUT_ROW_VERSION) return null
  if (
    typeof record.failures !== 'number' ||
    !Number.isInteger(record.failures) ||
    record.failures < 0
  ) {
    return null
  }
  if (
    typeof record.lockedUntilMs !== 'number' ||
    !Number.isFinite(record.lockedUntilMs) ||
    record.lockedUntilMs < 0
  ) {
    return null
  }
  const expiresAtMs = normalizeTimestamp(expiresAt)
  if (expiresAtMs === null) return null
  return { failures: record.failures, lockedUntilMs: record.lockedUntilMs, expiresAtMs }
}

function normalizeTimestamp(raw: unknown): number | null {
  if (raw instanceof Date) {
    const time = raw.getTime()
    return Number.isFinite(time) ? time : null
  }
  if (typeof raw === 'string' || typeof raw === 'number') {
    const time = new Date(raw).getTime()
    return Number.isFinite(time) ? time : null
  }
  return null
}

function normalizePolicyValue(raw: unknown, min: number, max: number, fallback: number): number {
  const value =
    typeof raw === 'number' ? raw : typeof raw === 'string' && raw.trim() ? Number(raw) : NaN
  if (!Number.isInteger(value) || value < min || value > max) {
    return fallback
  }
  return value
}
