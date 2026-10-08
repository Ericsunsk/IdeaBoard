import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { Pool } from 'pg'
import {
  configureMailConfigRuntimePrivileges,
  ensureMailConfigSchema,
  verifyMailConfigRuntimePrivileges,
} from './mailConfigSchema.js'

describe('mail config schema and privileges', () => {
  it('creates singleton encrypted settings and append-only audit tables', async () => {
    const queries: string[] = []
    const pool = {
      query: async (sql: string) => {
        queries.push(sql)
        return { rows: [] }
      },
    } as unknown as Pool

    await ensureMailConfigSchema(pool)

    const sql = queries.join('\n')
    assert.match(sql, /CREATE TABLE IF NOT EXISTS auth_mail_settings/)
    assert.match(sql, /CHECK \(id = 1\)/)
    assert.match(sql, /encrypted_api_key TEXT NOT NULL/)
    assert.match(sql, /config_version BIGINT NOT NULL/)
    assert.match(sql, /CREATE SEQUENCE IF NOT EXISTS auth_mail_settings_config_version_seq/)
    assert.match(sql, /auth_mail_settings_audit_append_only/)
  })

  it('advances a new config version sequence past an existing stored version', async () => {
    const queries: string[] = []
    const pool = {
      query: async (sql: string) => {
        queries.push(sql)
        return { rows: [] }
      },
    } as unknown as Pool

    await ensureMailConfigSchema(pool)

    const sequenceSync = queries.find((sql) => sql.includes('setval(')) ?? ''
    assert.match(sequenceSync, /MAX\(config_version\)/)
    assert.match(sequenceSync, /auth_mail_settings_config_version_seq/)
    assert.match(sequenceSync, /last_value/)
    assert.match(sequenceSync, /EXISTS \(SELECT 1 FROM auth_mail_settings\)/)
  })

  it('grants runtime writes only to settings and audit inserts', async () => {
    const queries: string[] = []
    const pool = {
      query: async (sql: string) => {
        queries.push(sql)
        return { rows: [] }
      },
    } as unknown as Pool

    await configureMailConfigRuntimePrivileges(pool, 'ideaboard_auth')

    assert.match(queries[0] || '', /SELECT, INSERT, UPDATE, DELETE ON auth_mail_settings/)
    assert.match(queries[1] || '', /SELECT, INSERT ON auth_mail_settings_audit/)
    assert.doesNotMatch(queries[1] || '', /UPDATE|DELETE|TRUNCATE/)
    assert.match(queries.join('\n'), /auth_mail_settings_config_version_seq/)
  })

  for (const { name, row } of [
    {
      name: 'runtime access that can mutate audit history',
      row: {
        can_manage_settings: true,
        can_insert_audit: true,
        can_allocate_config_version: true,
        can_mutate_audit: true,
      },
    },
    {
      name: 'runtime access that cannot allocate config versions',
      row: {
        can_manage_settings: true,
        can_insert_audit: true,
        can_allocate_config_version: false,
        can_mutate_audit: false,
      },
    },
  ]) {
    it(`rejects ${name}`, async () => {
      const pool = {
        query: async () => ({
          rows: [row],
        }),
      } as unknown as Pool

      await assert.rejects(
        () => verifyMailConfigRuntimePrivileges(pool),
        /Unsafe Auth mail config privileges/,
      )
    })
  }
})
