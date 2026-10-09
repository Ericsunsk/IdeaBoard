import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { AuthAppConfig, AuthAppDependencies } from './authApp.js'
import { createAuthApp } from './authApp.js'
import {
  type AuthIdentityMutationStore,
  type AuthSessionAPI,
  createAuthInternalCommands,
  type AuthInternalCommands,
} from '../trusted-identity/authInternalCommands.js'
import { authInternalRoutes } from '../generated/auth-internal/routes.gen.js'
import { createAuthAdmissionPolicy } from '../admission/authAdmissionPolicy.js'
import type { AuthGuardSettings } from '../admission/authGuardSettings.js'
import {
  createTrustedIdentityProvisioner,
  type TrustedIdentityAuthAPI,
  type TrustedIdentityRepository,
} from '../trusted-identity/trustedIdentity.js'
import type { MailConfigSummary } from '../transactional-mail/mailConfig.js'
import type { SocialProviderSettingsSummary } from '../social-identity/socialProviderSettings.js'

type TestAuthAPI = AuthSessionAPI & TrustedIdentityAuthAPI
type TestIdentityStore = AuthIdentityMutationStore & TrustedIdentityRepository

type TestOverrides = Omit<
  Partial<AuthAppDependencies>,
  'authApi' | 'config' | 'internalCommands'
> & {
  authApi?: Partial<TestAuthAPI>
  config?: Partial<AuthAppConfig>
  internalCommands?: Partial<AuthInternalCommands>
  repository?: Partial<TestIdentityStore>
  getAuthGuardSettings?: () => Promise<AuthGuardSettings>
}

