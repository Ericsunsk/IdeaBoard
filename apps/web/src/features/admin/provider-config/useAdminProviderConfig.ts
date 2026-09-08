import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminProviderConfigFeedback } from './useAdminProviderConfigFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { createAdminProviderConfigWorkspace } from './providerConfigWorkspace'
import type { AdminProviderConfigResource } from './providerConfigResource'
import { createProviderEndpointEditor } from './providerEndpointEditor'
import { createAccountEditor } from './accountEditor'

export type { ProviderEndpointEditor } from './providerEndpointEditor'
export type { AccountEditor } from './accountEditor'

export type UseAdminProviderConfigOptions = {
  feedback: AdminProviderConfigFeedback
  confirm: AdminConfirmPort
  resource?: AdminProviderConfigResource
}

export function useAdminProviderConfig(options: UseAdminProviderConfigOptions) {
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminProviderConfigWorkspace({
    feedback: options.feedback,
    confirm: options.confirm,
    runAsyncAction,
    resource: options.resource,
  })

  const providerEndpointEditor = createProviderEndpointEditor({
    store: workspace,
    feedback: options.feedback,
  })
  const accountEditor = createAccountEditor({
    store: workspace,
    feedback: options.feedback,
  })

  return {
    cards: workspace.cards,
    providerEndpointEditor,
    accountEditor,
    commands: {
      loadData: workspace.loadData,
      deleteProviderEndpoint: workspace.deleteProviderEndpoint,
      deleteAccount: workspace.deleteAccount,
      toggleProviderEndpointStatus: workspace.toggleProviderEndpointStatus,
      toggleAccountModelStatus: workspace.toggleAccountModelStatus,
      isAccountModelStatusChanging: workspace.isAccountModelStatusChanging,
      getApiTypeLabel: workspace.getApiTypeLabel,
      getAccountModelRows: workspace.getAccountModelRows,
      getAccountStatusDotClass: workspace.getAccountStatusDotClass,
      getAccountProgressPt: workspace.getAccountProgressPt,
    },
  }
}
