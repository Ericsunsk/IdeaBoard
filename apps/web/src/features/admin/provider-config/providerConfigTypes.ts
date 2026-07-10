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

export type ProviderEndpointFormData = {
  apiType: ProviderApiTypeValue
  name: string
  baseUrl: string
  status: number
}

export type ProviderEndpointFormField = keyof ProviderEndpointFormData

export type ProviderAccountFormField =
  'name' | 'secret' | 'modelValues' | 'maxConcurrent' | 'timeoutSeconds'

export type ProviderAccountForm = {
  providerEndpointId: number
  name: string
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

export type ProviderEndpointRecord = {
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
  providerEndpointId: number
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

export type ProviderEndpointCommandTarget = {
  kind: 'providerEndpoint'
  id: number
}

export type ProviderAccountCommandTarget = {
  kind: 'account'
  id: number
}

export type ProviderEndpointView = {
  id: number
  name: string
  baseUrl: string
  apiType: ProviderApiTypeValue
  apiTypeLabel: string
  status: number
  commandTarget: ProviderEndpointCommandTarget
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

export type ProviderEndpointCard = {
  providerEndpoint: ProviderEndpointView
  accounts: ProviderAccountView[]
  modelCount: number
}
