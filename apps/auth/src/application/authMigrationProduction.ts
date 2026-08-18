import { getAdapter } from "better-auth/db/adapter";
import { getMigrations } from "better-auth/db/migration";
import type { Pool } from "pg";
import {
  configureSocialProviderSettingsRuntimePrivileges,
  ensureSocialProviderSettingsSchema,
} from "../social-identity/socialProviderSchema.js";
import {
  configureMailConfigRuntimePrivileges,
  ensureMailConfigSchema,
} from "../transactional-mail/mailConfigSchema.js";
import {
  runAuthMigrations,
  type AuthMigrationDependencies,
} from "../infrastructure/authMigration.js";
import type { EnvSource } from "../infrastructure/env.js";
import { configureAuthRuntimePrivileges } from "../infrastructure/runtime-privileges.js";
import { createAuthPool, createStaticBetterAuthOptions } from "./auth.js";

const productionDependencies: AuthMigrationDependencies = {
  createDatabase(environment) {
    return createAuthPool(environment.database);
  },
  createOptions(environment, database) {
    return createStaticBetterAuthOptions(environment, database as Pool);
  },
  getAdapter,
  getMigrations,
  ensureMailConfigSchema(database) {
    return ensureMailConfigSchema(database as Pool);
  },
  ensureSocialProviderSettingsSchema(database) {
    return ensureSocialProviderSettingsSchema(database as Pool);
  },
  async configureRuntimePrivileges(database, runtimeUser) {
    const pool = database as Pool;
    await configureAuthRuntimePrivileges(pool, runtimeUser);
    await configureMailConfigRuntimePrivileges(pool, runtimeUser);
    await configureSocialProviderSettingsRuntimePrivileges(pool, runtimeUser);
  },
  log(message) {
    console.log(message);
  },
};

export function runProductionAuthMigrations(source: EnvSource): Promise<void> {
  return runAuthMigrations(source, productionDependencies);
}
