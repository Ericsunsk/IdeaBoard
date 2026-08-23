import {
  createAdminAccountRecord,
  deleteAdminAccountRecord,
  deleteAdminProviderEndpoint,
  detectAdminProviderEndpointModels,
  fetchAdminProviderEndpointRegistry,
  fetchAdminAccountLimits,
  fetchAdminProviderEndpoints,
  fetchAllAdminAccounts,
  patchAdminAccountModelRecord,
  saveAdminProviderEndpoint,
  setAdminProviderEndpointStatus,
  updateAdminAccountRecord,
} from './providerConfigApi'

export type AdminProviderConfigResource = {
  fetchProviderEndpoints: typeof fetchAdminProviderEndpoints
  fetchAccounts: typeof fetchAllAdminAccounts
  fetchAccountLimits: typeof fetchAdminAccountLimits
  saveProviderEndpoint: typeof saveAdminProviderEndpoint
  setProviderEndpointStatus: typeof setAdminProviderEndpointStatus
  deleteProviderEndpoint: typeof deleteAdminProviderEndpoint
  detectProviderEndpointModels: typeof detectAdminProviderEndpointModels
  fetchProviderEndpointRegistry?: typeof fetchAdminProviderEndpointRegistry
  createAccountRecord: typeof createAdminAccountRecord
  updateAccountRecord: typeof updateAdminAccountRecord
  patchAccountModelRecord: typeof patchAdminAccountModelRecord
  deleteAccountRecord: typeof deleteAdminAccountRecord
}

export function createAdminProviderConfigResource(): AdminProviderConfigResource {
  return {
    fetchProviderEndpoints: fetchAdminProviderEndpoints,
    fetchAccounts: fetchAllAdminAccounts,
    fetchAccountLimits: fetchAdminAccountLimits,
    saveProviderEndpoint: saveAdminProviderEndpoint,
    setProviderEndpointStatus: setAdminProviderEndpointStatus,
    deleteProviderEndpoint: deleteAdminProviderEndpoint,
    detectProviderEndpointModels: detectAdminProviderEndpointModels,
    fetchProviderEndpointRegistry: fetchAdminProviderEndpointRegistry,
    createAccountRecord: createAdminAccountRecord,
    updateAccountRecord: updateAdminAccountRecord,
    patchAccountModelRecord: patchAdminAccountModelRecord,
    deleteAccountRecord: deleteAdminAccountRecord,
  }
}
