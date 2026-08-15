import type {
  AccountModelRow,
  ProviderAccountRecord,
  ProviderAccountView,
  ProviderEndpointCard,
  ProviderEndpointRecord,
  ProviderEndpointView,
} from './providerConfigTypes'
import {
  getAdminProviderEndpointApiTypeLabel,
  getAdminProviderEndpointProtocolLabel,
} from './providerConfigPayload'

export function groupAccountsByProviderEndpointId(accounts: ProviderAccountRecord[]) {
  const grouped = new Map<number, ProviderAccountRecord[]>()
  accounts.forEach((account) => {
    const group = grouped.get(account.providerEndpointId)
    if (group) {
      group.push(account)
      return
    }
    grouped.set(account.providerEndpointId, [account])
  })
  return grouped
}

export function createAccountModelRows(account: ProviderAccountView): AccountModelRow[] {
  return account.models.map((model) => ({
    name: model.name,
    disabled: !model.enabled,
    supports2K: model.supports2K,
    supports4K: model.supports4K,
  }))
}

export function createProviderEndpointCards(
  providerEndpoints: ProviderEndpointRecord[],
  accounts: ProviderAccountRecord[],
): ProviderEndpointCard[] {
  const groupedAccounts = groupAccountsByProviderEndpointId(accounts)

  return providerEndpoints.map((providerEndpoint) => {
    const providerEndpointAccounts = groupedAccounts.get(providerEndpoint.id) || []
    const models = new Set<string>()
    providerEndpointAccounts.forEach((account) => {
      account.models.forEach((model) => models.add(model.name))
    })

    return {
      providerEndpoint: createProviderEndpointView(providerEndpoint),
      accounts: providerEndpointAccounts.map(createProviderAccountView),
      modelCount: models.size,
    }
  })
}

export function countUniqueAccountModels(accounts: ProviderAccountRecord[]) {
  const models = new Set<string>()
  accounts.forEach((account) => {
    account.models.forEach((model) => models.add(model.name))
  })
  return models.size
}

export function createProviderEndpointView(
  providerEndpoint: ProviderEndpointRecord,
): ProviderEndpointView {
  return {
    id: providerEndpoint.id,
    name: providerEndpoint.name,
    baseUrl: providerEndpoint.baseUrl,
    apiType: providerEndpoint.apiType,
    protocol: providerEndpoint.protocol,
    capabilities: providerEndpoint.capabilities,
    apiTypeLabel: getAdminProviderEndpointApiTypeLabel(providerEndpoint.apiType),
    protocolLabel: getAdminProviderEndpointProtocolLabel(providerEndpoint.protocol),
    apiKeyConfigured: providerEndpoint.apiKeyConfigured,
    status: providerEndpoint.status,
    commandTarget: {
      kind: 'providerEndpoint',
      id: providerEndpoint.id,
    },
  }
}

export function createProviderAccountView(account: ProviderAccountRecord): ProviderAccountView {
  const maxConcurrent = account.maxConcurrent || 1
  const currentConcurrent = account.currentConcurrent || 0

  return {
    id: account.id,
    name: account.name,
    models: account.models.map((model) => ({
      name: model.name,
      enabled: model.enabled,
      supports2K: model.supports2K,
      supports4K: model.supports4K,
    })),
    maxConcurrent,
    currentConcurrent,
    concurrencyLabel: `${currentConcurrent}/${maxConcurrent}`,
    concurrencyPercent: (currentConcurrent / maxConcurrent) * 100,
    isEnabled: account.status === 1,
    isDegraded: account.failCount >= 5,
    commandTarget: {
      kind: 'account',
      id: account.id,
    },
  }
}
