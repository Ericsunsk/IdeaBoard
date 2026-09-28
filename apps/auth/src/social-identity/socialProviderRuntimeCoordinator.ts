import type { BetterAuthOptions } from 'better-auth'
import { createRuntimeAuthRegistry, type BetterAuthFactory } from './runtimeAuthRegistry.js'
import type { RuntimeSocialProviderConfigs, SupportedSocialProvider } from './catalog.js'

export function createSocialProviderRuntimeCoordinator(factory: BetterAuthFactory) {
  let configs: RuntimeSocialProviderConfigs = {}
  let registry: ReturnType<typeof createRuntimeAuthRegistry> | undefined

  return {
    configure(baseOptions: BetterAuthOptions) {
      if (registry) throw new Error('Social Provider runtime is already configured')
      registry = createRuntimeAuthRegistry(factory, { baseOptions })
      return registry
    },
    isReady(provider: SupportedSocialProvider): boolean {
      return Boolean(configs[provider])
    },
    apply(nextConfigs: RuntimeSocialProviderConfigs): void {
      if (!registry) throw new Error('Social Provider runtime is not configured')
      configs = { ...nextConfigs }
      registry.apply(configs)
    },
  }
}
