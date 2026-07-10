import { getAdapter } from 'better-auth/db/adapter'
import { getMigrations } from 'better-auth/db/migration'
import { configureAuthRuntimePrivileges } from './runtime-privileges.js'

async function runAuthMigrations() {
  const runtimeUser = requiredEnv('DB_USER')
  const migrationUser = requiredEnv('DB_MIGRATION_USER')
  const migrationPassword = requiredEnv('DB_MIGRATION_PASSWORD')
  if (runtimeUser === migrationUser) {
    throw new Error('DB_MIGRATION_USER must differ from DB_USER')
  }

  // The migration process owns schema objects. The long-running Auth process
  // never receives these credentials.
  process.env.DB_USER = migrationUser;
  process.env.DB_PASSWORD = migrationPassword;
  const { auth, authDB } = await import('./auth.js')
  try {
    const config = auth.options
    const db = await getAdapter(config)
    if (!db) {
      throw new Error(
        'Invalid database configuration: Better Auth adapter could not be created',
      )
    }
    if (db.id !== "kysely") {
      throw new Error(`Unsupported Better Auth migration adapter: ${db.id}`)
    }

    const { toBeAdded, toBeCreated, runMigrations } =
      await getMigrations(config)
    if (toBeAdded.length || toBeCreated.length) {
      console.log(
        `Better Auth migration: applying ${toBeCreated.length} table changes and ${toBeAdded.length} field changes`,
      )
      await runMigrations()
      console.log('Better Auth migration: completed successfully')
    } else {
      console.log('Better Auth migration: no changes needed')
    }

    await configureAuthRuntimePrivileges(authDB, runtimeUser)
    console.log('Better Auth runtime privileges configured')
  } finally {
    await authDB.end()
  }
}

function requiredEnv(name: string): string {
  const value = process.env[name]?.trim() || ''
  if (!value) {
    throw new Error(`${name} is required`)
  }
  return value
}

runAuthMigrations().catch((error) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