function createTestDeps(overrides: TestOverrides = {}): AuthAppDependencies {
  const { getAuthGuardSettings, ...dependencyOverrides } = overrides
  const config: AuthAppConfig = {
    trustProxy: false,
    internalKey: 'test-internal-key',
    trustedOrigins: ['https://app.example.com'],
    ...overrides.config,
  }
  const authApi: TestAuthAPI = {
    getSession: async () => ({
      user: {
        id: 'auth-user-1',
        name: 'Alice',
        email: 'alice@example.com',
      },
    }),
    signUpEmail: async ({ body }) => ({
      user: {
        id: 'auth-user-2',
        name: body.name,
        email: body.email,
      },
    }),
    ...overrides.authApi,
  }
  const repository: TestIdentityStore = {
    findUserByEmail: async () => null,
    findUserEmailsByIDs: async (authUserIDs) =>
      authUserIDs.map((auth_user_id) => ({
        auth_user_id,
        email: `${auth_user_id}@example.com`,
        providers: ['email'],
      })),
    findUserStatesByIDs: async (authUserIDs) =>
      authUserIDs.map((auth_user_id) => ({
        auth_user_id,
        role: 'user',
        banned: false,
        ban_reason: null,
        ban_expires: null,
      })),
    finalizeTrustedRegistrationIdentity: async () => true,
    updateAuthUserRole: async () => true,
    updateAuthUserBanState: async () => true,
    deleteAuthUserSessions: async () => true,
    removeAuthUserCascade: async () => true,
    ...overrides.repository,
  }
  const internalCommands: AuthInternalCommands = {
    ...createAuthInternalCommands(
      createTrustedIdentityProvisioner(authApi, repository),
      repository,
    ),
    ...overrides.internalCommands,
  }

  const admissionPolicy =
    overrides.admissionPolicy ??
    createAuthAdmissionPolicy({
      trustedOrigins: config.trustedOrigins,
      getSettings:
        getAuthGuardSettings ??
        (async () => ({
          registrationEnabled: true,
          windowSeconds: 60,
          maxRequests: 30,
          lockoutThreshold: 10,
          lockoutDurationSeconds: 900,
        })),
      isMailReady: overrides.mailConfigService
        ? async () => (await overrides.mailConfigService!.getSummary()).readiness === 'ready'
        : undefined,
      getSocialProviderReadiness: overrides.socialProviderSettingsService
        ? async (provider) =>
            (await overrides.socialProviderSettingsService!.getCapabilities()).social[provider]
        : undefined,
    })
  const mailSummary: MailConfigSummary = {
    configured: true,
    apiKeyConfigured: true,
    enabled: true,
    fromName: 'IdeaBoard',
    fromEmail: 'auth@example.com',
    replyTo: '',
    configVersion: 1,
    readiness: 'ready',
    updatedAt: null,
    lastTestedAt: null,
    lastTestErrorCode: null,
  }
  const baseDeps: AuthAppDependencies = {
    config,
    healthCheck: async () => {},
    authHandler: async (_req, res) => {
      res.statusCode = 204
      res.end()
    },
    authApi,
    internalCommands,
    admissionPolicy,
    mailConfigService: {
      getSummary: async () => mailSummary,
      update: async () => mailSummary,
      clear: async () => {},
      sendTest: async () => {},
    },
    socialProviderSettingsService: {
      getSummary: async (
        provider: SocialProviderSettingsSummary['provider'],
      ): Promise<SocialProviderSettingsSummary> => ({
        provider,
        enabled: true,
        configured: true,
        available: true,
        clientId: 'client',
        clientSecretConfigured: true,
        scopes: [],
        configVersion: 1,
        callbackURL: 'https://app.example.com/callback',
        updatedAt: null,
      }),
      update: async (provider: SocialProviderSettingsSummary['provider']) => ({
        provider,
        enabled: true,
        configured: true,
        available: true,
        clientId: 'client',
        clientSecretConfigured: true,
        scopes: [],
        configVersion: 1,
        callbackURL: 'https://app.example.com/callback',
        updatedAt: null,
      }),
      getCapabilities: async () => ({ social: { google: true, discord: true } }),
    },
  }

  return {
    ...baseDeps,
    ...dependencyOverrides,
    config,
    authApi,
    internalCommands,
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

    const response = await app.inject({
      method: 'GET',
      url: '/internal/session',
    })

    assert.equal(response.statusCode, 401)
    assert.deepEqual(response.json(), {
      code: 401,
      msg: 'unauthorized internal request',
    })
    assert.equal(sessionChecked, false)
  })

  it('registers every operation from the generated internal contract', async (t) => {
    const app = createAuthApp(createTestDeps())
    t.after(() => app.close())
    const requests = {
      validateSession: {},
      ensureUser: {
        payload: {
          email: 'alice@example.com',
          display_name: 'Alice',
          password: 'secret1234',
          allow_existing: false,
          role: 'user',
        },
      },
      getUserEmails: { payload: { user_ids: ['auth-user-1'] } },
      getUserStates: { payload: { user_ids: ['auth-user-1'] } },
      deleteUser: {},
      setUserRole: { payload: { role: 'user' } },
      banUser: { payload: { userId: 'auth-user-1' } },
      unbanUser: { payload: { userId: 'auth-user-1' } },
      revokeUserSessions: { payload: { userId: 'auth-user-1' } },
      updateMailConfig: {
        payload: {
          from_name: 'IdeaBoard',
          from_email: 'auth@example.com',
          actor_id: 1,
        },
      },
      clearMailConfig: { payload: { actor_id: 1 } },
      sendMailConfigTest: { payload: { recipient: 'admin@example.com', actor_id: 1 } },
      updateSocialProvider: { payload: { enabled: true, client_id: 'client' } },
    } as const

    for (const operationID of Object.keys(authInternalRoutes) as Array<
      keyof typeof authInternalRoutes
    >) {
      const route = authInternalRoutes[operationID]
      const response = await app.inject({
        method: route.method,
        url: route.fastifyPath.replace(/:id|:provider/g, 'google'),
        headers: { 'x-internal-key': 'test-internal-key' },
        ...(requests[operationID as keyof typeof requests] ?? {}),
      })

      assert.equal(response.statusCode, 200, `${operationID}: ${response.body}`)
    }
  })

  it('returns trusted canonical identity state for Credit Wallet provisioning', async (t) => {
    const app = createAuthApp(
      createTestDeps({
        authApi: {
          getSession: async () => ({
            user: {
              id: 'auth-user-verified',
              name: 'Verified User',
              email: 'verified-user@example.com',
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

  it('blocks sign-up requests when registration is disabled', async (t) => {
    let authHandlerCalled = false
    const app = createAuthApp(
      createTestDeps({
        getAuthGuardSettings: async () => ({
          registrationEnabled: false,
          windowSeconds: 60,
          maxRequests: 30,
          lockoutThreshold: 10,
          lockoutDurationSeconds: 900,
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
        password: 'secret1234',
      },
    })

    assert.equal(response.statusCode, 403)
    assert.deepEqual(response.json(), {
      code: 403,
      msg: '注册已关闭',
    })
    assert.equal(authHandlerCalled, false)
  })

  it('keeps social runtime capabilities available when registration is disabled', async (t) => {
    let registrationEnabled = false
    const app = createAuthApp(
      createTestDeps({
        getAuthGuardSettings: async () => ({
          registrationEnabled,
          windowSeconds: 60,
          maxRequests: 30,
          lockoutThreshold: 10,
          lockoutDurationSeconds: 900,
        }),
        socialProviderSettingsService: {
          getSummary: async () => {
            throw new Error('unused')
          },
          update: async () => {
            throw new Error('unused')
          },
          getCapabilities: async () => ({ social: { google: true, discord: true } }),
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'GET',
      url: '/api/auth/capabilities',
    })

    assert.equal(response.statusCode, 200)
    assert.deepEqual(response.json().data, {
      social: { google: true, discord: true },
      providers: [
        { provider: 'google', label: 'Google', available: true },
        { provider: 'discord', label: 'Discord', available: true },
      ],
    })

    registrationEnabled = true
    const enabledResponse = await app.inject({
      method: 'GET',
      url: '/api/auth/capabilities',
    })
    assert.deepEqual(enabledResponse.json().data, {
      social: { google: true, discord: true },
      providers: [
        { provider: 'google', label: 'Google', available: true },
        { provider: 'discord', label: 'Discord', available: true },
      ],
    })
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
            lockoutThreshold: 10,
            lockoutDurationSeconds: 900,
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
    assert.equal(response.headers['access-control-allow-headers'], 'content-type, authorization')
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
            lockoutThreshold: 10,
            lockoutDurationSeconds: 900,
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
            lockoutThreshold: 10,
            lockoutDurationSeconds: 900,
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
        password: 'secret1234',
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
            lockoutThreshold: 10,
            lockoutDurationSeconds: 900,
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
        password: 'secret1234',
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
            lockoutThreshold: 10,
            lockoutDurationSeconds: 900,
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
          password: 'secret1234',
        },
      })
      const secondResponse = await app.inject({
        method: 'POST',
        url,
        payload: {
          email: 'alice@example.com',
          password: 'secret1234',
        },
      })

      assert.equal(firstResponse.statusCode, 204)
      assert.equal(secondResponse.statusCode, 429)
      assert.ok(Number(secondResponse.headers['retry-after']) > 0)
      assert.deepEqual(secondResponse.json(), {
        code: 429,
        msg: '请求过于频繁，请稍后重试',
      })
      assert.equal(authHandlerCalls, 1)
    })
  }

  it('binds an internal ensure request and returns command data', async (t) => {
    const inputs: unknown[] = []
    const app = createAuthApp(
      createTestDeps({
        internalCommands: {
          ensureIdentity: async (input) => {
            inputs.push(input)
            return {
              ok: true,
              data: {
                user: {
                  auth_user_id: 'auth-admin',
                  display_name: '管理员',
                  email: 'admin@example.com',
                  claim_identity: 'admin@example.com',
                  claim_identity_kind: 'canonical_email',
                  claim_identity_verified: true,
                  role: 'admin',
                  existing: false,
                },
              },
            }
          },
        },
      }),
    )
    t.after(() => app.close())

    const response = await app.inject({
      method: 'POST',
      url: '/internal/users/ensure',
      headers: { 'x-internal-key': 'test-internal-key' },
      payload: {
        email: 'admin@example.com',
        display_name: '管理员',
        password: 'secret1234',
        role: 'admin',
        allow_existing: true,
      },
    })

    assert.equal(response.statusCode, 200)
    assert.deepEqual(inputs, [
      {
        email: 'admin@example.com',
        displayName: '管理员',
        password: 'secret1234',
        role: 'admin',
        allowExisting: true,
      },
    ])
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
      const app = createAuthApp(
        createTestDeps({
          internalCommands: {
            ensureIdentity: async () => ({
              ok: false,
              kind,
              message: `${kind} failure`,
            }),
          },
        }),
      )
      const response = await app.inject({
        method: 'POST',
        url: '/internal/users/ensure',
        headers: { 'x-internal-key': 'test-internal-key' },
        payload: {
          email: 'admin@example.com',
          display_name: '管理员',
          password: 'secret1234',
          role: 'admin',
          allow_existing: true,
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
