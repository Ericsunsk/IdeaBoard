import { toNodeHandler } from 'better-auth/node'
import { auth, authDB } from './auth.js'
import { createAuthApp } from './authApp.js'
import { getAuthGuardSettings } from './authGuardSettings.js'
import {
  deleteAuthUserSessions,
  findUserByUsernameOrEmail,
  finalizeTrustedRegistrationIdentity,
  removeAuthUserCascade,
  updateAuthUserBanState,
  updateAuthUserRole,
} from './authRepository.js'
import { env } from './env.js'
import { verifyAuthRuntimePrivileges } from './runtime-privileges.js'

const app = createAuthApp({
  config: {
    trustProxy: env.auth.trustProxy,
    internalKey: env.auth.internalKey,
    allowInsecureInternalBypass: env.auth.allowInsecureInternalBypass,
    trustedOrigins: env.auth.trustedOrigins,
  },
  healthCheck: async () => {
    await authDB.query('SELECT 1')
  },
  authHandler: toNodeHandler(auth),
  authApi: {
    getSession: (input) => auth.api.getSession(input),
    signUpEmail: (input) => auth.api.signUpEmail(input),
  },
  repository: {
    findUserByUsernameOrEmail,
    finalizeTrustedRegistrationIdentity,
    updateAuthUserRole,
    updateAuthUserBanState,
    deleteAuthUserSessions,
    removeAuthUserCascade,
  },
  getAuthGuardSettings,
})

try {
  await verifyAuthRuntimePrivileges(authDB)
  await app.listen({ port: env.auth.port, host: env.auth.host })
  if (!env.auth.internalKey) {
    console.warn(
      '[auth] AUTH_INTERNAL_KEY is empty because AUTH_INSECURE_DEV=1 and AUTH_HOST is loopback; internal endpoints are not protected',
    )
  }
  console.log(`[auth] Better Auth service listening on http://${env.auth.host}:${env.auth.port}`)
} catch (error) {
  console.error('[auth] Failed to start auth service', error)
  await authDB.end()
  process.exit(1)
}

function gracefulShutdown(signal: string) {
  console.log(`[auth] ${signal} received, shutting down gracefully...`)
  // Force exit after 10s if graceful close hangs
  setTimeout(() => {
    console.error('[auth] Graceful shutdown timed out, forcing exit')
    process.exit(1)
  }, 10_000).unref()

  app
    .close()
    .then(() => authDB.end())
    .then(() => {
      console.log('[auth] Database pool closed')
      process.exit(0)
    })
    .catch((err) => {
      console.error('[auth] Error closing auth service', err)
      process.exit(1)
    })
}

process.on('SIGTERM', () => gracefulShutdown('SIGTERM'))
process.on('SIGINT', () => gracefulShutdown('SIGINT'))
