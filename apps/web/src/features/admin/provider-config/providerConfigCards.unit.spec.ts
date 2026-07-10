import { describe, expect, it } from 'vitest'
import type { ProviderAccountRecord, ProviderEndpointRecord } from './providerConfigTypes'
import {
  countUniqueAccountModels,
  createProviderEndpointCards,
  groupAccountsByProviderEndpointId,
} from './providerConfigCards'

describe('provider config cards', () => {
  it('groups accounts by providerEndpoint and builds card model counts', () => {
    const cards = createProviderEndpointCards(
      [makeProviderEndpoint({ id: 1, name: 'A' }), makeProviderEndpoint({ id: 2, name: 'B' })],
      [
        makeAccount({ id: 11, providerEndpointId: 1, models: [makeModel({ name: 'gpt-4o' })] }),
        makeAccount({
          id: 12,
          providerEndpointId: 1,
          models: [makeModel({ name: 'gpt-4o', enabled: false }), makeModel({ name: 'imagen' })],
        }),
        makeAccount({ id: 21, providerEndpointId: 2, models: [makeModel({ name: 'gemini' })] }),
      ],
    )

    expect(
      cards.map((card) => [
        card.providerEndpoint.id,
        card.providerEndpoint.baseUrl,
        card.providerEndpoint.apiTypeLabel,
        card.accounts.length,
        card.modelCount,
      ]),
    ).toEqual([
      [1, 'https://api.example.com/v1', 'OpenAI Compatible', 2, 2],
      [2, 'https://api.example.com/v1', 'OpenAI Compatible', 1, 1],
    ])
    expect(cards[0]?.accounts[0]).toMatchObject({
      id: 11,
      concurrencyLabel: '0/3',
      concurrencyPercent: 0,
      commandTarget: { kind: 'account', id: 11 },
    })
    expect(cards[0]?.providerEndpoint.commandTarget).toEqual({ kind: 'providerEndpoint', id: 1 })
  })

  it('returns grouped account map for direct lookup', () => {
    const grouped = groupAccountsByProviderEndpointId([
      makeAccount({ id: 11, providerEndpointId: 1 }),
      makeAccount({ id: 21, providerEndpointId: 2 }),
    ])

    expect(grouped.get(1)?.map((account) => account.id)).toEqual([11])
    expect(grouped.get(2)?.map((account) => account.id)).toEqual([21])
  })

  it('counts unique model names across all accounts', () => {
    expect(
      countUniqueAccountModels([
        makeAccount({ models: [makeModel({ name: 'gpt-4o' })] }),
        makeAccount({
          models: [makeModel({ name: 'gpt-4o', enabled: false }), makeModel({ name: 'gemini' })],
        }),
      ]),
    ).toBe(2)
  })
})

function makeProviderEndpoint(
  overrides: Partial<ProviderEndpointRecord> = {},
): ProviderEndpointRecord {
  return {
    id: 1,
    apiType: 'openai-compatible',
    name: 'ProviderEndpoint',
    baseUrl: 'https://api.example.com/v1',
    status: 1,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

function makeAccount(overrides: Partial<ProviderAccountRecord> = {}): ProviderAccountRecord {
  return {
    id: 11,
    name: 'Account',
    providerEndpointId: 1,
    apiType: 'openai-compatible',
    baseUrl: 'https://api.example.com/v1',
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

function makeModel(overrides: Partial<ProviderAccountRecord['models'][number]> = {}) {
  return {
    name: 'model',
    enabled: true,
    weight: 1,
    maxConcurrent: undefined,
    featureSlugs: undefined,
    sortOrder: 0,
    ...overrides,
  }
}
