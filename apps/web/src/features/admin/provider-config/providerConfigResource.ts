import {
  createAdminAccountRecord,
  deleteAdminAccountRecord,
  deleteAdminProviderEndpoint,
  detectAdminProviderEndpointModels,
  fetchAdminAccountLimits,
  fetchAdminProviderEndpoints,
  fetchAllAdminAccounts,
  patchAdminAccountModelRecord,
  saveAdminProviderEndpoint,
  updateAdminAccountRecord,
} from './providerConfigApi'

export type AdminProviderConfigResource = {
  fetchProviderEndpoints: typeof fetchAdminProviderEndpoints
  fetchAccounts: typeof fetchAllAdminAccounts
  fetchAccountLimits: typeof fetchAdminAccountLimits
  saveProviderEndpoint: typeof saveAdminProviderEndpoint
  deleteProviderEndpoint: typeof deleteAdminProviderEndpoint
  detectProviderEndpointModels: typeof detectAdminProviderEndpointModels
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
    deleteProviderEndpoint: deleteAdminProviderEndpoint,
    detectProviderEndpointModels: detectAdminProviderEndpointModels,
    createAccountRecord: createAdminAccountRecord,
    updateAccountRecord: updateAdminAccountRecord,
    patchAccountModelRecord: patchAdminAccountModelRecord,
    deleteAccountRecord: deleteAdminAccountRecord,
  }
}
