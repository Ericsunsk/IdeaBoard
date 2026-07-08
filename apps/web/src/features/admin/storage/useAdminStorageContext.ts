import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import type { AdminStorageResource } from './storageResource'
import { createAdminStorageWorkspace, type StorageProfileCommandTarget } from './storageWorkspace'
import { useAdminStorageFeedback, type AdminStorageFeedback } from './useAdminStorageFeedback'
import { useStorageProfileEditor } from './useStorageProfileEditor'

export type UseAdminStorageContextOptions = {
  confirm: AdminConfirmPort
  feedback?: AdminStorageFeedback
  resource?: AdminStorageResource
}

export function useAdminStorageContext(options: UseAdminStorageContextOptions) {
  const { runAsyncAction } = useAsyncAction()
  const storageFeedback = options.feedback ?? useAdminStorageFeedback()
  const workspace = createAdminStorageWorkspace({
    feedback: storageFeedback,
    confirm: options.confirm,
    runAsyncAction,
    resource: options.resource,
  })
  const editor = useStorageProfileEditor({
    creatableProviders: workspace.creatableProviders,
    providerMap: workspace.providerMap,
    storages: workspace.storages,
    saveProfile: workspace.saveProfile,
    applyProfile: workspace.applyProfile,
    testProfile: workspace.testProfile,
    deleteProfile: workspace.deleteProfile,
    fetchBucketDomains: workspace.fetchBucketDomains,
  })

  function requestRemoveProfile(target: StorageProfileCommandTarget) {
    workspace.requestRemoveProfile(target)
  }

  return {
    ...editor,
    cards: workspace.cards,
    loading: workspace.loading,
    storages: workspace.storages,
    creatableProviders: workspace.creatableProviders,
    applyingId: workspace.applyingId,
    testingId: workspace.testingId,
    applyStorage: workspace.applyStorage,
    handleTest: workspace.testStorage,
    requestRemoveProfile,
    loadData: workspace.loadData,
    refreshData: workspace.refreshData,
  }
}
