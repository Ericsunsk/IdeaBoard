import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { AuthAppConfig, AuthAppDependencies } from './authApp.js'
import { createAuthApp } from './authApp.js'
import {
  createAuthInternalCommands,
  type AuthInternalCommandRepository,
  type AuthInternalCommands,
  type InternalAuthAPI,
} from './authInternalCommands.js'

type TestOverrides = Omit<
  Partial<AuthAppDependencies>,
  'authApi' | 'config' | 'internalCommands' | 'repository'
> & {
  authApi?: Partial<InternalAuthAPI>
  config?: Partial<AuthAppConfig>
  internalCommands?: Partial<AuthInternalCommands>
  repository?: Partial<AuthInternalCommandRepository>
}

function createTestDeps(overrides: TestOverrides = {}): AuthAppDependencies {
  const config: AuthAppConfig = {
    trustProxy: false,
    internalKey: 'test-internal-key',
    trustedOrigins: ['https://app.example.com'],
    ...overrides.config,
  }
  const authApi: InternalAuthAPI = {
    getSession: async () => ({
      user: {
        id: 'auth-user-1',
        username: 'alice',
        email: 'alice@example.com',
      },
    }),
    signUpEmail: async ({ body }) => ({
      user: {
        id: 'auth-user-2',
        username: body.username,
        email: body.email,
      },
    }),
    ...overrides.authApi,
  }
  const repository: AuthInternalCommandRepository = {
    findUserByUsernameOrEmail: async () => null,
    finalizeTrustedRegistrationIdentity: async () => true,
    updateAuthUserRole: async () => true,
    updateAuthUserBanState: async () => true,
    deleteAuthUserSessions: async () => true,
    removeAuthUserCascade: async () => true,
    ...overrides.repository,
  }
  const internalCommands: AuthInternalCommands = {
    ...createAuthInternalCommands(authApi, repository),
    ...overrides.internalCommands,
  }

  const baseDeps: AuthAppDependencies = {
    config,
    healthCheck: async () => {},
    authHandler: async (_req, res) => {
      res.statusCode = 204
      res.end()
    },
    authApi,
    repository,
    internalCommands,
    getAuthGuardSettings: async () => ({
      registrationEnabled: true,
      windowSeconds: 60,
      maxRequests: 30,
    }),
  }

  return {
    ...baseDeps,
    ...overrides,
    config,
    authApi,
    internalCommands,
    repository,
  }
}

