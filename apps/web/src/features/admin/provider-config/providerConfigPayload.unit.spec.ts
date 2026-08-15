import { describe, expect, it } from 'vitest'
import type {
  ProviderAccountModelRecord,
  ProviderAccountRecord,
  ProviderEndpointRecord,
} from './providerConfigTypes'
import {
  DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
  accountModelInputsFromValues,
  createAccountCreatePayload,
  createAccountUpdatePayload,
  inferProviderEndpointApiTypeFromUrl,
  isModellessProviderEndpoint,
} from './providerConfigPayload'

describe('provider config payload mapping', () => {
  it('maps new account form values to the backend create payload defaults', () => {
    const payload = createAccountCreatePayload({
      form: {
        providerEndpointId: 7,
        name: 'Google APIKey A',
        modelValues: ['gemini-2.5-flash'],
        modelCapabilities: {},
        secret: 'sk-test',
        maxConcurrent: 4,
        timeoutSeconds: 90,
      },
      providerEndpoint: makeProviderEndpoint(),
      modelValues: ['gemini-2.5-flash', ' gemini-2.5-flash '],
      secret: 'sk-test',
    })

    expect(payload).toMatchObject({
      provider_endpoint_id: 7,
      name: 'Google APIKey A',
      secret: 'sk-test',
      max_concurrent: 4,
      timeout: 90,
      extra_config: DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
    })
    expect(payload.models).toEqual([
      {
        name: 'gemini-2.5-flash',
        enabled: true,
        weight: 1,
        supports_2k: false,
        supports_4k: false,
      },
    ])
  })

  it('maps edited account form values while preserving account backend-only fields', () => {
    const payload = createAccountUpdatePayload({
      form: {
        providerEndpointId: 7,
        name: 'Google APIKey B',
        modelValues: ['gemini-2.5-flash', 'imagen-4'],
        modelCapabilities: {},
        secret: '',
        maxConcurrent: 8,
        timeoutSeconds: 180,
      },
      providerEndpoint: makeProviderEndpoint(),
      existingAccount: makeAccount({
        weight: 250,
        extraConfig: '{"region":"asia"}',
        models: [
          makeModel({
            name: 'gemini-2.5-flash',
            enabled: false,
            weight: 3,
          }),
        ],
      }),
      modelValues: ['gemini-2.5-flash', 'imagen-4'],
    })

    expect(payload).toMatchObject({
      name: 'Google APIKey B',
      max_concurrent: 8,
      timeout: 180,
      weight: 250,
      extra_config: '{"region":"asia"}',
    })
    expect(payload).not.toHaveProperty('secret')
    expect(payload.models).toEqual([
      {
        name: 'gemini-2.5-flash',
        enabled: false,
        weight: 3,
        max_concurrent: undefined,
        supports_2k: false,
        supports_4k: false,
      },
      {
        name: 'imagen-4',
        enabled: true,
        weight: 1,
        max_concurrent: undefined,
        supports_2k: false,
        supports_4k: false,
      },
    ])
    if (!payload.models) {
      throw new Error('account update payload must include models')
    }
    expect(payload.models.every((model) => !Object.hasOwn(model, 'feature_slugs'))).toBe(true)
  })

  it('dedupes model names case-insensitively and keeps existing model settings', () => {
    const models = accountModelInputsFromValues(
      [' GPT-4o ', 'gpt-4o', 'Gemini'],
      [makeModel({ name: 'gpt-4o', enabled: false, weight: 9, maxConcurrent: 2 })],
    )

    expect(models).toEqual([
      {
        name: 'GPT-4o',
        enabled: false,
        weight: 9,
        max_concurrent: 2,
        supports_2k: false,
        supports_4k: false,
      },
      {
        name: 'Gemini',
        enabled: true,
        weight: 1,
        max_concurrent: undefined,
        supports_2k: false,
        supports_4k: false,
      },
    ])
    expect(models.every((model) => !Object.hasOwn(model, 'feature_slugs'))).toBe(true)
  })

  it('infers known provider api types from Base URL', () => {
    expect(
      inferProviderEndpointApiTypeFromUrl('https://generativelanguage.googleapis.com/v1beta'),
    ).toBe('google-ai')
    expect(inferProviderEndpointApiTypeFromUrl('https://aiplatform.googleapis.com')).toBe(
      'google-cloud',
    )
    expect(inferProviderEndpointApiTypeFromUrl('https://api.openai.com/v1')).toBe(
      'openai-compatible',
    )
    expect(inferProviderEndpointApiTypeFromUrl('https://example.com/custom', 'google-ai')).toBe(
      'google-ai',
    )
  })

  it('identifies capability endpoints that do not require a model', () => {
    expect(
      isModellessProviderEndpoint(
        makeProviderEndpoint({
          protocol: 'image-url-json-to-image',
          capabilities: ['background-removal'],
        }),
      ),
    ).toBe(true)
    expect(
      isModellessProviderEndpoint(
        makeProviderEndpoint({
          protocol: 'image-url-json-to-image',
          capabilities: ['image-upscale'],
        }),
      ),
    ).toBe(true)
    expect(isModellessProviderEndpoint(makeProviderEndpoint())).toBe(false)
  })
})

function makeProviderEndpoint(
  overrides: Partial<ProviderEndpointRecord> = {},
): ProviderEndpointRecord {
  return {
    id: 7,
    apiType: 'google-ai',
    protocol: 'model-api',
    capabilities: ['image-generation'],
    name: 'Google ProviderEndpoint',
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta',
    status: 1,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

function makeAccount(overrides: Partial<ProviderAccountRecord> = {}): ProviderAccountRecord {
  return {
    id: 3,
    name: 'Account',
    providerEndpointId: 7,
    apiType: 'google-ai',
    protocol: 'model-api',
    capabilities: ['image-generation'],
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta',
    models: [],
    secretConfigured: true,
    maxConcurrent: 3,
    currentConcurrent: 0,
    waitQueueCount: 0,
    runningStatus: '',
    failCount: 0,
    weight: 100,
    timeoutSeconds: 120,
    extraConfig: '{}',
    status: 1,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

function makeModel(
  overrides: Partial<ProviderAccountModelRecord> = {},
): ProviderAccountModelRecord {
  return {
    name: 'model',
    enabled: true,
    weight: 1,
    maxConcurrent: undefined,
    sortOrder: 0,
    supports2K: false,
    supports4K: false,
    ...overrides,
  }
}
