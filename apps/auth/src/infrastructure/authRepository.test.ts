import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { createAuthIdentityStore } from './authRepository.js'

describe('AuthIdentityStore', () => {
  it('reads linked login providers with each email identity', async () => {
    const store = createAuthIdentityStore({
      async query() {
        return {
          rows: [
            {
              id: 'auth-user-1',
              email: 'user@example.com',
              providers: ['discord', 'email'],
            },
          ],
          rowCount: 1,
        }
      },
      async connect() {
        throw new Error('not used')
      },
    })

    assert.deepEqual(await store.findUserEmailsByIDs(['auth-user-1']), [
      {
        auth_user_id: 'auth-user-1',
        email: 'user@example.com',
        providers: ['discord', 'email'],
      },
    ])
  })

  it('reads role and ban state for a batch of identities', async () => {
    const queries: Array<{ text: string; values?: unknown[] }> = []
    const store = createAuthIdentityStore({
      async query(text: string, values?: unknown[]) {
        queries.push({ text, values })
        return {
          rows: [
            {
              id: 'auth-user-1',
              role: 'user',
              banned: false,
              banExpires: null,
            },
            {
              id: 'auth-user-2',
              role: 'admin',
              banned: true,
              banExpires: new Date('2026-10-11T03:00:00.000Z'),
            },
            // Rows without a usable identity are dropped, never fabricated.
            { id: '', role: 'user', banned: false, banExpires: null },
            { id: 'auth-user-3', role: '', banned: false, banExpires: null },
          ],
          rowCount: 4,
        }
      },
      async connect() {
        throw new Error('not used')
      },
    })

    assert.deepEqual(await store.findUserStatesByIDs(['auth-user-1', 'auth-user-2']), [
      {
        auth_user_id: 'auth-user-1',
        role: 'user',
        banned: false,
        ban_expires: null,
      },
      {
        auth_user_id: 'auth-user-2',
        role: 'admin',
        banned: true,
        ban_expires: '2026-10-11T03:00:00.000Z',
      },
    ])
    assert.equal(queries.length, 1)
    assert.match(queries[0]?.text ?? '', /FROM "user" users/)
    assert.match(queries[0]?.text ?? '', /WHERE users\.id = ANY\(\$1::text\[\]\)/)
    assert.deepEqual(queries[0]?.values, [['auth-user-1', 'auth-user-2']])
  })

  it('reads identities through the injected database', async () => {
    const queries: Array<{ text: string; values?: unknown[] }> = []
    const store = createAuthIdentityStore({
      async query(text: string, values?: unknown[]) {
        queries.push({ text, values })
        return {
          rows: [
            {
              id: 'auth-user-1',
              email: 'user@example.com',
              name: 'User',
              role: 'user',
              emailVerified: true,
            },
          ],
          rowCount: 1,
        }
      },
      async connect() {
        throw new Error('not used')
      },
    })

    assert.deepEqual(await store.findUserByEmail('user@example.com'), {
      id: 'auth-user-1',
      email: 'user@example.com',
      name: 'User',
      role: 'user',
      emailVerified: true,
    })
    assert.deepEqual(queries, [
      {
        text: 'SELECT id, email, name, role, "emailVerified" FROM "user" WHERE email = $1 LIMIT 1',
        values: ['user@example.com'],
      },
    ])
  })

  it('preserves an existing ban expiry when a partial ban command is retried', async () => {
    const queries: Array<{ text: string; values?: unknown[] }> = []
    const store = createAuthIdentityStore({
      async query(text: string, values?: unknown[]) {
        queries.push({ text, values })
        return { rows: [], rowCount: 1 }
      },
      async connect() {
        throw new Error('not used')
      },
    })

    await store.updateAuthUserBanState('auth-user-1', true, 3_600)

    assert.match(
      queries[0]?.text ?? '',
      /CASE WHEN \$1 AND "banned" = TRUE THEN "banExpires" ELSE \$2 END/,
    )
  })

  it('removes an identity in one transaction and releases the connection', async () => {
    const calls: string[] = []
    let released = false
    const store = createAuthIdentityStore({
      async query() {
        throw new Error('not used')
      },
      async connect() {
        return {
          async query(text: string) {
            calls.push(text)
            if (text.startsWith('SELECT id, email')) {
              return {
                rows: [{ id: 'auth-user-1', email: 'user@example.com' }],
                rowCount: 1,
              }
            }
            return { rows: [], rowCount: 1 }
          },
          release() {
            released = true
          },
        }
      },
    })

    assert.equal(await store.removeAuthUserCascade('auth-user-1'), true)
    assert.equal(calls[0], 'BEGIN')
    assert.equal(calls.at(-1), 'COMMIT')
    assert.equal(released, true)
  })

  it('preserves operation and rollback failures while releasing the connection', async () => {
    const operationError = new Error('delete failed')
    const rollbackError = new Error('rollback failed')
    let released = false
    const store = createAuthIdentityStore({
      async query() {
        throw new Error('not used')
      },
      async connect() {
        return {
          async query(text: string) {
            if (text.startsWith('SELECT id, email')) {
              return {
                rows: [{ id: 'auth-user-1', email: 'user@example.com' }],
                rowCount: 1,
              }
            }
            if (text.startsWith('DELETE FROM "account"')) throw operationError
            if (text === 'ROLLBACK') throw rollbackError
            return { rows: [], rowCount: 1 }
          },
          release() {
            released = true
          },
        }
      },
    })

    await assert.rejects(store.removeAuthUserCascade('auth-user-1'), (error: unknown) => {
      assert.ok(error instanceof AggregateError)
      assert.deepEqual(error.errors, [operationError, rollbackError])
      return true
    })
    assert.equal(released, true)
  })
})
