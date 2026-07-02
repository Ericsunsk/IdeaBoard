import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useAdminStorageFeedback } from './useAdminStorageFeedback'
import { useStorageProfileEditor } from './useStorageProfileEditor'
import { useStorageProfilesResource } from './useStorageProfilesResource'

export function useAdminStorageContext() {
  const { runAsyncAction } = useAsyncAction()
  const storageFeedback = useAdminStorageFeedback()
  const resource = useStorageProfilesResource()
  const editor = useStorageProfileEditor({
    creatableProviders: resource.creatableProviders,
    providerMap: resource.providerMap,
    storages: resource.storages,
    saveProfile: resource.saveProfile,
    applyProfile: resource.applyProfile,
    testProfile: resource.testProfile,
    deleteProfile: resource.deleteProfile,
    fetchBucketDomains: resource.fetchBucketDomains,
  })

  async function loadData() {
    await runAsyncAction(resource.loadData, {
      feedback: {
        error: storageFeedback.refreshFailed,
      },
    })
  }

  return {
    loading: resource.loading,
    storages: resource.storages,
    creatableProviders: resource.creatableProviders,
    loadData,
    ...editor,
  }
}
