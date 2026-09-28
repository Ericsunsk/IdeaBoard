import { toNodeHandler } from 'better-auth/node'
import type { Pool } from 'pg'
import { createAuthComponents } from './auth.js'
import { createAuthApp } from './authApp.js'
import { cleanupUnverifiedUsers } from '../infrastructure/authCleanup.js'
import {
  type AuthSessionAPI,
  createAuthInternalCommands,
} from '../trusted-identity/authInternalCommands.js'
import { createAuthIdentityStore } from '../infrastructure/authRepository.js'
import { verifyAuthRuntimePrivileges } from '../infrastructure/runtime-privileges.js'
import {
  createTrustedIdentityProvisioner,
  type TrustedIdentityAuthAPI,
} from '../trusted-identity/trustedIdentity.js'
import type { AuthEnvironment } from '../infrastructure/env.js'
import type { AuthRuntimeResources } from '../infrastructure/authRuntime.js'

export function createLiveAuthResources(
  environment: AuthEnvironment,
  database: Pool,
): AuthRuntimeResources {
  const components = createAuthComponents(environment, database)
  const identityStore = createAuthIdentityStore(database)
  const authSessionApi: AuthSessionAPI = {
    getSession: (input) => components.runtimeAuthRegistry.current.api.getSession(input),
  }
  const provisioningAuthApi: TrustedIdentityAuthAPI = {
    signUpEmail: (input) => components.runtimeAuthRegistry.current.api.signUpEmail(input),
  }
  const provisioner = createTrustedIdentityProvisioner(provisioningAuthApi, identityStore)
  const app = createAuthApp({
    config: {
      trustProxy: environment.auth.trustProxy,
      internalKey: environment.auth.internalKey,
      trustedOrigins: environment.auth.trustedOrigins,
    },
    healthCheck: async () => {
      await database.query('SELECT 1')
    },
    authHandler: (request, response) =>
      toNodeHandler(components.runtimeAuthRegistry.current)(request, response),
    authApi: authSessionApi,
    internalCommands: createAuthInternalCommands(provisioner, identityStore),
    admissionPolicy: components.authAdmissionPolicy,
    mailConfigService: components.mailConfigService,
    socialProviderSettingsService: components.socialProviderSettingsService,
  })

  return {
    verifyPrivileges: () => verifyAuthRuntimePrivileges(database),
    initializeProviders: () => components.socialProviderSettingsService.initialize(),
    async listen() {
      await app.listen({
        port: environment.auth.port,
        host: environment.auth.host,
      })
    },
    closeServer: () => app.close(),
    cleanupUnverifiedUsers: () => cleanupUnverifiedUsers(database),
  }
}
