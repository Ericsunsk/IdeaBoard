import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { runAuthMigrations } from './authMigration.js'

describe('runAuthMigrations', () => {
  it('uses isolated migration credentials without mutating the source and always closes', async () => {
    const source = {
      AUTH_INTERNAL_KEY: 'test-internal-key',
      AUTH_PG_USER: 'auth_runtime',
      AUTH_PG_PASSWORD: 'runtime-password',
      PG_MIGRATION_USER: 'schema_owner',
      PG_MIGRATION_PASSWORD: 'owner-password',
    }
    const original = { ...source }
    const calls: string[] = []
    const database = {
      async end() {
        calls.push('close')
      },
    }

    await runAuthMigrations(source, {
      createDatabase(environment) {
        calls.push(`database:${environment.database.user}`)
        assert.equal(environment.database.password, 'owner-password')
        return database
      },
      createOptions(environment, value) {
        calls.push('options')
        assert.equal(environment.database.user, 'schema_owner')
        assert.equal(value, database)
        return {}
      },
      async getAdapter() {
        calls.push('adapter')
        return { id: 'kysely' }
      },
      async getMigrations() {
        calls.push('plan')
        return {
          toBeAdded: [],
          toBeCreated: [],
          async runMigrations() {
            calls.push('migrate')
          },
        }
      },
      async ensureMailConfigSchema() {
        calls.push('mail_schema')
      },
      async ensureSocialProviderSettingsSchema() {
        calls.push('provider_schema')
      },
      async configureRuntimePrivileges(_database, runtimeUser) {
        calls.push(`privileges:${runtimeUser}`)
      },
      log() {},
    })

    assert.deepEqual(source, original)
    assert.deepEqual(calls, [
      'database:schema_owner',
      'options',
      'adapter',
      'plan',
      'mail_schema',
      'provider_schema',
      'privileges:auth_runtime',
      'close',
    ])
  })
})
