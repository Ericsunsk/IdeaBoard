import {
  EXTERNAL_IMAGE_PROVIDERS,
  EXTERNAL_IMAGE_PROVIDER_LABELS,
  type ExternalImageProvider,
} from '../../../../../core/contracts/externalImageProviders'

export type SearchProviderId = ExternalImageProvider

export type SearchProviderTestState = 'never' | 'passed' | 'failed'

export interface SearchProviderConfig {
  provider: SearchProviderId
  api_key_configured: boolean
  endpoint_url?: string
  model?: string
  enabled: boolean
  test_state: SearchProviderTestState
  failure_category?: string
  status_message?: string
  tested_at?: string | null
}

export interface SearchProviderList {
  providers: SearchProviderConfig[]
}

export interface SearchProviderUpdate {
  api_key?: string
  enabled: boolean
}

export interface SearchProviderTestResult {
  provider: SearchProviderId
  test_state: SearchProviderTestState
  failure_category?: string
  message: string
}

export const SEARCH_PROVIDERS: readonly SearchProviderId[] = EXTERNAL_IMAGE_PROVIDERS

export const SEARCH_PROVIDER_LABELS: Record<SearchProviderId, string> =
  EXTERNAL_IMAGE_PROVIDER_LABELS
