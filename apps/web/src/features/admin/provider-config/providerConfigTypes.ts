export type ProviderApiTypeValue = 'openai-compatible' | 'google-ai'

export type AdminProviderAccountLimitRange = {
  min: number
  max: number
  default: number
}

export type AdminProviderAccountLimits = {
  maxConcurrent: AdminProviderAccountLimitRange
  timeoutSeconds: AdminProviderAccountLimitRange
}

export type ProviderModelDetectionInput = {
  baseUrl: string
  apiKey: string
  apiType: ProviderApiTypeValue
}

export type ProviderChannelFormData = {
  apiType: ProviderApiTypeValue
  name: string
  baseUrl: string
}

export type ProviderChannelFormField = keyof ProviderChannelFormData

export type ProviderAccountFormField =
  | 'secret'
  | 'modelValues'
  | 'maxConcurrent'
  | 'timeoutSeconds'

export type ProviderAccountForm = {
  channelId: number
  modelValues: string[]
  secret: string
  maxConcurrent: number
  timeoutSeconds: number
}

export type ProviderAccountModelView = {
  name: string
  enabled: boolean
}

export type ProviderAccountModelRecord = {
  name: string
  enabled: boolean
  weight: number
  maxConcurrent?: number
  featureSlugs?: string[]
  sortOrder: number
}

export type ProviderChannelRecord = {
  id: number
  apiType: ProviderApiTypeValue
  name: string
  baseUrl: string
  status: number
  createdAt: string
  updatedAt: string
}

export type ProviderAccountRecord = {
  id: number
  name: string
  channelId: number
  platform: string
  apiType: string
  baseUrl: string
  models: ProviderAccountModelRecord[]
  secretConfigured: boolean
  maxConcurrent: number
  currentConcurrent: number
  waitQueueCount: number
  runningStatus: string
  failCount: number
  weight: number
  timeoutSeconds: number
  extraConfig: string
  status: number
  createdAt: string
  updatedAt: string
}

export type AccountModelRow = {
  name: string
  disabled: boolean
}

export type ProviderChannelCommandTarget = {
  kind: 'channel'
  id: number
}

export type ProviderAccountCommandTarget = {
  kind: 'account'
  id: number
}

export type ProviderChannelView = {
  id: number
  name: string
  baseUrl: string
  apiType: ProviderApiTypeValue
  apiTypeLabel: string
  commandTarget: ProviderChannelCommandTarget
}

export type ProviderAccountView = {
  id: number
  name: string
  models: ProviderAccountModelView[]
  maxConcurrent: number
  currentConcurrent: number
  concurrencyLabel: string
  concurrencyPercent: number
  isEnabled: boolean
  isDegraded: boolean
  commandTarget: ProviderAccountCommandTarget
}

export type ProviderChannelCard = {
  channel: ProviderChannelView
  accounts: ProviderAccountView[]
  modelCount: number
}
