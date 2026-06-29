import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useStorageProfileEditor } from './useStorageProfileEditor'
import { useStorageProfilesResource } from './useStorageProfilesResource'

export function useAdminStorageContext() {
  const { runAsyncAction } = useAsyncAction()
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
      errorMessage: '刷新存储配置失败',
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
