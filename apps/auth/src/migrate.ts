import dotenv from 'dotenv'
import { runProductionAuthMigrations } from './application/authMigrationProduction.js'

// Same ENV_FILE precedence as src/index.ts (see note there).
if (process.env.ENV_FILE?.trim()) {
  dotenv.config({ path: process.env.ENV_FILE.trim(), quiet: true })
}
dotenv.config({ quiet: true })

runProductionAuthMigrations(process.env).catch((error) => {
  console.error(error instanceof Error ? error.message : error)
  process.exitCode = 1
})
