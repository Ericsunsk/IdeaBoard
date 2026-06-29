import type {
  AccountModelRow,
  ProviderAccountRecord,
  ProviderAccountView,
  ProviderChannelCard,
  ProviderChannelRecord,
  ProviderChannelView,
} from './providerConfigTypes'
import { getAdminChannelApiTypeLabel } from './providerConfigPayload'

export function groupAccountsByChannelId(accounts: ProviderAccountRecord[]) {
  const grouped = new Map<number, ProviderAccountRecord[]>()
  accounts.forEach((account) => {
    const group = grouped.get(account.channelId)
    if (group) {
      group.push(account)
      return
    }
    grouped.set(account.channelId, [account])
  })
  return grouped
}

export function createAccountModelRows(account: ProviderAccountView): AccountModelRow[] {
  return account.models.map((model) => ({
    name: model.name,
    disabled: !model.enabled,
  }))
}

export function createProviderChannelCards(
  channels: ProviderChannelRecord[],
  accounts: ProviderAccountRecord[],
): ProviderChannelCard[] {
  const groupedAccounts = groupAccountsByChannelId(accounts)

  return channels.map((channel) => {
    const channelAccounts = groupedAccounts.get(channel.id) || []
    const models = new Set<string>()
    channelAccounts.forEach((account) => {
      account.models.forEach((model) => models.add(model.name))
    })

    return {
      channel: createProviderChannelView(channel),
      accounts: channelAccounts.map(createProviderAccountView),
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

export function createProviderChannelView(channel: ProviderChannelRecord): ProviderChannelView {
  return {
    id: channel.id,
    name: channel.name,
    baseUrl: channel.baseUrl,
    apiType: channel.apiType,
    apiTypeLabel: getAdminChannelApiTypeLabel(channel.apiType),
    commandTarget: {
      kind: 'channel',
      id: channel.id,
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
