import assert from 'node:assert/strict'
import { test } from 'node:test'
import { defaultStatements } from 'better-auth/plugins/admin/access'
import { createAuthAdmissionPolicy } from '../admission/authAdmissionPolicy.js'
import { createBetterAuthOptions, createStaticBetterAuthOptions } from './auth.js'
import { loadEnv } from '../infrastructure/env.js'

type RoleShape = {
  authorize: (permissions: Record<string, string[]>) => { success: boolean } | undefined
}

test('native authentication security configuration stays explicit', async (t) => {
  void t
  const environment = loadEnv({
    NODE_ENV: 'test',
    AUTH_INTERNAL_KEY: 'test-internal-key',
    DB_SSLMODE: 'disable',
  })
  const admissionPolicy = createAuthAdmissionPolicy({
    trustedOrigins: environment.auth.trustedOrigins,
    getSettings: async () => ({
      registrationEnabled: false,
      windowSeconds: 60,
      maxRequests: 30,
    }),
  })
  const options = createBetterAuthOptions({
    environment,
    database: {} as never,
    mailConfigService: { send: async () => {} },
    authAdmissionPolicy: admissionPolicy,
  })
  assert.equal(options.emailAndPassword?.enabled, true)
  assert.equal(options.emailAndPassword?.minPasswordLength, 10)
  assert.equal(options.emailAndPassword?.maxPasswordLength, 128)
  assert.equal(options.emailAndPassword?.requireEmailVerification, true)
  assert.equal(options.emailVerification?.autoSignInAfterVerification, true)
  assert.equal(options.emailAndPassword?.sendResetPassword, undefined)
  assert.equal(options.emailVerification?.sendVerificationEmail, undefined)

  const plugins = options.plugins ?? []
  assert.equal(
    plugins.some((plugin) => plugin.id === 'username'),
    false,
  )
  const emailOTP = plugins.find((plugin) => plugin.id === 'email-otp') as
    | {
        id: string
        options?: {
          otpLength?: number
          expiresIn?: number
          allowedAttempts?: number
          storeOTP?: string
          resendStrategy?: string
          sendVerificationOnSignUp?: boolean
          changeEmail?: { enabled?: boolean }
          rateLimit?: { window?: number; max?: number }
        }
      }
    | undefined
  assert.ok(emailOTP)
  assert.equal(emailOTP.options?.otpLength, 6)
  assert.equal(emailOTP.options?.expiresIn, 600)
  assert.equal(emailOTP.options?.allowedAttempts, 5)
  assert.equal(emailOTP.options?.storeOTP, 'hashed')
  assert.equal(emailOTP.options?.resendStrategy, 'rotate')
  assert.equal(emailOTP.options?.sendVerificationOnSignUp, true)
  assert.equal(emailOTP.options?.changeEmail?.enabled, true)
  assert.deepEqual(emailOTP.options?.rateLimit, { window: 3600, max: 5 })

  assert.equal(
    plugins.some((plugin) => plugin.id === 'have-i-been-pwned'),
    false,
  )
})

test('static schema construction does not attach runtime provider configuration', () => {
  const environment = loadEnv({
    NODE_ENV: 'test',
    AUTH_INTERNAL_KEY: 'test-internal-key',
    DB_SSLMODE: 'disable',
  })
  const options = createStaticBetterAuthOptions(environment, {} as never)

  assert.equal(options.socialProviders, undefined)
  assert.deepEqual(options.database, {})
  assert.ok(options.plugins?.some((plugin) => plugin.id === 'email-otp'))
  assert.ok(options.plugins?.some((plugin) => plugin.id === 'admin'))
})

// Regression guard for the browser-reachable admin plugin surface. Every
// /api/auth/admin/* permission below must be denied to the admin role so that
// reaching it from a browser session can never mutate Auth state behind the Go
// Control Plane (DESIGN.md requires role changes and account deletion to be
// orchestrated by Go). `impersonate` is the single exception: the Go Control
// Plane starts impersonation through that endpoint on the internal network.
test('admin plugin permissions are narrowed to the one endpoint Go needs', () => {
  const environment = loadEnv({
    NODE_ENV: 'test',
    AUTH_INTERNAL_KEY: 'test-internal-key',
    DB_SSLMODE: 'disable',
  })
  const options = createBetterAuthOptions({
    environment,
    database: {} as never,
    mailConfigService: { send: async () => {} },
    authAdmissionPolicy: {
      admitRequest: async () => ({ allowed: true }),
      admitRegistration: async () => ({ allowed: true }),
    },
  })
  const admin = options.plugins?.find((plugin) => plugin.id === 'admin')
  assert.ok(admin, 'admin plugin must stay registered')
  const roles = (admin.options as { roles?: Record<string, RoleShape> }).roles
  assert.ok(roles?.admin, 'admin role must be explicitly redefined')
  assert.ok(roles?.user, 'user role must be explicitly redefined')

  // Mirrors better-auth's hasPermission(): the request is allowed when any
  // role on the session authorizes every requested permission.
  const authorized = (role: string, permissions: Record<string, string[]>) =>
    (role || 'user')
      .split(',')
      .some((name) => roles[name]?.authorize(permissions)?.success === true)

  assert.equal(authorized('admin', { user: ['impersonate'] }), true)

  for (const permission of defaultStatements.user) {
    if (permission === 'impersonate') continue
    assert.equal(
      authorized('admin', { user: [permission] }),
      false,
      `admin must not hold user:${permission}`,
    )
  }
  for (const permission of defaultStatements.session) {
    assert.equal(
      authorized('admin', { session: [permission] }),
      false,
      `admin must not hold session:${permission}`,
    )
  }

  // A non-admin role must not be able to start impersonation either.
  assert.equal(authorized('user', { user: ['impersonate'] }), false)
})
