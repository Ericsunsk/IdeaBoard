import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { APIError } from 'better-auth/api'
import { randomUUID } from 'node:crypto'
import { Pool } from 'pg'
import type { AuthGuardSettings } from './authGuardSettings.js'
import {
  LOGIN_LOCKOUT_COPY,
  createLoginLockoutGuard,
  extractLockoutEmail,
  recordSignInOutcome,
  resolveLoginLockoutPolicy,
  throwIfLocked,
  type LoginLockoutGuard,
} from './loginLockout.js'

type FakeRow = { value: string; expiresAt: Date; updatedAt: Date }

function createFakeDatabase(initial: Record<string, FakeRow> = {}) {
  const rows = new Map(Object.entries(initial))
  let pending = Promise.resolve()
  async function query(text: string, values: unknown[] = []) {
    if (text.startsWith('SELECT value')) {
      const row = rows.get(String(values[0]))
      return { rows: row ? [{ value: row.value, expiresAt: row.expiresAt }] : [] }
    }
    if (text.startsWith('DELETE')) {
      rows.delete(String(values[0]))
      return { rows: [] }
    }
    if (text.startsWith('INSERT')) {
      rows.set(String(values[0]), {
        value: String(values[1]),
        expiresAt: values[2] as Date,
        updatedAt: new Date(),
      })
      return { rows: [] }
    }
    throw new Error(`unexpected query: ${text}`)
  }
  return {
    rows,
    query,
    async connect() {
      let releaseLock: (() => void) | undefined
      return {
        release() {},
        async query(text: string, values: unknown[] = []) {
          if (text === 'BEGIN') return { rows: [] }
          if (text.startsWith('SELECT pg_advisory_xact_lock')) {
            const previous = pending
            pending = new Promise<void>((resolve) => {
              releaseLock = resolve
            })
            await previous
            return { rows: [] }
          }
          if (text === 'COMMIT' || text === 'ROLLBACK') {
            releaseLock?.()
            return { rows: [] }
          }
          return query(text, values)
        },
      }
    },
  }
}

function testSettings(overrides: Partial<AuthGuardSettings> = {}): AuthGuardSettings {
  return {
    registrationEnabled: true,
    windowSeconds: 60,
    maxRequests: 30,
    lockoutThreshold: 3,
    lockoutDurationSeconds: 60,
    ...overrides,
  }
}

function createGuard(
  database: ReturnType<typeof createFakeDatabase>,
  settings: AuthGuardSettings = testSettings(),
  nowValue = { now: 1_000_000 },
): { guard: LoginLockoutGuard; nowValue: { now: number } } {
  const guard = createLoginLockoutGuard(database, async () => settings, {
    now: () => nowValue.now,
    onError: () => {},
  })
  return { guard, nowValue }
}

describe('resolveLoginLockoutPolicy', () => {
  it('applies configured budgets within bounds', () => {
    assert.deepEqual(resolveLoginLockoutPolicy(testSettings()), {
      threshold: 3,
      durationSeconds: 60,
    })
  })

  it('falls back to defaults on missing or out-of-range budgets', () => {
    assert.deepEqual(resolveLoginLockoutPolicy(null), { threshold: 10, durationSeconds: 900 })
    assert.deepEqual(
      resolveLoginLockoutPolicy({ lockoutThreshold: 2, lockoutDurationSeconds: 59 }),
      {
        threshold: 10,
        durationSeconds: 900,
      },
    )
  })
})

