import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { AuthAppConfig, AuthAppDependencies } from './auth-app.js'
import { createAuthApp } from './auth-app.js'
import type { AuthUserRepository, InternalAuthAPI } from './internal-routes.js'

type TestOverrides = Omit<Partial<AuthAppDependencies>, 'authApi' | 'config' | 'repository'> & {
  authApi?: Partial<InternalAuthAPI>
  config?: Partial<AuthAppConfig>
  repository?: Partial<AuthUserRepository>
}

function createTestDeps(overrides: TestOverrides = {}): AuthAppDependencies {
  const config: AuthAppConfig = {
    trustProxy: false,
    internalKey: 'test-internal-key',
    allowInsecureInternalBypass: false,
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
})
