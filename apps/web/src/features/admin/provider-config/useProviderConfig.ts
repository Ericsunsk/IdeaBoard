import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminProviderConfigFeedback } from './useProviderConfigFeedback'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { createAdminProviderConfigWorkspace } from './workspace'
import type { AdminProviderConfigResource } from './resource'
import { createProviderEndpointEditor } from './providerEndpointEditor'
import { createAccountEditor } from './accountEditor'

import { inject, provide, type InjectionKey } from 'vue'

export type { ProviderEndpointEditor } from './providerEndpointEditor'
export type { AccountEditor } from './accountEditor'
export type AdminProviderConfigContext = ReturnType<typeof useProviderConfig>
export type AdminProviderConfigCommands = AdminProviderConfigContext['commands']

const ADMIN_PROVIDER_CONFIG_KEY: InjectionKey<AdminProviderConfigContext> = Symbol('AdminProviderConfigContext')

export function provideAdminProviderConfig(context: AdminProviderConfigContext) {
  provide(ADMIN_PROVIDER_CONFIG_KEY, context)
}

export function useAdminProviderConfigContext(): AdminProviderConfigContext {
  const context = inject(ADMIN_PROVIDER_CONFIG_KEY)
  if (!context) {
    throw new Error('useAdminProviderConfigContext must be used within provideAdminProviderConfig')
  }
  return context
}

export type UseAdminProviderConfigOptions = {
  feedback: AdminProviderConfigFeedback
  confirm: AdminConfirmPort
  resource?: AdminProviderConfigResource
}

export function useProviderConfig(options: UseAdminProviderConfigOptions) {
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
