import {
  isSupportedSocialProvider,
  socialProviderDefinition,
  socialProviderLabel,
  socialProviderScopes,
  supportedSocialProviders,
  type RuntimeSocialProviderConfig,
  type RuntimeSocialProviderConfigs,
  type SupportedSocialProvider,
} from './catalog.js'

export const GOOGLE_PROVIDER = 'google' as const
export const DISCORD_PROVIDER = 'discord' as const

export type {
  RuntimeSocialProviderConfig,
  RuntimeSocialProviderConfigs,
  SupportedSocialProvider,
} from './catalog.js'

export type StoredSocialProviderSettings = {
  provider: SupportedSocialProvider
  enabled: boolean
  clientId: string
  encryptedClientSecret: string
  scopes: string[]
  configVersion: number
  updatedAt: Date
}

export type SocialProviderSettingsSummary = {
  provider: SupportedSocialProvider
  enabled: boolean
  configured: boolean
  available: boolean
  clientId: string
  clientSecretConfigured: boolean
  scopes: string[]
  configVersion: number
  callbackURL: string
  updatedAt: string | null
}

export type SocialProviderSettingsInput = {
  enabled: boolean
  clientId: string
  clientSecret?: string
}

export type SocialProviderSettingsRepository = {
  get(provider: SupportedSocialProvider): Promise<StoredSocialProviderSettings | null>
  put(
    value: Omit<StoredSocialProviderSettings, 'configVersion' | 'updatedAt'>,
  ): Promise<StoredSocialProviderSettings>
}

export type RuntimeSocialProviderApply = (
  configs: RuntimeSocialProviderConfigs,
) => Promise<void> | void

export type SocialProviderSecretCipher = {
  encrypt(plaintext: string): string
  decrypt(encoded: string): string
}

export const SOCIAL_PROVIDER_SETTINGS_ENCRYPTION_KEY_ENV = 'APP_SECRETS_ENCRYPTION_KEY'

export class SocialProviderSettingsService {
  private updateBarrier: Promise<void> = Promise.resolve()

  constructor(
    private readonly repository: SocialProviderSettingsRepository,
    private readonly secretCipher: SocialProviderSecretCipher,
    private readonly baseURL: string,
    private readonly applyRuntimeConfig?: RuntimeSocialProviderApply,
  ) {}

  async initialize(): Promise<void> {
    await this.applyAllRuntimeConfigs()
  }

  async getSummary(providerValue: string): Promise<SocialProviderSettingsSummary> {
    const provider = requireSupportedProvider(providerValue)
    const stored = await this.repository.get(provider)
    if (!stored) return emptySummary(provider, this.callbackURL(provider))
    return summarize(
      stored,
      Boolean(await resolveStoredSettings(stored, this.secretCipher)),
      this.callbackURL(provider),
    )
  }

  async update(
    providerValue: string,
    input: SocialProviderSettingsInput,
  ): Promise<SocialProviderSettingsSummary> {
    const run = this.updateBarrier.then(() => this.updateSerialized(providerValue, input))
    this.updateBarrier = run.then(
      () => undefined,
      () => undefined,
    )
    return run
  }

  private async updateSerialized(
    providerValue: string,
    input: SocialProviderSettingsInput,
  ): Promise<SocialProviderSettingsSummary> {
    const provider = requireSupportedProvider(providerValue)
    const definition = socialProviderDefinition(provider)
    const current = await this.repository.get(provider)
    const clientId = input.clientId.trim()
    const nextClientSecret = input.clientSecret?.trim() ?? ''

    if (
      typeof input.enabled !== 'boolean' ||
      (clientId && !definition.validateClientId(clientId)) ||
      nextClientSecret.length > 2_048 ||
      /[\u0000-\u001f\u007f]/.test(nextClientSecret)
    ) {
      throw new SocialProviderSettingsError(
        'invalid_input',
        `${socialProviderLabel(provider)} OAuth Client ID 格式无效`,
      )
    }
    if (current?.enabled && current.clientId !== clientId) {
      throw new SocialProviderSettingsError(
        'disable_before_client_id_change',
        `请先关闭 ${socialProviderLabel(provider)} OAuth，再修改 Client ID`,
      )
    }

    let encryptedClientSecret = current?.encryptedClientSecret ?? ''
    if (nextClientSecret) {
      try {
        encryptedClientSecret = this.secretCipher.encrypt(nextClientSecret)
      } catch {
        throw new SocialProviderSettingsError(
          'encryption_unavailable',
          'OAuth Provider 配置加密密钥未配置',
        )
      }
    }
    if (input.enabled && (!clientId || !encryptedClientSecret)) {
      throw new SocialProviderSettingsError(
        'incomplete_configuration',
        `启用 ${providerLabel(provider)} OAuth 前必须配置 Client ID 和 Client Secret`,
      )
    }
    if (input.enabled) {
      try {
        if (!this.secretCipher.decrypt(encryptedClientSecret)) {
          throw new Error('empty secret')
        }
      } catch {
        throw new SocialProviderSettingsError(
          'encryption_unavailable',
          `${socialProviderLabel(provider)} OAuth Client Secret 不可用，请重新填写`,
        )
      }
    }

    const scopes = socialProviderScopes(provider)
    const changed =
      !current ||
      current.enabled !== input.enabled ||
      current.clientId !== clientId ||
      Boolean(nextClientSecret) ||
      !sameStrings(current.scopes, scopes)
    const saved = changed
      ? await this.repository.put({
          provider,
          enabled: input.enabled,
          clientId,
          encryptedClientSecret,
          scopes,
        })
      : current
    const resolved = await resolveStoredSettings(saved, this.secretCipher)
    if (changed) await this.applyAllRuntimeConfigs()
    return summarize(saved, Boolean(resolved), this.callbackURL(provider))
  }

