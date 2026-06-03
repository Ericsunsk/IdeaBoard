import { getAdapter } from 'better-auth/db/adapter'
import { getMigrations } from 'better-auth/db/migration'
import { auth, authDB } from './auth.js'

async function runAuthMigrations() {
  const config = auth.options
  const db = await getAdapter(config)
  if (!db) {
    throw new Error('Invalid database configuration: Better Auth adapter could not be created')
  }
  if (db.id !== 'kysely') {
    throw new Error(`Unsupported Better Auth migration adapter: ${db.id}`)
  }

  const { toBeAdded, toBeCreated, runMigrations } = await getMigrations(config)
  if (!toBeAdded.length && !toBeCreated.length) {
    console.log('Better Auth migration: no changes needed')
    return
  }

  console.log(
    `Better Auth migration: applying ${toBeCreated.length} table changes and ${toBeAdded.length} field changes`,
  )
  await runMigrations()
  console.log('Better Auth migration: completed successfully')
}

runAuthMigrations()
  .catch((error) => {
    console.error(error instanceof Error ? error.message : error)
    process.exitCode = 1
  })
  .finally(() => {
    return authDB.end()
  })
