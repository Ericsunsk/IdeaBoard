import { toNodeHandler } from "better-auth/node";
import type { Pool } from "pg";
import { createAuthComponents, createAuthPool } from "./auth.js";
import { createAuthApp } from "./authApp.js";
import { cleanupUnverifiedUsers } from "./authCleanup.js";
import {
  type AuthSessionAPI,
  createAuthInternalCommands,
} from "./trusted-identity/authInternalCommands.js";
import { createAuthIdentityStore } from "./authRepository.js";
import {
  createAuthRuntime,
  type AuthRuntime,
  type AuthRuntimeFactories,
} from "./authRuntime.js";
import type { AuthEnvironment } from "./env.js";
import { verifyAuthRuntimePrivileges } from "./runtime-privileges.js";
import {
  createTrustedIdentityProvisioner,
  type TrustedIdentityAuthAPI,
} from "./trusted-identity/trustedIdentity.js";

const productionAuthRuntimeFactories: AuthRuntimeFactories = {
  createDatabase: createAuthPool,
  async closeDatabase(database) {
    await (database as Pool).end();
  },
  createResources({ environment, database }) {
    const authDatabase = database as Pool;
    const components = createAuthComponents(environment, authDatabase);
    const identityStore = createAuthIdentityStore(authDatabase);
    const authSessionApi: AuthSessionAPI = {
      getSession: (input) =>
        components.runtimeAuthRegistry.current.api.getSession(input),
    };
    const provisioningAuthApi: TrustedIdentityAuthAPI = {
      signUpEmail: (input) =>
        components.runtimeAuthRegistry.current.api.signUpEmail(input),
    };
    const provisioner = createTrustedIdentityProvisioner(
      provisioningAuthApi,
      identityStore,
    );
    const app = createAuthApp({
      config: {
        trustProxy: environment.auth.trustProxy,
        internalKey: environment.auth.internalKey,
        trustedOrigins: environment.auth.trustedOrigins,
      },
      healthCheck: async () => {
        await authDatabase.query("SELECT 1");
      },
      authHandler: (request, response) =>
        toNodeHandler(components.runtimeAuthRegistry.current)(request, response),
      authApi: authSessionApi,
      internalCommands: createAuthInternalCommands(provisioner, identityStore),
      admissionPolicy: components.authAdmissionPolicy,
      mailConfigService: components.mailConfigService,
      socialProviderSettingsService: components.socialProviderSettingsService,
    });

    return {
      verifyPrivileges: () => verifyAuthRuntimePrivileges(authDatabase),
      initializeProviders: () =>
        components.socialProviderSettingsService.initialize(),
      async listen() {
        await app.listen({
          port: environment.auth.port,
          host: environment.auth.host,
        });
      },
      async closeServer() {
        await app.close();
      },
      cleanupUnverifiedUsers: () => cleanupUnverifiedUsers(authDatabase),
    };
  },
  scheduleCleanup(run, intervalMs) {
    const timer = setInterval(run, intervalMs);
    timer.unref();
    return () => clearInterval(timer);
  },
};

export function createProductionAuthRuntime(
  environment: AuthEnvironment,
): AuthRuntime {
  return createAuthRuntime(environment, productionAuthRuntimeFactories);
}
