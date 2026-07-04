import { describe, expect, it } from 'vitest'
import type {
  ProviderAccountModelRecord,
  ProviderAccountRecord,
  ProviderChannelRecord,
} from './providerConfigTypes'
import {
  DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
  accountModelInputsFromValues,
  createAccountCreatePayload,
  createAccountUpdatePayload,
  inferChannelApiTypeFromUrl,
} from './providerConfigPayload'

describe('provider config payload mapping', () => {
  it('maps new account form values to the backend create payload defaults', () => {
    const payload = createAccountCreatePayload({
      form: {
        channelId: 7,
        modelValues: ['gemini-2.5-flash'],
        secret: 'sk-test',
        maxConcurrent: 4,
        timeoutSeconds: 90,
      },
      channel: makeChannel(),
      modelValues: ['gemini-2.5-flash', ' gemini-2.5-flash '],
      secret: 'sk-test',
    })

    expect(payload).toMatchObject({
      channel_id: 7,
      name: 'Google Channel',
      platform: 'google-ai',
      api_type: 'google-ai',
      proxy_url: 'https://generativelanguage.googleapis.com/v1beta',
      secret: 'sk-test',
      max_concurrent: 4,
      timeout: 90,
      extra_config: DEFAULT_ADMIN_ACCOUNT_EXTRA_CONFIG,
    })
    expect(payload.models).toEqual([{ name: 'gemini-2.5-flash', enabled: true, weight: 1 }])
  })

  it('maps edited account form values while preserving account backend-only fields', () => {
    const payload = createAccountUpdatePayload({
      form: {
        channelId: 7,
        modelValues: ['gemini-2.5-flash', 'imagen-4'],
        secret: '',
        maxConcurrent: 8,
        timeoutSeconds: 180,
      },
      channel: makeChannel(),
      existingAccount: makeAccount({
        weight: 250,
        extraConfig: '{"region":"asia"}',
        models: [
          makeModel({
            name: 'gemini-2.5-flash',
            enabled: false,
            weight: 3,
            featureSlugs: ['image'],
          }),
        ],
      }),
      modelValues: ['gemini-2.5-flash', 'imagen-4'],
    })

    expect(payload).toMatchObject({
      name: 'Google Channel',
      platform: 'google-ai',
      api_type: 'google-ai',
      proxy_url: 'https://generativelanguage.googleapis.com/v1beta',
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
        feature_slugs: ['image'],
      },
      {
        name: 'imagen-4',
        enabled: true,
        weight: 1,
        max_concurrent: undefined,
        feature_slugs: undefined,
      },
    ])
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
        feature_slugs: undefined,
      },
      {
        name: 'Gemini',
        enabled: true,
        weight: 1,
        max_concurrent: undefined,
        feature_slugs: undefined,
      },
    ])
  })

  it('infers known provider api types from Base URL', () => {
    expect(inferChannelApiTypeFromUrl('https://generativelanguage.googleapis.com/v1beta')).toBe(
      'google-ai',
    )
    expect(inferChannelApiTypeFromUrl('https://api.openai.com/v1')).toBe('openai-compatible')
    expect(inferChannelApiTypeFromUrl('https://example.com/custom', 'google-ai')).toBe('google-ai')
  })
})

function makeChannel(overrides: Partial<ProviderChannelRecord> = {}): ProviderChannelRecord {
  return {
    id: 7,
    apiType: 'google-ai',
    name: 'Google Channel',
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
    channelId: 7,
    platform: 'google-ai',
    apiType: 'google-ai',
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
    featureSlugs: undefined,
    maxConcurrent: undefined,
    sortOrder: 0,
    ...overrides,
  }
}