describe('loginLockoutGuard', () => {
  it('counts concurrent failures across guard instances without losing increments', async () => {
    const database = createFakeDatabase()
    const guards = Array.from({ length: 10 }, () => createGuard(database).guard)

    const results = await Promise.all(
      guards.map((guard) => guard.recordFailure('concurrent@example.com')),
    )

    assert.equal(results.filter((status) => status.locked).length, 8)
    const stored = database.rows.get('login-lockout:concurrent@example.com')
    assert.ok(stored)
    assert.equal(JSON.parse(stored.value).failures, 10)
    assert.equal((await createGuard(database).guard.check('concurrent@example.com')).locked, true)
  })

  it('serializes successful sign-in with pending failures in the same instance', async () => {
    const database = createFakeDatabase()
    const { guard } = createGuard(database)
    await Promise.all([
      guard.recordFailure('ordered@example.com'),
      guard.recordFailure('ordered@example.com'),
      guard.recordSuccess('ordered@example.com'),
      guard.recordFailure('ordered@example.com'),
    ])
    assert.equal(
      JSON.parse(database.rows.get('login-lockout:ordered@example.com')!.value).failures,
      1,
    )
    assert.equal((await guard.check('ordered@example.com')).locked, false)
  })

  it('rolls back and releases connections when transaction operations fail', async () => {
    for (const failingQuery of ['BEGIN', 'SELECT pg_advisory_xact_lock', 'COMMIT']) {
      const calls: string[] = []
      let released = 0
      let errors = 0
      const database = {
        query: async () => ({ rows: [] }),
        async connect() {
          return {
            release() {
              released += 1
            },
            async query(text: string) {
              calls.push(text)
              if (text.startsWith(failingQuery)) throw new Error('injected store failure')
              return { rows: [] }
            },
          }
        },
      }
      const guard = createLoginLockoutGuard(database, async () => testSettings(), {
        onError: () => {
          errors += 1
        },
      })
      assert.equal((await guard.recordFailure('failure@example.com')).locked, false)
      await guard.recordSuccess('failure@example.com')
      assert.equal(released, 2)
      assert.equal(errors, 2)
      assert.equal(calls.filter((text) => text === 'ROLLBACK').length, 2)
    }
  })

  it('fails open without pretending a query-only store can persist atomic counters', async () => {
    let queries = 0
    let errors = 0
    const guard = createLoginLockoutGuard(
      {
        query: async () => {
          queries += 1
          return { rows: [] }
        },
      },
      async () => testSettings(),
      {
        onError: () => {
          errors += 1
        },
      },
    )
    await Promise.all(Array.from({ length: 10 }, () => guard.recordFailure('query@example.com')))
    assert.equal(queries, 0)
    assert.equal(errors, 10)
    assert.equal((await guard.check('query@example.com')).locked, false)
  })

  it('locks the email after threshold consecutive failures', async () => {
    const database = createFakeDatabase()
    const { guard } = createGuard(database)

    assert.deepEqual(await guard.recordFailure('Alice@Example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
    assert.deepEqual(await guard.recordFailure('alice@example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
    assert.deepEqual(await guard.recordFailure(' alice@example.com '), {
      locked: true,
      retryAfterSeconds: 60,
    })

    const status = await guard.check('ALICE@example.com')
    assert.equal(status.locked, true)
    assert.ok(status.retryAfterSeconds > 0 && status.retryAfterSeconds <= 60)
    const stored = database.rows.get('login-lockout:alice@example.com')
    assert.ok(stored)
    assert.deepEqual(JSON.parse(stored.value).failures, 3)
  })

  it('throws the locked error with 429 semantics and Chinese copy', async () => {
    const database = createFakeDatabase()
    const { guard } = createGuard(database)

    await guard.recordFailure('bob@example.com')
    await guard.recordFailure('bob@example.com')
    await guard.recordFailure('bob@example.com')

    await assert.rejects(
      () => throwIfLocked(guard, 'bob@example.com'),
      (error: unknown) => {
        assert.ok(error instanceof APIError)
        assert.equal((error as { statusCode?: unknown }).statusCode, 429)
        assert.match((error as Error).message, new RegExp(LOGIN_LOCKOUT_COPY))
        return true
      },
    )
  })

  it('clears the counter on successful sign-in', async () => {
    const database = createFakeDatabase()
    const { guard } = createGuard(database)

    await guard.recordFailure('carol@example.com')
    await guard.recordFailure('carol@example.com')
    await guard.recordSuccess('carol@example.com')

    assert.deepEqual(await guard.check('carol@example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
    assert.equal(database.rows.has('login-lockout:carol@example.com'), false)
    assert.deepEqual(await guard.recordFailure('carol@example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
  })

  it('unlocks after the duration and restarts the counter', async () => {
    const database = createFakeDatabase()
    const { guard, nowValue } = createGuard(database)

    await guard.recordFailure('dave@example.com')
    await guard.recordFailure('dave@example.com')
    await guard.recordFailure('dave@example.com')
    assert.equal((await guard.check('dave@example.com')).locked, true)

    nowValue.now += 61_000
    assert.deepEqual(await guard.check('dave@example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
    assert.deepEqual(await guard.recordFailure('dave@example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
  })

  it('survives restarts and propagates clears across instances', async () => {
    const database = createFakeDatabase()
    const settings = testSettings()
    const clock = { now: 1_000_000 }
    const options = { now: () => clock.now, onError: () => {} }
    const first = createLoginLockoutGuard(database, async () => settings, options)
    await first.recordFailure('erin@example.com')
    await first.recordFailure('erin@example.com')
    await first.recordFailure('erin@example.com')

    const second = createLoginLockoutGuard(database, async () => settings, options)
    assert.equal((await second.check('erin@example.com')).locked, true)
    await second.recordSuccess('erin@example.com')
    assert.equal((await second.check('erin@example.com')).locked, false)

    // The first instance still holds the in-memory lock until the
    // revalidation interval elapses, then it observes the cleared row.
    assert.equal((await first.check('erin@example.com')).locked, true)
    clock.now += 31_000
    assert.equal((await first.check('erin@example.com')).locked, false)
  })

  it('never touches foreign verification rows', async () => {
    const database = createFakeDatabase({
      'verify-email:frank@example.com': {
        value: 'otp-hash',
        expiresAt: new Date(2_000_000),
        updatedAt: new Date(1_000_000),
      },
    })
    const { guard } = createGuard(database)

    await guard.recordFailure('frank@example.com')
    await guard.recordFailure('frank@example.com')
    await guard.recordFailure('frank@example.com')
    await guard.recordSuccess('frank@example.com')

    assert.equal(database.rows.get('verify-email:frank@example.com')?.value, 'otp-hash')
  })

  it('fails open when settings or the store are unavailable', async () => {
    const failingSettings = createLoginLockoutGuard(
      createFakeDatabase(),
      async () => {
        throw new Error('settings unavailable')
      },
      { onError: () => {} },
    )
    assert.deepEqual(await failingSettings.check('gail@example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
    // Missing settings degrade to the default threshold of 10: nine
    // failures stay unlocked, the tenth locks.
    for (let index = 0; index < 9; index += 1) {
      assert.deepEqual(await failingSettings.recordFailure('gail@example.com'), {
        locked: false,
        retryAfterSeconds: 0,
      })
    }
    assert.deepEqual(await failingSettings.recordFailure('gail@example.com'), {
      locked: true,
      retryAfterSeconds: 900,
    })

    const failingStore = createLoginLockoutGuard(
      {
        query: async () => {
          throw new Error('store unavailable')
        },
      },
      async () => testSettings(),
      { onError: () => {} },
    )
    assert.deepEqual(await failingStore.check('gail@example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
    await failingStore.recordFailure('gail@example.com')
    await failingStore.recordSuccess('gail@example.com')
  })
})

describe('sign-in outcome recording', () => {
  it('counts API failures and clears on success', async () => {
    const database = createFakeDatabase()
    const { guard } = createGuard(database)

    await recordSignInOutcome(
      guard,
      'henry@example.com',
      new APIError('UNAUTHORIZED', {
        message: 'Invalid email or password',
      }),
    )
    await recordSignInOutcome(guard, 'henry@example.com', { statusCode: 401 })
    assert.deepEqual(await guard.recordFailure('henry@example.com'), {
      locked: true,
      retryAfterSeconds: 60,
    })

    await recordSignInOutcome(guard, 'henry@example.com', { user: { id: 'u1' } })
    assert.deepEqual(await guard.check('henry@example.com'), {
      locked: false,
      retryAfterSeconds: 0,
    })
  })

  it('ignores missing emails and extracts canonical addresses', () => {
    assert.equal(extractLockoutEmail(null), null)
    assert.equal(extractLockoutEmail({}), null)
    assert.equal(extractLockoutEmail({ email: '  Ivy@Example.com ' }), 'ivy@example.com')
  })
})

// This explicit test entrypoint only targets the caller-provided isolated PostgreSQL database.
const integrationDatabaseURL = process.env.AUTH_LOCKOUT_TEST_DATABASE_URL
it(
  'preserves concurrent counters and committed reads in PostgreSQL',
  {
    skip: !integrationDatabaseURL,
  },
  async () => {
    const schema = `lockout_test_${randomUUID().replaceAll('-', '')}`
    const admin = new Pool({ connectionString: integrationDatabaseURL })
    const database = new Pool({
      connectionString: integrationDatabaseURL,
      options: `-c search_path=${schema}`,
      max: 10,
    })
    try {
      await admin.query(`CREATE SCHEMA "${schema}"`)
      await database.query(`CREATE TABLE verification (
      id text PRIMARY KEY, identifier text NOT NULL, value text NOT NULL,
      "expiresAt" timestamptz NOT NULL, "createdAt" timestamptz NOT NULL,
      "updatedAt" timestamptz NOT NULL
    )`)
      const guards = Array.from({ length: 10 }, () =>
        createLoginLockoutGuard(database, async () => testSettings(), {
          onError: (error) => {
            throw error
          },
        }),
      )
      const email = 'pg-concurrent@example.com'
      const statuses = await Promise.all(guards.map((guard) => guard.recordFailure(email)))
      assert.equal(statuses.filter((status) => status.locked).length, 8)
      const rows = await database.query('SELECT value FROM verification WHERE identifier = $1', [
        `login-lockout:${email}`,
      ])
      assert.equal(rows.rows.length, 1)
      assert.equal(JSON.parse(rows.rows[0].value).failures, 10)

      // Hold a replacement between DELETE and INSERT; checks must still see the committed lock.
      const writer = await database.connect()
      try {
        await writer.query('BEGIN')
        await writer.query('SELECT pg_advisory_xact_lock(hashtextextended($1, 0))', [
          `login-lockout:${email}`,
        ])
        await writer.query('DELETE FROM verification WHERE identifier = $1', [
          `login-lockout:${email}`,
        ])
        assert.equal(
          (await createLoginLockoutGuard(database, async () => testSettings()).check(email)).locked,
          true,
        )
        const success = guards[1].recordSuccess(email)
        await writer.query('ROLLBACK')
        await success
      } finally {
        writer.release()
      }
      // A failure from an instance with a stale cached lock must use the cleared durable count.
      assert.equal((await guards[0].recordFailure(email)).locked, false)
      const afterClear = await database.query(
        'SELECT value FROM verification WHERE identifier = $1',
        [`login-lockout:${email}`],
      )
      assert.equal(JSON.parse(afterClear.rows[0].value).failures, 1)
    } finally {
      await database.end()
      await admin.query(`DROP SCHEMA IF EXISTS "${schema}" CASCADE`)
      await admin.end()
    }
  },
)