  async resolve(providerValue: string): Promise<RuntimeSocialProviderConfig | null> {
    try {
      const provider = requireSupportedProvider(providerValue)
      const stored = await this.repository.get(provider)
      return stored ? await resolveStoredSettings(stored, this.secretCipher) : null
    } catch (error) {
      if (error instanceof SocialProviderSettingsError && error.code === 'unsupported_provider') {
        throw error
      }
      return null
    }
  }

  async getCapabilities(): Promise<{
    social: Record<string, boolean>
  }> {
    const resolved = await Promise.all(
      supportedSocialProviders.map(
        async (provider) => [provider, await this.resolve(provider)] as const,
      ),
    )
    const social = Object.fromEntries(
      resolved.map(([provider, config]) => [provider, Boolean(config)]),
    )
    return {
      social,
    }
  }

  private async applyAllRuntimeConfigs(): Promise<void> {
    const entries = await Promise.all(
      supportedSocialProviders.map(
        async (provider) => [provider, await this.resolve(provider)] as const,
      ),
    )
    const configs: RuntimeSocialProviderConfigs = {}
    for (const [provider, config] of entries) {
      if (config) configs[provider] = config
    }
    await this.applyRuntimeConfig?.(configs)
  }

  private callbackURL(provider: SupportedSocialProvider): string {
    return new URL(`/api/auth/callback/${provider}`, this.baseURL).toString()
  }
}

export class SocialProviderSettingsError extends Error {
  constructor(
    readonly code:
      | 'invalid_input'
      | 'unsupported_provider'
      | 'incomplete_configuration'
      | 'disable_before_client_id_change'
      | 'encryption_unavailable',
    message: string,
  ) {
    super(message)
    this.name = 'SocialProviderSettingsError'
  }
}

function requireSupportedProvider(value: string): SupportedSocialProvider {
  const provider = value.trim().toLowerCase()
  if (!isSupportedSocialProvider(provider)) {
    throw new SocialProviderSettingsError('unsupported_provider', '不支持的 OAuth Provider')
  }
  return provider
}

function providerLabel(provider: SupportedSocialProvider): string {
  return socialProviderLabel(provider)
}

async function resolveStoredSettings(
  stored: StoredSocialProviderSettings,
  secretCipher: SocialProviderSecretCipher,
): Promise<RuntimeSocialProviderConfig | null> {
  const definition = socialProviderDefinition(stored.provider)
  const expectedScopes = socialProviderScopes(stored.provider)
  if (
    !stored.enabled ||
    !definition.validateClientId(stored.clientId) ||
    !stored.encryptedClientSecret ||
    !sameStrings(stored.scopes, expectedScopes)
  ) {
    return null
  }
  try {
    const clientSecret = secretCipher.decrypt(stored.encryptedClientSecret)
    if (!clientSecret) return null
    return {
      provider: stored.provider,
      clientId: stored.clientId,
      clientSecret,
      scopes: expectedScopes,
    }
  } catch {
    return null
  }
}

function emptySummary(
  provider: SupportedSocialProvider,
  callbackURL: string,
): SocialProviderSettingsSummary {
  return {
    provider,
    enabled: false,
    configured: false,
    available: false,
    clientId: '',
    clientSecretConfigured: false,
    scopes: socialProviderScopes(provider),
    configVersion: 0,
    callbackURL,
    updatedAt: null,
  }
}

function summarize(
  stored: StoredSocialProviderSettings,
  available: boolean,
  callbackURL: string,
): SocialProviderSettingsSummary {
  return {
    provider: stored.provider,
    enabled: stored.enabled,
    configured: Boolean(stored.clientId && stored.encryptedClientSecret),
    available,
    clientId: stored.clientId,
    clientSecretConfigured: Boolean(stored.encryptedClientSecret),
    scopes: [...stored.scopes],
    configVersion: stored.configVersion,
    callbackURL,
    updatedAt: stored.updatedAt.toISOString(),
  }
}

function sameStrings(left: string[], right: string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index])
}
