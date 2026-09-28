import type { AuthEnvironment } from './env.js'
import { AUTH_CLEANUP_INTERVAL_MS } from './authCleanup.js'

export type AuthRuntimeResources = {
  verifyPrivileges(): Promise<void>
  initializeProviders(): Promise<void>
  listen(): Promise<void>
  closeServer(): Promise<void>
  cleanupUnverifiedUsers(): Promise<unknown>
}

export type AuthRuntimeFactories = {
  createDatabase(config: AuthEnvironment['database']): unknown
  closeDatabase(database: unknown): Promise<void>
  createResources(input: { environment: AuthEnvironment; database: unknown }): AuthRuntimeResources
  scheduleCleanup(run: () => void, intervalMs: number): () => void
}

export type AuthRuntime = {
  start(): Promise<void>
  close(): Promise<void>
}

export function createAuthRuntime(
  environment: AuthEnvironment,
  factories: AuthRuntimeFactories,
): AuthRuntime {
  let database: unknown
  let hasDatabase = false
  let resources: AuthRuntimeResources | undefined
  let cancelCleanup: (() => void) | undefined
  let startPromise: Promise<void> | undefined
  let closePromise: Promise<void> | undefined
  let closeRequested = false
  let resourcesReleased = false

  async function startInternal(): Promise<void> {
    try {
      assertStartupAllowed()
      database = factories.createDatabase(environment.database)
      hasDatabase = true
      const createdResources = factories.createResources({
        environment,
        database,
      })
      resources = createdResources
      await createdResources.verifyPrivileges()
      assertStartupAllowed()
      await createdResources.initializeProviders()
      assertStartupAllowed()
      await createdResources.listen()
      assertStartupAllowed()
      cancelCleanup = factories.scheduleCleanup(() => {
        void createdResources.cleanupUnverifiedUsers().catch((error) => {
          console.error('[auth] Failed to clean up unverified users', error)
        })
      }, AUTH_CLEANUP_INTERVAL_MS)
    } catch (error) {
      try {
        await releaseResources()
      } catch (cleanupError) {
        throw new AggregateError([error, cleanupError], 'Auth Runtime failed to start and clean up')
      }
      throw error
    }
  }

  async function closeInternal(): Promise<void> {
    closeRequested = true
    if (startPromise) {
      try {
        await startPromise
      } catch {
        // startInternal owns cleanup and preserves the startup error.
      }
    }
    await releaseResources()
  }

  async function releaseResources(): Promise<void> {
    if (resourcesReleased) return
    resourcesReleased = true
    const errors: unknown[] = []
    if (cancelCleanup) {
      try {
        cancelCleanup()
      } catch (error) {
        errors.push(error)
      } finally {
        cancelCleanup = undefined
      }
    }
    if (resources) {
      try {
        await resources.closeServer()
      } catch (error) {
        errors.push(error)
      } finally {
        resources = undefined
      }
    }
    if (hasDatabase) {
      try {
        await factories.closeDatabase(database)
      } catch (error) {
        errors.push(error)
      } finally {
        database = undefined
        hasDatabase = false
      }
    }
    if (errors.length > 0) {
      throw new AggregateError(errors, 'Auth Runtime failed to close')
    }
  }

  function assertStartupAllowed(): void {
    if (closeRequested || resourcesReleased) {
      throw new Error('Auth Runtime shutdown requested during startup')
    }
  }

  return {
    start() {
      startPromise ??= startInternal()
      return startPromise
    },
    close() {
      closePromise ??= closeInternal()
      return closePromise
    },
  }
}
