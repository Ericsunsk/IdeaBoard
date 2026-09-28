import type { Pool } from 'pg'
import { createAuthPool } from './auth.js'
import { createLiveAuthResources } from './authRuntimeComposition.js'
import {
  createAuthRuntime,
  type AuthRuntime,
  type AuthRuntimeFactories,
} from '../infrastructure/authRuntime.js'
import type { AuthEnvironment } from '../infrastructure/env.js'

const productionAuthRuntimeFactories: AuthRuntimeFactories = {
  createDatabase: createAuthPool,
  async closeDatabase(database) {
    await (database as Pool).end()
  },
  createResources({ environment, database }) {
    return createLiveAuthResources(environment, database as Pool)
  },
  scheduleCleanup(run, intervalMs) {
    const timer = setInterval(run, intervalMs)
    timer.unref()
    return () => clearInterval(timer)
  },
}

export function createProductionAuthRuntime(environment: AuthEnvironment): AuthRuntime {
  return createAuthRuntime(environment, productionAuthRuntimeFactories)
}
