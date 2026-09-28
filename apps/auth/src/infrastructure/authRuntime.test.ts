import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { createAuthRuntime, type AuthRuntimeFactories } from './authRuntime.js'
import type { AuthEnvironment } from './env.js'

const testEnvironment: AuthEnvironment = {
  auth: {
    port: 6001,
    host: '127.0.0.1',
    trustProxy: 'loopback',
    internalKey: 'test-internal-key',
    secret: 'test-secret',
    baseURL: 'http://localhost:5173',
    trustedOrigins: ['http://localhost:5173'],
    apiInternalURL: 'http://127.0.0.1:6000',
  },
  database: {
    host: '127.0.0.1',
    port: 5432,
    user: 'auth_runtime',
    password: 'password',
    database: 'ideaboard',
    sslMode: 'disable',
  },
  secrets: { encryptionKey: '' },
}

describe('AuthRuntime', () => {
  it('constructs isolated resources without starting external work', async () => {
    const calls: string[] = []
    const factories = {
      createDatabase: () => {
        calls.push('create_database')
        return {} as never
      },
      closeDatabase: async () => {
        calls.push('close_database')
      },
      createResources: () => {
        calls.push('create_resources')
        return {
          verifyPrivileges: async () => {
            calls.push('verify_privileges')
          },
          initializeProviders: async () => {
            calls.push('initialize_providers')
          },
          listen: async () => {
            calls.push('listen')
          },
          closeServer: async () => {
            calls.push('close_server')
          },
          cleanupUnverifiedUsers: async () => {
            calls.push('cleanup')
          },
        }
      },
      scheduleCleanup: () => {
        calls.push('schedule_cleanup')
        return () => calls.push('clear_cleanup')
      },
    } satisfies AuthRuntimeFactories

    const runtime = createAuthRuntime(testEnvironment, factories)

    assert.deepEqual(calls, [])
    await runtime.start()
    assert.deepEqual(calls, [
      'create_database',
      'create_resources',
      'verify_privileges',
      'initialize_providers',
      'listen',
      'schedule_cleanup',
    ])
    await runtime.close()
  })

  it('starts in fail-closed order and releases every resource exactly once', async () => {
    const calls: string[] = []
    let clearCleanup: (() => void) | undefined
    const factories = {
      createDatabase: () => {
        calls.push('create_database')
        return {} as never
      },
      closeDatabase: async () => {
        calls.push('close_database')
      },
      createResources: () => ({
        verifyPrivileges: async () => {
          calls.push('verify_privileges')
        },
        initializeProviders: async () => {
          calls.push('initialize_providers')
        },
        listen: async () => {
          calls.push('listen')
        },
        closeServer: async () => {
          calls.push('close_server')
        },
        cleanupUnverifiedUsers: async () => {
          calls.push('cleanup')
        },
      }),
      scheduleCleanup: () => {
        calls.push('schedule_cleanup')
        clearCleanup = () => calls.push('clear_cleanup')
        return clearCleanup
      },
    } satisfies AuthRuntimeFactories

    const runtime = createAuthRuntime(testEnvironment, factories)
    await runtime.start()
    assert.deepEqual(calls, [
      'create_database',
      'verify_privileges',
      'initialize_providers',
      'listen',
      'schedule_cleanup',
    ])

    await Promise.all([runtime.close(), runtime.close()])
    assert.deepEqual(calls, [
      'create_database',
      'verify_privileges',
      'initialize_providers',
      'listen',
      'schedule_cleanup',
      'clear_cleanup',
      'close_server',
      'close_database',
    ])
    assert.ok(clearCleanup)
  })

  it('stops startup safely when shutdown is requested during initialization', async () => {
    const calls: string[] = []
    let releasePrivilegeCheck: (() => void) | undefined
    const privilegeCheck = new Promise<void>((resolve) => {
      releasePrivilegeCheck = resolve
    })
    const runtime = createAuthRuntime(testEnvironment, {
      createDatabase: () => ({}),
      closeDatabase: async () => {
        calls.push('close_database')
      },
      createResources: () => ({
        async verifyPrivileges() {
          calls.push('verify_privileges')
          await privilegeCheck
        },
        async initializeProviders() {
          calls.push('initialize_providers')
        },
        async listen() {
          calls.push('listen')
        },
        async closeServer() {
          calls.push('close_server')
        },
        async cleanupUnverifiedUsers() {},
      }),
      scheduleCleanup: () => {
        calls.push('schedule_cleanup')
        return () => {}
      },
    })

    const start = runtime.start()
    const close = runtime.close()
    releasePrivilegeCheck?.()

    await assert.rejects(start, /shutdown requested/)
    await close
    assert.deepEqual(calls, ['verify_privileges', 'close_server', 'close_database'])
  })

  it('closes the database when resource composition fails', async () => {
    const compositionError = new Error('composition failed')
    const calls: string[] = []
    const runtime = createAuthRuntime(testEnvironment, {
      createDatabase: () => {
        calls.push('create_database')
        return { id: 'database' }
      },
      createResources: () => {
        calls.push('create_resources')
        throw compositionError
      },
      closeDatabase: async (database) => {
        calls.push(`close_database:${String((database as { id: string }).id)}`)
      },
      scheduleCleanup: () => () => {},
    })

    await assert.rejects(() => runtime.start(), compositionError)
    await runtime.close()
    assert.deepEqual(calls, ['create_database', 'create_resources', 'close_database:database'])
  })
})
