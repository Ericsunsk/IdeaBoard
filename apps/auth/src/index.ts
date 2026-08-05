import { toNodeHandler } from 'better-auth/node'
import { auth, authDB, mailConfigService } from './auth.js'
import { createAuthApp } from './authApp.js'
import { getAuthGuardSettings } from './authGuardSettings.js'
import { createAuthInternalCommands, type InternalAuthAPI } from './authInternalCommands.js'
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
import { AUTH_CLEANUP_INTERVAL_MS, cleanupUnverifiedUsers } from './authCleanup.js'

const authApi: InternalAuthAPI = {
  getSession: (input) => auth.api.getSession(input),
  signUpEmail: (input) => auth.api.signUpEmail(input),
}

const repository = {
  findUserByUsernameOrEmail,
  finalizeTrustedRegistrationIdentity,
  updateAuthUserRole,
  updateAuthUserBanState,
  deleteAuthUserSessions,
  removeAuthUserCascade,
}

const app = createAuthApp({
  config: {
    trustProxy: env.auth.trustProxy,
    internalKey: env.auth.internalKey,
    trustedOrigins: env.auth.trustedOrigins,
  },
  healthCheck: async () => {
    await authDB.query('SELECT 1')
  },
  authHandler: toNodeHandler(auth),
  authApi,
  repository,
  internalCommands: createAuthInternalCommands(authApi, repository),
  getAuthGuardSettings,
  mailConfigService,
})

const cleanupTimer = setInterval(() => {
  void cleanupUnverifiedUsers(authDB).catch((error) => {
    console.error('[auth] Failed to clean up unverified users', error)
  })
}, AUTH_CLEANUP_INTERVAL_MS)
cleanupTimer.unref()

try {
  await verifyAuthRuntimePrivileges(authDB)
  await app.listen({ port: env.auth.port, host: env.auth.host })
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
    .then(() => {
      clearInterval(cleanupTimer)
      return authDB.end()
    })
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
