import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { Pool } from 'pg'
import {
  createPgMailConfigRepository,
  createResendTransport,
  MailDeliveryError,
} from './mailConfigAdapters.js'

const candidate = {
  encryptedApiKey: 'encrypted',
  enabled: true,
  fromName: 'IdeaBoard',
  fromEmail: 'auth@example.com',
  replyTo: '',
  readiness: 'not_tested' as const,
}

describe('PostgreSQL mail config adapter', () => {
  it('updates with compare-and-swap and allocates the version in PostgreSQL', async () => {
    const queries: Array<{ sql: string; values?: unknown[] }> = []
    let released = 0
    const client = {
      async query(sql: string, values?: unknown[]) {
        queries.push({ sql, values })
        if (sql.includes('UPDATE auth_mail_settings')) {
          return { rows: [mailRow(9)], rowCount: 1 }
        }
        return { rows: [], rowCount: 1 }
      },
      release() {
        released += 1
      },
    }
    const pool = { connect: async () => client } as unknown as Pool

    const saved = await createPgMailConfigRepository(pool).put(
      candidate,
      8,
      7,
      'update_mail_config',
    )

    const mutation = queries.find(({ sql }) => sql.includes('UPDATE auth_mail_settings'))
    assert.match(mutation?.sql ?? '', /nextval\('auth_mail_settings_config_version_seq'/)
    assert.match(mutation?.sql ?? '', /enabled = \$2/)
    assert.match(mutation?.sql ?? '', /readiness = \$6/)
    assert.match(mutation?.sql ?? '', /config_version = \$7/)
    assert.equal(mutation?.values?.[1], true)
    assert.equal(mutation?.values?.at(-1), 8)
    assert.equal(saved?.configVersion, 9)
    assert.equal(saved?.enabled, true)
    assert.equal(queries[0]?.sql, 'BEGIN')
    assert.equal(queries.at(-1)?.sql, 'COMMIT')
    assert.equal(released, 1)
  })

  it('returns a conflict without writing audit history', async () => {
    const queries: string[] = []
    const client = {
      async query(sql: string) {
        queries.push(sql)
        if (sql.includes('UPDATE auth_mail_settings')) {
          return { rows: [], rowCount: 0 }
        }
        return { rows: [], rowCount: 1 }
      },
      release() {},
    }
    const pool = { connect: async () => client } as unknown as Pool

    const saved = await createPgMailConfigRepository(pool).put(
      candidate,
      8,
      7,
      'update_mail_config',
    )

    assert.equal(saved, null)
    assert.equal(
      queries.some((sql) => sql.includes('auth_mail_settings_audit')),
      false,
    )
  })

  it('rolls back a failed transaction and always releases the client', async () => {
    const failure = new Error('write failed')
    const queries: string[] = []
    let released = 0
    const client = {
      async query(sql: string) {
        queries.push(sql)
        if (sql.includes('INSERT INTO auth_mail_settings ')) throw failure
        return { rows: [], rowCount: 1 }
      },
      release() {
        released += 1
      },
    }
    const pool = { connect: async () => client } as unknown as Pool

    await assert.rejects(
      () => createPgMailConfigRepository(pool).put(candidate, 0, 7, 'create_mail_config'),
      failure,
    )

    assert.equal(queries.at(-1), 'ROLLBACK')
    assert.equal(released, 1)
  })

  it('preserves both the operation and rollback failures', async () => {
    const failure = new Error('write failed')
    const rollbackFailure = new Error('rollback failed')
    let released = 0
    const client = {
      async query(sql: string) {
        if (sql === 'ROLLBACK') throw rollbackFailure
        if (sql.includes('INSERT INTO auth_mail_settings ')) throw failure
        return { rows: [], rowCount: 1 }
      },
      release() {
        released += 1
      },
    }
    const pool = { connect: async () => client } as unknown as Pool

    await assert.rejects(
      () => createPgMailConfigRepository(pool).put(candidate, 0, 7, 'create_mail_config'),
      (error) =>
        error instanceof AggregateError &&
        error.errors.includes(failure) &&
        error.errors.includes(rollbackFailure),
    )
    assert.equal(released, 1)
  })

  it('inserts the enabled flag for new configurations', async () => {
    const queries: Array<{ sql: string; values?: unknown[] }> = []
    const client = {
      async query(sql: string, values?: unknown[]) {
        queries.push({ sql, values })
        if (sql.includes('INSERT INTO auth_mail_settings ')) {
          return { rows: [mailRow(3, false)], rowCount: 1 }
        }
        return { rows: [], rowCount: 1 }
      },
      release() {},
    }
    const pool = { connect: async () => client } as unknown as Pool

    const saved = await createPgMailConfigRepository(pool).put(
      { ...candidate, enabled: false },
      0,
      7,
      'create_mail_config',
    )

    const insertion = queries.find(({ sql }) => sql.includes('INSERT INTO auth_mail_settings '))
    assert.match(insertion?.sql ?? '', /enabled/)
    assert.equal(insertion?.values?.[1], false)
    assert.equal(saved?.enabled, false)
  })

  it('reads the persisted enabled flag', async () => {
    const poolWith = (row: Record<string, unknown>) =>
      ({
        query: async () => ({ rows: [row] }),
      }) as unknown as Pool

    const disabled = await createPgMailConfigRepository(
      poolWith({ ...mailRow(4), enabled: false }),
    ).get()
    assert.equal(disabled?.enabled, false)

    const enabled = await createPgMailConfigRepository(
      poolWith({ ...mailRow(4), enabled: true }),
    ).get()
    assert.equal(enabled?.enabled, true)
  })
})

describe('Resend transport', () => {
  it('sends the supported HTTP contract without exposing credentials in the body', async () => {
    let request: { url: string; init?: RequestInit } | undefined
    const fetchImpl = (async (url: string | URL | Request, init?: RequestInit) => {
      request = { url: String(url), init }
      return new Response(JSON.stringify({ id: 'email-1' }), { status: 200 })
    }) as typeof fetch
    await createResendTransport(fetchImpl).send(
      {
        from: 'IdeaBoard <auth@example.com>',
        to: 'admin@example.com',
        replyTo: 'support@example.com',
        idempotencyKey: 'workspace-invite:wsi_test',
        content: {
          subject: 'Test',
          html: '<p>Test</p>',
          text: 'Test',
        },
      },
      're_secret',
    )

    assert.equal(request?.url, 'https://api.resend.com/emails')
    assert.equal(new Headers(request?.init?.headers).get('authorization'), 'Bearer re_secret')
    assert.equal(
      new Headers(request?.init?.headers).get('idempotency-key'),
      'workspace-invite:wsi_test',
    )
    assert.doesNotMatch(String(request?.init?.body), /re_secret/)
    assert.deepEqual(JSON.parse(String(request?.init?.body)), {
      from: 'IdeaBoard <auth@example.com>',
      to: ['admin@example.com'],
      reply_to: 'support@example.com',
      subject: 'Test',
      html: '<p>Test</p>',
      text: 'Test',
    })
  })

  it('reports non-2xx responses without exposing the provider body', async () => {
    const fetchImpl = (async () =>
      new Response('api-key=re_secret raw provider payload', {
        status: 422,
      })) as typeof fetch

    await assert.rejects(
      () =>
        createResendTransport(fetchImpl).send(
          {
            from: 'IdeaBoard <auth@example.com>',
            to: 'admin@example.com',
            content: { subject: 'Test', html: '<p>Test</p>', text: 'Test' },
          },
          're_secret',
        ),
      (error) =>
        error instanceof MailDeliveryError &&
        error.code === 'non_2xx' &&
        !error.message.includes('re_secret'),
    )
  })

  it('aborts timed out requests with a typed error', async () => {
    const fetchImpl = ((_url: string | URL | Request, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () =>
          reject(new DOMException('aborted', 'AbortError')),
        )
      })) as typeof fetch

    await assert.rejects(
      () =>
        createResendTransport(fetchImpl, 5).send(
          {
            from: 'IdeaBoard <auth@example.com>',
            to: 'admin@example.com',
            content: { subject: 'Test', html: '<p>Test</p>', text: 'Test' },
          },
          're_secret',
        ),
      (error) => error instanceof MailDeliveryError && error.code === 'timeout',
    )
  })
})

function mailRow(configVersion: number, enabled: boolean = candidate.enabled) {
  return {
    encrypted_api_key: candidate.encryptedApiKey,
    enabled,
    from_name: candidate.fromName,
    from_email: candidate.fromEmail,
    reply_to: candidate.replyTo,
    config_version: configVersion,
    readiness: candidate.readiness,
    updated_at: new Date('2026-08-05T10:00:00Z'),
    last_tested_at: null,
    last_test_error_code: null,
  }
}
