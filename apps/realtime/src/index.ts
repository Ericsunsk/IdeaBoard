import dotenv from 'dotenv'
import { loadEnv } from './infrastructure/env.js'
import { createRealtimeServer } from './presence/server.js'

// Mirror apps/api config.go loadEnvFile precedence: an explicit ENV_FILE path
// loads first, then the default .env. dotenv never overrides real process env,
// so this only fills unset values and is safe to add without behavior change
// for fully-specified environments.
if (process.env.ENV_FILE?.trim()) {
  dotenv.config({ path: process.env.ENV_FILE.trim(), quiet: true })
}
dotenv.config({ quiet: true })

const environment = loadEnv(process.env)
const { httpServer, close } = createRealtimeServer({
  host: environment.realtime.host,
  port: environment.realtime.port,
  ticketSecret: environment.realtime.ticketSecret,
  internalKey: environment.realtime.internalKey,
  apiInternalURL: environment.realtime.apiInternalURL,
  compactEveryUpdates: environment.realtime.yCompactEvery,
  persistMaxRetries: environment.realtime.yPersistRetries,
})
let shutdownPromise: Promise<void> | undefined

process.once('SIGTERM', () => {
  void gracefulShutdown('SIGTERM')
})
process.once('SIGINT', () => {
  void gracefulShutdown('SIGINT')
})

try {
  await new Promise<void>((resolve, reject) => {
    httpServer.once('error', reject)
    httpServer.listen(environment.realtime.port, environment.realtime.host, () => {
      httpServer.off('error', reject)
      resolve()
    })
  })
  console.log(
    `[realtime] Presence sidecar listening on http://${environment.realtime.host}:${environment.realtime.port}`,
  )
} catch (error) {
  console.error('[realtime] Failed to start realtime service', error)
  process.exitCode = 1
}

function gracefulShutdown(signal: string): Promise<void> {
  shutdownPromise ??= shutdown(signal)
  return shutdownPromise
}

async function shutdown(signal: string): Promise<void> {
  console.log(`[realtime] ${signal} received, shutting down gracefully...`)
  const forceExitTimer = setTimeout(() => {
    console.error('[realtime] Graceful shutdown timed out, forcing exit')
    process.exit(1)
  }, 10_000)
  forceExitTimer.unref()

  try {
    await close()
    clearTimeout(forceExitTimer)
    console.log('[realtime] Realtime server closed')
    process.exit(0)
  } catch (error) {
    clearTimeout(forceExitTimer)
    console.error('[realtime] Error closing realtime service', error)
    process.exit(1)
  }
}
