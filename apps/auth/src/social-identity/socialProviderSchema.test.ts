import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { Pool } from 'pg'
import {
  configureSocialProviderSettingsRuntimePrivileges,
  ensureSocialProviderSettingsSchema,
} from './socialProviderSchema.js'

describe('social provider settings schema', () => {
  it('creates a generic encrypted provider settings table', async () => {
    const queries: string[] = []
    const pool = {
      query: async (sql: string) => {
        queries.push(sql)
        return { rows: [] }
      },
    } as unknown as Pool

    await ensureSocialProviderSettingsSchema(pool)

    assert.match(queries.join('\n'), /CREATE TABLE IF NOT EXISTS social_provider_settings/)
    for (const column of [
      'provider',
      'enabled',
      'client_id',
      'encrypted_client_secret',
      'scopes',
      'config_version',
    ]) {
      assert.match(queries.join('\n'), new RegExp(`\\b${column}\\b`))
    }
  })

  it('grants runtime read and save access without destructive privileges', async () => {
    const queries: string[] = []
    const pool = {
      query: async (sql: string) => {
        queries.push(sql)
        return { rows: [] }
      },
    } as unknown as Pool

    await configureSocialProviderSettingsRuntimePrivileges(pool, 'ideaboard_auth')

    assert.match(
      queries.join('\n'),
      /GRANT SELECT, INSERT, UPDATE ON social_provider_settings TO "ideaboard_auth"/,
    )
    assert.doesNotMatch(queries.join('\n'), /DELETE|TRUNCATE|TRIGGER/)
  })
})
