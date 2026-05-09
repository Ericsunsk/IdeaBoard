// Re-export aggregation layer — no API implementations in this file.
export { default } from './client'
export type { ApiClientError } from './client'

export * from './user'
export * from './admin'
export * from './task'
export * from './adminTask'
export * from './monitor'
export * from './upload'
export * from './vectorize'
export * from './featurePrompt'
export * from './channel'
export * from './board'

export {
  accountApi,
  backfillStatistics,
  createAccount,
  deleteAccount,
  getAdminStatistics,
  getAdminTask,
  getMonitor,
  getTask,
  listAccounts,
  listAdminTasks,
  listTasks,
  startAccount,
  stopAccount,
  submitAdminTask,
  submitTask,
  updateAccount,
} from './account'
export type {
  AccountListResponse,
  AdminStatisticsBackfillResult,
  AdminStatisticsBackfillState,
  AdminStatisticsResponse,
  ApiAccount,
  CreateAccountRequest,
  MonitorResponse,
  SubmitTaskRequest,
  TaskListResponse,
  UpdateAccountRequest,
} from './account'

// Storage (factory instance)
import api from './client'
import { createStorageApi } from './admin/storage'
export const storageApi = createStorageApi(api)
export type {
  StorageFieldType,
  StorageProfile,
  StorageProfilePayload,
  StorageProvider,
  StorageProviderField,
  StorageProviderMeta,
} from './admin/storage'
