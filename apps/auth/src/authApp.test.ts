import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { AuthAppConfig, AuthAppDependencies } from './authApp.js'
import { createAuthApp } from './authApp.js'
import type { AuthUserRepository, InternalAuthAPI } from './internalRoutes.js'

type TestOverrides = Omit<Partial<AuthAppDependencies>, 'authApi' | 'config' | 'repository'> & {
  authApi?: Partial<InternalAuthAPI>
  config?: Partial<AuthAppConfig>
  repository?: Partial<AuthUserRepository>
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
  const repository: AuthUserRepository = {
    findUserByUsernameOrEmail: async () => null,
    finalizeTrustedRegistrationIdentity: async () => true,
    updateAuthUserRole: async () => true,
    updateAuthUserBanState: async () => true,
    deleteAuthUserSessions: async () => true,
    removeAuthUserCascade: async () => true,
    ...overrides.repository,
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

    assert.equal(response.statusCode, 400)
    assert.deepEqual(removed, ['auth-empty-payload'])
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

  it('ensures a new internal admin with an explicit trusted claim identity contract', async (t) => {
    const marked: Array<[string, string]> = []
    const app = createAuthApp(createTestDeps({ repository: {
      finalizeTrustedRegistrationIdentity: async (authUserID, claimIdentity) => {
        marked.push([authUserID, claimIdentity])
        return true
      },
    } }))
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST', url: '/internal/users/ensure', headers: { 'x-internal-key': 'test-internal-key' },
      payload: { username: 'Admin', password: 'secret123', role: 'admin', allow_existing: true },
    })

    assert.equal(response.statusCode, 200)
    assert.deepEqual(marked, [['auth-user-2', 'admin@users.ideaboard.local']])
    assert.equal(response.json().data.user.claim_identity_verified, true)
  })

  it('accepts an existing internal admin only when its trusted identity and role already match', async (t) => {
    const marked: Array<[string, string]> = []
    const app = createAuthApp(createTestDeps({
      authApi: { signUpEmail: async () => { throw new Error('user already exists') } },
      repository: {
        findUserByUsernameOrEmail: async () => ({ id: 'auth-admin-existing', username: 'admin', email: 'admin@users.ideaboard.local', role: 'admin', emailVerified: true }),
        finalizeTrustedRegistrationIdentity: async (authUserID, claimIdentity) => {
          marked.push([authUserID, claimIdentity])
          return true
        },
      },
    }))
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST', url: '/internal/users/ensure', headers: { 'x-internal-key': 'test-internal-key' },
      payload: { username: 'admin', password: 'secret123', role: 'admin', allow_existing: true },
    })

    assert.equal(response.statusCode, 200)
    assert.deepEqual(marked, [])
    assert.equal(response.json().data.user.existing, true)
  })

  it('rejects an attacker-preclaimed admin username without promoting the existing user', async (t) => {
    let finalized = false
    const app = createAuthApp(createTestDeps({
      authApi: { signUpEmail: async () => { throw new Error('user already exists') } },
      repository: {
        findUserByUsernameOrEmail: async () => ({
          id: 'auth-attacker', username: 'admin', email: 'admin@users.ideaboard.local', role: 'user', emailVerified: true,
        }),
        finalizeTrustedRegistrationIdentity: async () => {
          finalized = true
          return true
        },
      },
    }))
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST', url: '/internal/users/ensure', headers: { 'x-internal-key': 'test-internal-key' },
      payload: { username: 'admin', password: 'unknown-password', role: 'admin', allow_existing: true },
    })

    assert.equal(response.statusCode, 409)
    assert.equal(finalized, false)
  })

  it('updates auth user role through the internal route adapter', async (t) => {
    const updates: Array<{ authUserID: string; role: 'admin' | 'user' }> = []
    const app = createAuthApp(
      createTestDeps({
        repository: {
          updateAuthUserRole: async (authUserID, role) => {
            updates.push({ authUserID, role })
            return true
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
    assert.deepEqual(updates, [{ authUserID: 'auth-user-1', role: 'admin' }])
  })
})
