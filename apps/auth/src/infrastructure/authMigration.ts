import type { BetterAuthOptions } from 'better-auth'
import { loadEnv, type AuthEnvironment, type EnvSource } from './env.js'

type MigrationDatabase = {
  end(): Promise<void>
}

type MigrationPlan = {
  toBeAdded: unknown[]
  toBeCreated: unknown[]
  runMigrations(): Promise<void>
}

export type AuthMigrationDependencies = {
  createDatabase(environment: AuthEnvironment): MigrationDatabase
  createOptions(environment: AuthEnvironment, database: MigrationDatabase): BetterAuthOptions
  getAdapter(options: BetterAuthOptions): Promise<{ id: string } | null>
  getMigrations(options: BetterAuthOptions): Promise<MigrationPlan>
  ensureAccountIssuerRemoved(database: MigrationDatabase): Promise<void>
  ensureMailConfigSchema(database: MigrationDatabase): Promise<void>
  ensureSocialProviderSettingsSchema(database: MigrationDatabase): Promise<void>
  configureRuntimePrivileges(database: MigrationDatabase, runtimeUser: string): Promise<void>
  log(message: string): void
}

export async function runAuthMigrations(
  source: EnvSource,
  dependencies: AuthMigrationDependencies,
): Promise<void> {
  const runtimeUser = requiredValue(source, 'AUTH_PG_USER')
  const migrationUser = requiredValue(source, 'PG_MIGRATION_USER')
  const migrationPassword = requiredValue(source, 'PG_MIGRATION_PASSWORD')
  if (runtimeUser === migrationUser) {
    throw new Error('PG_MIGRATION_USER must differ from AUTH_PG_USER')
  }

  const migrationEnvironment = loadEnv({
    ...source,
    AUTH_PG_USER: migrationUser,
    AUTH_PG_PASSWORD: migrationPassword,
  })
  const database = dependencies.createDatabase(migrationEnvironment)
  try {
    const options = dependencies.createOptions(migrationEnvironment, database)
    const adapter = await dependencies.getAdapter(options)
    if (!adapter) {
      throw new Error('Invalid database configuration: Better Auth adapter could not be created')
    }
    if (adapter.id !== 'kysely') {
      throw new Error(`Unsupported Better Auth migration adapter: ${adapter.id}`)
    }

    const plan = await dependencies.getMigrations(options)
    if (plan.toBeAdded.length || plan.toBeCreated.length) {
      dependencies.log(
        `Better Auth migration: applying ${plan.toBeCreated.length} table changes and ${plan.toBeAdded.length} field changes`,
      )
      await plan.runMigrations()
      dependencies.log('Better Auth migration: completed successfully')
    } else {
      dependencies.log('Better Auth migration: no changes needed')
    }

    await dependencies.ensureAccountIssuerRemoved(database)
    dependencies.log('Auth account issuer column removed (Better Auth 1.7 upgrade)')
    await dependencies.ensureMailConfigSchema(database)
    dependencies.log('Auth mail configuration schema ensured')
    await dependencies.ensureSocialProviderSettingsSchema(database)
    dependencies.log('Auth social provider settings schema ensured')
    await dependencies.configureRuntimePrivileges(database, runtimeUser)
    dependencies.log('Better Auth runtime privileges configured')
  } finally {
    await database.end()
  }
}

function requiredValue(source: EnvSource, name: string): string {
  const value = source[name]?.trim() || ''
  if (!value) throw new Error(`${name} is required`)
  return value
}
