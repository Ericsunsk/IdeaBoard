import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient } from '@tanstack/vue-query'
import { computed, ref, toRaw, watch } from 'vue'
import type { Ref } from 'vue'
import type { AdminOutboundPolicyFeedback } from './useSettingsFeedback'
import {
  DEFAULT_OUTBOUND_POLICY_FORM,
  normalizeOutboundPolicyForm,
  outboundPolicyFormToRequest,
  validateOutboundPolicyForm,
  type OutboundPolicyForm,
  type OutboundPolicySetting,
} from './outboundPolicyContract'
import { outboundPolicyQueryKey, useOutboundPolicyResource } from './outboundPolicyResource'

export function useOutboundPolicy(options: { feedback: AdminOutboundPolicyFeedback }) {
  const queryClient = useQueryClient()
  const resource = useOutboundPolicyResource()
  const setting = computed(() => resource.query.data.value)
  const form = ref<OutboundPolicyForm>(
    cloneForm(DEFAULT_OUTBOUND_POLICY_FORM),
  ) as Ref<OutboundPolicyForm>
  const lastSyncedForm = ref<OutboundPolicyForm>(
    cloneForm(DEFAULT_OUTBOUND_POLICY_FORM),
  ) as Ref<OutboundPolicyForm>
  const validationError = ref('')
  const loading = computed(() => resource.query.isLoading.value || resource.query.isFetching.value)
  const saveMutation = useMutation({
    mutationFn: (draft: OutboundPolicyForm) => resource.update(outboundPolicyFormToRequest(draft)),
    onSuccess: async (savedSetting) => {
      hydrate(savedSetting)
      writeOutboundPolicyQueryCache(queryClient, savedSetting)
      await queryClient.invalidateQueries({ queryKey: outboundPolicyQueryKey })
      if (resource.query.data.value) hydrate(resource.query.data.value)
    },
  })
  const saving = computed(() => saveMutation.isPending.value)
  const isDirty = computed(() => !isSameForm(form.value, lastSyncedForm.value))

  function hydrate(savedSetting: Partial<OutboundPolicySetting> | null | undefined) {
    const nextForm = normalizeOutboundPolicyForm(savedSetting)
    lastSyncedForm.value = cloneForm(nextForm)
    form.value = cloneForm(nextForm)
    validationError.value = ''
  }

  async function save(): Promise<boolean> {
    const validationMessage = validateOutboundPolicyForm(form.value)
    if (validationMessage) {
      validationError.value = validationMessage
      options.feedback.validationFailed(validationMessage)
      return false
    }

    try {
      await saveMutation.mutateAsync(normalizeOutboundPolicyFormValue(form.value))
      options.feedback.saved()
      return true
    } catch (error) {
      options.feedback.saveFailed(error)
      return false
    }
  }

  function reset() {
    form.value = cloneForm(lastSyncedForm.value)
    validationError.value = ''
  }

  watch(
    form,
    () => {
      validationError.value = ''
    },
    { deep: true, flush: 'sync' },
  )

  watch(
    () => resource.query.error.value,
    (error) => {
      if (error) options.feedback.loadFailed(error)
    },
  )

  watch(
    setting,
    (savedSetting) => {
      if (!savedSetting || saving.value || isDirty.value) return
      hydrate(savedSetting)
    },
    { immediate: true },
  )

  return {
    error: computed(() => resource.query.error.value),
    form,
    isDirty,
    loading,
    refetch: resource.query.refetch,
    reset,
    save,
    saving,
    setting,
    validationError,
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

function writeOutboundPolicyQueryCache(queryClient: QueryClient, setting: OutboundPolicySetting) {
  queryClient.setQueryData(outboundPolicyQueryKey, {
    code: 0,
    msg: 'ok',
    data: setting,
  })
}

function cloneForm(form: OutboundPolicyForm): OutboundPolicyForm {
  return structuredClone(toRaw(form))
}
