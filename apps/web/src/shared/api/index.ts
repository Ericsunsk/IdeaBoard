// Re-export aggregation layer — no API implementations in this file.
export { default } from './client'
export type { ApiClientError } from './client'

export * from './user'
export * from './admin'
export * from './task'
export * from './monitor'
export * from './upload'
export * from './featurePrompt'
export * from './channel'
export * from './board'
export * from './adminStatistics'
export type {
  AdminStatisticsBackfillResult,
  AdminStatisticsBackfillState,
  AdminStatisticsResponse,
} from './adminStatistics'

export * from './storage'
