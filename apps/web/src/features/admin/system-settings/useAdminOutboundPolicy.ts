import type { QueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import type { AdminOutboundPolicyFeedback } from './useAdminSystemSettingsFeedback'
import {
  DEFAULT_OUTBOUND_POLICY_FORM,
  normalizeOutboundPolicyForm,
  outboundPolicyFormToRequest,
  validateOutboundPolicyForm,
  type OutboundPolicyForm,
  type OutboundPolicySetting,
} from './outboundPolicyContract'
import { outboundPolicyQueryKey, useOutboundPolicyResource } from './outboundPolicyResource'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

export function useAdminOutboundPolicy(options: {
  feedback: AdminOutboundPolicyFeedback
}) {
  const resource = useOutboundPolicyResource()
  const setting = computed(() => resource.query.data.value)
  const source = computed(() => setting.value && normalizeOutboundPolicyForm(setting.value))
  let savedSetting: OutboundPolicySetting | null = null
  const lifecycle = useSettingResourceLifecycle<OutboundPolicyForm>({
    defaultSetting: DEFAULT_OUTBOUND_POLICY_FORM,
    feedback: options.feedback,
    isSame: isSameForm,
    normalize: normalizeOutboundPolicyFormValue,
    query: resource.query,
    queryKey: outboundPolicyQueryKey,
    save: async (form) => {
      savedSetting = await resource.update(outboundPolicyFormToRequest(form))
      return normalizeOutboundPolicyForm(savedSetting)
    },
    source,
    validate: validateOutboundPolicyForm,
    writeQueryCache: (queryClient) => writeOutboundPolicyQueryCache(queryClient, savedSetting),
  })

  return {
    error: computed(() => resource.query.error.value),
    form: lifecycle.form,
    loading: lifecycle.loading,
    refetch: resource.query.refetch,
    reset: lifecycle.reset,
    save: lifecycle.save,
    saving: lifecycle.saving,
    setting,
    validationError: lifecycle.validationError,
  }
}

function normalizeOutboundPolicyFormValue(
  form: Partial<OutboundPolicyForm> | null | undefined,
): OutboundPolicyForm {
  return {
    allow_hosts: form?.allow_hosts ?? '',
  }
}

function isSameForm(left: OutboundPolicyForm, right: OutboundPolicyForm): boolean {
  return left.allow_hosts === right.allow_hosts
}

function writeOutboundPolicyQueryCache(
  queryClient: QueryClient,
  setting: OutboundPolicySetting | null,
) {
  if (!setting) return
  queryClient.setQueryData(outboundPolicyQueryKey, {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}