describe('createAuthApp', () => {
  it('serves health checks through an injected database check', async (t) => {
    let checked = false
    const app = createAuthApp(
      createTestDeps({
        healthCheck: async () => {
          checked = true
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({ method: 'GET', url: '/api/health' })

    assert.equal(response.statusCode, 200)
    assert.deepEqual(response.json(), {
      code: 0,
      msg: 'success',
      data: {
        status: 'ok',
        database: 'ok',
      },
    })
    assert.equal(checked, true)
  })

  it('rejects internal routes without the internal key before calling auth APIs', async (t) => {
    let sessionChecked = false
    const app = createAuthApp(
      createTestDeps({
        authApi: {
          getSession: async () => {
            sessionChecked = true
            return { user: { id: 'auth-user-1' } }
          },
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({ method: 'GET', url: '/internal/session' })

    assert.equal(response.statusCode, 401)
    assert.deepEqual(response.json(), {
      code: 401,
      msg: 'unauthorized internal request',
    })
    assert.equal(sessionChecked, false)
  })

  it('returns trusted canonical identity state for Credit Wallet provisioning', async (t) => {
    const app = createAuthApp(
      createTestDeps({
        authApi: {
          getSession: async () => ({
            user: {
              id: 'auth-user-verified',
              username: 'verified-user',
              email: 'verified-user@users.ideaboard.local',
              emailVerified: true,
            },
          }),
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'GET',
      url: '/internal/session',
      headers: { 'x-internal-key': 'test-internal-key' },
    })

    assert.equal(response.statusCode, 200)
    assert.equal(response.json().data.session.claim_identity_verified, true)
  })

  it('registers a trusted canonical username identity without accepting caller email', async (t) => {
    let signupBody: unknown
    let trustedIdentity: [string, string] | null = null
    const app = createAuthApp(
      createTestDeps({
        authApi: {
          signUpEmail: async ({ body }) => {
            signupBody = body
            return { user: { id: 'auth-alice', username: body.username, email: body.email } }
          },
        },
        repository: {
          finalizeTrustedRegistrationIdentity: async (authUserID, claimIdentity) => {
            trustedIdentity = [authUserID, claimIdentity]
            return true
          },
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/api/auth/register/username',
      payload: { username: ' Alice ', password: 'secret123', email: 'attacker@example.com' },
    })

    assert.equal(response.statusCode, 200)
    assert.deepEqual(signupBody, {
      username: 'alice', name: 'alice', password: 'secret123', email: 'alice@users.ideaboard.local',
    })
    assert.deepEqual(trustedIdentity, ['auth-alice', 'alice@users.ideaboard.local'])
    assert.equal(response.json().data.user.claim_identity_verified, true)
  })

  it('blocks direct Better Auth email sign-up before it can create an untrusted account', async (t) => {
    let authHandlerCalled = false
    let markedTrusted = false
    const app = createAuthApp(
      createTestDeps({
        authHandler: async (_req, res) => {
          authHandlerCalled = true
          res.statusCode = 204
          res.end()
        },
        repository: {
          finalizeTrustedRegistrationIdentity: async () => {
            markedTrusted = true
            return true
          },
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/api/auth/sign-up/email',
      payload: { email: 'attacker@example.com', username: 'alice', password: 'secret123' },
    })

    assert.equal(response.statusCode, 404)
    assert.equal(authHandlerCalled, false)
    assert.equal(markedTrusted, false)
  })

  it('fails loudly when incomplete trusted registration cleanup returns false', async (t) => {
    t.mock.method(console, 'error', () => {})
    const app = createAuthApp(
      createTestDeps({
        repository: {
          finalizeTrustedRegistrationIdentity: async () => false,
          removeAuthUserCascade: async () => false,
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/api/auth/register/username',
      payload: { username: 'cleanup-failure', password: 'secret123' },
    })

    assert.equal(response.statusCode, 500)
    assert.match(response.json().msg, /cleanup requires operator attention/)
  })

  it('recovers the created user id and cleans up when sign-up returns an empty payload', async (t) => {
    t.mock.method(console, 'error', () => {})
    const removed: string[] = []
    const app = createAuthApp(
      createTestDeps({
        authApi: { signUpEmail: async () => ({}) },
        repository: {
          findUserByUsernameOrEmail: async () => ({
            id: 'auth-empty-payload', username: 'empty-payload', email: 'empty-payload@users.ideaboard.local', role: 'user',
          }),
          removeAuthUserCascade: async (authUserID) => {
            removed.push(authUserID)
            return true
          },
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST', url: '/api/auth/register/username',
      payload: { username: 'empty-payload', password: 'secret123' },
    })

    assert.equal(response.statusCode, 500)
    assert.equal(response.json().msg, 'registration failed')
    assert.deepEqual(removed, ['auth-empty-payload'])
  })

  it('does not expose public registration dependency errors', async (t) => {
    t.mock.method(console, 'error', () => {})
    const app = createAuthApp(
      createTestDeps({
        authApi: {
          signUpEmail: async () => {
            throw new Error('password=secret constraint users_email_key')
          },
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/api/auth/register/username',
      payload: { username: 'dependency-failure', password: 'secret123' },
    })

    assert.equal(response.statusCode, 500)
    assert.deepEqual(response.json(), { code: 500, msg: 'registration failed' })
    assert.doesNotMatch(response.body, /password=secret|users_email_key/)
  })

  it('returns a fixed conflict for duplicate public usernames', async (t) => {
    const app = createAuthApp(
      createTestDeps({
        authApi: {
          signUpEmail: async () => {
            throw new Error('user already exists: users_email_key')
          },
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/api/auth/register/username',
      payload: { username: 'duplicate-user', password: 'secret123' },
    })

    assert.equal(response.statusCode, 409)
    assert.deepEqual(response.json(), { code: 409, msg: 'username already exists' })
    assert.doesNotMatch(response.body, /users_email_key/)
  })

  for (const username of ['admin', 'ADMIN', 'deleted-user-123', 'DELETED-USER-ABC']) {
    it(`rejects reserved public username ${username} after canonicalization`, async (t) => {
      let signupCalled = false
      const app = createAuthApp(createTestDeps({ authApi: {
        signUpEmail: async () => {
          signupCalled = true
          return {}
        },
      } }))
      t.after(() => app.close())

      const response = await app.inject({
        method: 'POST', url: '/api/auth/register/username',
        payload: { username, password: 'secret123' },
      })

      assert.equal(response.statusCode, 409)
      assert.equal(signupCalled, false)
    })
  }

  it('blocks sign-up requests when registration is disabled', async (t) => {
    let authHandlerCalled = false
    const app = createAuthApp(
      createTestDeps({
        getAuthGuardSettings: async () => ({
          registrationEnabled: false,
          windowSeconds: 60,
          maxRequests: 30,
        }),
        authHandler: async (_req, res) => {
          authHandlerCalled = true
          res.statusCode = 204
          res.end()
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/api/auth/sign-up/email',
      payload: {
        email: 'new@example.com',
        password: 'secret123',
      },
    })

    assert.equal(response.statusCode, 403)
    assert.deepEqual(response.json(), {
      code: 403,
      msg: '注册已关闭',
    })
    assert.equal(authHandlerCalled, false)
  })

  it('handles trusted CORS preflight before querying guard settings', async (t) => {
    let settingsQueried = false
    let authHandlerCalled = false
    const app = createAuthApp(
      createTestDeps({
        getAuthGuardSettings: async () => {
          settingsQueried = true
          return {
            registrationEnabled: true,
            windowSeconds: 60,
            maxRequests: 30,
          }
        },
        authHandler: async (_req, res) => {
          authHandlerCalled = true
          res.statusCode = 204
          res.end()
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'OPTIONS',
      url: '/api/auth/sign-in/email',
      headers: {
        origin: 'https://app.example.com',
        'access-control-request-headers': 'content-type, authorization',
      },
    })

    assert.equal(response.statusCode, 204)
    assert.equal(response.headers['access-control-allow-origin'], 'https://app.example.com')
    assert.equal(response.headers['access-control-allow-credentials'], 'true')
    assert.equal(
      response.headers['access-control-allow-headers'],
      'content-type, authorization',
    )
    assert.equal(settingsQueried, false)
    assert.equal(authHandlerCalled, false)
  })

  it('rejects disallowed CORS preflight before querying guard settings', async (t) => {
    let settingsQueried = false
    let authHandlerCalled = false
    const app = createAuthApp(
      createTestDeps({
        getAuthGuardSettings: async () => {
          settingsQueried = true
          return {
            registrationEnabled: true,
            windowSeconds: 60,
            maxRequests: 30,
          }
        },
        authHandler: async (_req, res) => {
          authHandlerCalled = true
          res.statusCode = 204
          res.end()
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'OPTIONS',
      url: '/api/auth/sign-in/email',
      headers: {
        origin: 'https://evil.example.com',
        'access-control-request-headers': 'content-type, authorization',
      },
    })

    assert.equal(response.statusCode, 403)
    assert.equal(settingsQueried, false)
    assert.equal(authHandlerCalled, false)
  })

  it('rejects disallowed CORS auth requests before querying guard settings', async (t) => {
    let settingsQueried = false
    let authHandlerCalled = false
    const app = createAuthApp(
      createTestDeps({
        getAuthGuardSettings: async () => {
          settingsQueried = true
          return {
            registrationEnabled: true,
            windowSeconds: 60,
            maxRequests: 30,
          }
        },
        authHandler: async (_req, res) => {
          authHandlerCalled = true
          res.statusCode = 204
          res.end()
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/api/auth/sign-in/email',
      headers: {
        origin: 'https://evil.example.com',
      },
      payload: {
        email: 'alice@example.com',
        password: 'secret123',
      },
    })

    assert.equal(response.statusCode, 403)
    assert.equal(settingsQueried, false)
    assert.equal(authHandlerCalled, false)
  })

  it('allows auth requests without an Origin header through the guard', async (t) => {
    let settingsQueried = false
    let authHandlerCalled = false
    const app = createAuthApp(
      createTestDeps({
        getAuthGuardSettings: async () => {
          settingsQueried = true
          return {
            registrationEnabled: true,
            windowSeconds: 60,
            maxRequests: 30,
          }
        },
        authHandler: async (_req, res) => {
          authHandlerCalled = true
          res.statusCode = 204
          res.end()
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/api/auth/sign-in/email',
      payload: {
        email: 'alice@example.com',
        password: 'secret123',
      },
    })

    assert.equal(response.statusCode, 204)
    assert.equal(settingsQueried, true)
    assert.equal(authHandlerCalled, true)
  })

  for (const url of ['/api/auth/sign-in/email', '/api/auth/sign-up/email']) {
    it(`rate-limits ${url} before calling the auth handler again`, async (t) => {
      let authHandlerCalls = 0
      const app = createAuthApp(
        createTestDeps({
          getAuthGuardSettings: async () => ({
            registrationEnabled: true,
            windowSeconds: 60,
            maxRequests: 1,
          }),
          authHandler: async (_req, res) => {
            authHandlerCalls += 1
            res.statusCode = 204
            res.end()
          },
        }),
      )
      t.after(() => app.close())

      const firstResponse = await app.inject({
        method: 'POST',
        url,
        payload: {
          email: 'alice@example.com',
          password: 'secret123',
        },
      })
      const secondResponse = await app.inject({
        method: 'POST',
        url,
        payload: {
          email: 'alice@example.com',
          password: 'secret123',
        },
      })

      assert.equal(firstResponse.statusCode, url.includes('/sign-up/') ? 404 : 204)
      assert.equal(secondResponse.statusCode, 429)
      assert.ok(Number(secondResponse.headers['retry-after']) > 0)
      assert.deepEqual(secondResponse.json(), {
        code: 429,
        msg: '请求过于频繁，请稍后重试',
      })
      assert.equal(authHandlerCalls, url.includes('/sign-up/') ? 0 : 1)
    })
  }

  it('binds an internal ensure request and returns command data', async (t) => {
    const inputs: unknown[] = []
    const app = createAuthApp(createTestDeps({
      internalCommands: {
        ensureIdentity: async (input) => {
          inputs.push(input)
          return {
            ok: true,
            data: {
              user: {
                auth_user_id: 'auth-admin',
                username: 'admin',
                email: 'admin@users.ideaboard.local',
                claim_identity: 'admin@users.ideaboard.local',
                claim_identity_kind: 'trusted_canonical_registration',
                claim_identity_verified: true,
                role: 'admin',
                existing: false,
              },
            },
          }
        },
      },
    }))
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST', url: '/internal/users/ensure', headers: { 'x-internal-key': 'test-internal-key' },
      payload: { username: 'Admin', password: 'secret123', role: 'admin', allow_existing: true },
    })

    assert.equal(response.statusCode, 200)
    assert.deepEqual(inputs, [{
      username: 'Admin',
      password: 'secret123',
      role: 'admin',
      allowExisting: true,
    }])
    assert.equal(response.json().data.user.auth_user_id, 'auth-admin')
  })

  it('translates every command failure kind through the shared HTTP result adapter', async () => {
    const cases = [
      ['invalid_input', 400],
      ['not_found', 404],
      ['conflict', 409],
      ['repository_failure', 500],
    ] as const

    for (const [kind, status] of cases) {
      const app = createAuthApp(createTestDeps({
        internalCommands: {
          ensureIdentity: async () => ({
            ok: false,
            kind,
            message: `${kind} failure`,
          }),
        },
      }))
      const response = await app.inject({
        method: 'POST',
        url: '/internal/users/ensure',
        headers: { 'x-internal-key': 'test-internal-key' },
        payload: {
          username: 'admin', password: 'secret123', role: 'admin', allow_existing: true,
        },
      })
      await app.close()

      assert.equal(response.statusCode, status)
      assert.deepEqual(response.json(), {
        code: status,
        msg: `${kind} failure`,
      })
    }
  })

  it('updates auth user role through the internal route adapter', async (t) => {
    const inputs: unknown[] = []
    const app = createAuthApp(
      createTestDeps({
        internalCommands: {
          setRole: async (input) => {
            inputs.push(input)
            return {
              ok: true,
              data: { auth_user_id: 'auth-user-1', role: 'admin' },
            }
          },
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'PUT',
      url: '/internal/users/auth-user-1/role',
      headers: {
        'x-internal-key': 'test-internal-key',
      },
      payload: {
        role: 'ADMIN',
      },
    })

    assert.equal(response.statusCode, 200)
    assert.deepEqual(response.json(), {
      code: 0,
      msg: 'success',
      data: {
        auth_user_id: 'auth-user-1',
        role: 'admin',
      },
    })
    assert.deepEqual(inputs, [{ authUserID: 'auth-user-1', role: 'ADMIN' }])
  })
})
