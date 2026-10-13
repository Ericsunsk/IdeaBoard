import { useAsyncAction } from '../../../../../core/composables/useAsyncAction'
import type { AdminConfirmPort } from '../../../shared/index'
import type { AdminStorageResource } from './resource'
import { createAdminStorageWorkspace } from './workspace'
import { useStorageFeedback, type AdminStorageFeedback } from './useStorageFeedback'
import { useStorageProfileEditor } from './useStorageProfileEditor'

export type UseAdminStorageContextOptions = {
  confirm: AdminConfirmPort
  feedback?: AdminStorageFeedback
  resource?: AdminStorageResource
}

export function useStorageContext(options: UseAdminStorageContextOptions) {
  const { runAsyncAction } = useAsyncAction()
  const storageFeedback = options.feedback ?? useStorageFeedback()
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
    fetchBucketDomains: workspace.fetchBucketDomains,
  })

  return {
    // Editor-owned: form/draft, saving, bucket-domain detection.
    saving: editor.saving,
    fetching: editor.fetching,
    editorOpen: editor.editorOpen,
    editorMode: editor.editorMode,
    bucketDomains: editor.bucketDomains,
    selectedVendorKey: editor.selectedVendorKey,
    form: editor.form,
    selectedProviderMeta: editor.selectedProviderMeta,
    textFields: editor.textFields,
    switchFields: editor.switchFields,
    activeVendorPreset: editor.activeVendorPreset,
    vendorSelectOptions: editor.vendorSelectOptions,
    configTextValue: editor.configTextValue,
    configBoolValue: editor.configBoolValue,
    updateConfigValue: editor.updateConfigValue,
    providerLabel: editor.providerLabel,
    handleVendorSelect: editor.handleVendorSelect,
    openCreate: editor.openCreate,
    openEdit: editor.openEdit,
    closeEditor: editor.closeEditor,
    saveDraft: editor.saveDraft,
    fetchBucketDomains: editor.fetchBucketDomains,
    // Workspace-owned: apply/test/delete/enable orchestration.
    cards: workspace.cards,
    loading: workspace.isLoading,
    storages: workspace.storages,
    creatableProviders: workspace.creatableProviders,
    applyingId: workspace.applyingId,
    testingId: workspace.testingId,
    toggle: workspace.toggle,
    toggleEnabled: workspace.toggleEnabled,
    applyStorage: workspace.applyStorage,
    handleTest: workspace.testStorage,
    requestRemoveProfile: workspace.requestRemoveProfile,
    loadData: workspace.loadData,
    refreshData: workspace.refreshData,
  }
}
