import { computed, ref, watch } from 'vue'
import type { Ref } from 'vue'
import type { AdminOperationalSettingsFeedback } from '../feedback/useAdminFeedback'
import {
  DEFAULT_OPERATIONAL_SETTINGS_FORM,
  normalizeOperationalSettingsForm,
  operationalSettingsFormToRequest,
  validateOperationalSettingsForm,
  type OperationalEventRetentionSetting,
  type OperationalSettingsForm,
  type OperationalOutboundPolicySetting,
  type OperationalTaskRuntimeSetting,
} from './operationalSettingsContract'
import { useOperationalSettingsResource } from './operationalSettingsResource'

export function useAdminOperationalSettings(options: {
  feedback: AdminOperationalSettingsFeedback
}) {
  const resource = useOperationalSettingsResource()
  const form = ref(cloneForm(DEFAULT_OPERATIONAL_SETTINGS_FORM)) as Ref<OperationalSettingsForm>
  const lastSyncedForm = ref(
    cloneForm(DEFAULT_OPERATIONAL_SETTINGS_FORM),
  ) as Ref<OperationalSettingsForm>
  const validationError = ref('')
  const state = computed(() => resource.query.data.value)
  const loading = computed(() => resource.query.isLoading.value || resource.query.isFetching.value)
  const saving = computed(() => resource.updatePending.value)
  const isDirty = computed(() => !isSameForm(form.value, lastSyncedForm.value))

  watch(
    () => resource.query.error.value,
    (error) => {
      if (error) options.feedback.loadFailed(error)
    },
  )
  watch(
    state,
    (nextState) => {
      if (!nextState || saving.value || isDirty.value) return
      hydrate(
        nextState.task_runtime.configured,
        nextState.outbound_policy.configured,
        nextState.event_retention.configured,
      )
    },
    { immediate: true },
  )
  watch(
    form,
    () => {
      validationError.value = ''
    },
    { deep: true },
  )

  async function save() {
    const error = validateOperationalSettingsForm(form.value)
    validationError.value = error
    if (error) {
      options.feedback.validationFailed(error)
      return false
    }

    try {
      const updatedState = await resource.update(operationalSettingsFormToRequest(form.value))
      hydrate(
        updatedState.task_runtime.configured,
        updatedState.outbound_policy.configured,
        updatedState.event_retention.configured,
      )
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

  function hydrate(
    taskRuntime: OperationalTaskRuntimeSetting,
    outboundPolicy: OperationalOutboundPolicySetting,
    eventRetention: OperationalEventRetentionSetting,
  ) {
    const normalized = normalizeOperationalSettingsForm({
      task_runtime: taskRuntime,
      outbound_policy: outboundPolicy,
      event_retention: eventRetention,
    })
    lastSyncedForm.value = cloneForm(normalized)
    form.value = cloneForm(normalized)
    validationError.value = ''
  }

  return {
    error: computed(() => resource.query.error.value),
    form,
    loading,
    refetch: resource.query.refetch,
    reset,
    save,
    saving,
    state,
    validationError,
  }
}

function cloneForm(form: OperationalSettingsForm): OperationalSettingsForm {
  return {
    task_runtime: { ...form.task_runtime },
    outbound_allow_hosts: form.outbound_allow_hosts,
    nats_retention_days: form.nats_retention_days,
    reason: form.reason,
  }
}

function isSameForm(left: OperationalSettingsForm, right: OperationalSettingsForm): boolean {
  return (
    left.task_runtime.worker_concurrency === right.task_runtime.worker_concurrency &&
    left.task_runtime.task_timeout_seconds === right.task_runtime.task_timeout_seconds &&
    left.task_runtime.no_capacity_retry_seconds === right.task_runtime.no_capacity_retry_seconds &&
    left.task_runtime.max_retry === right.task_runtime.max_retry &&
    left.outbound_allow_hosts === right.outbound_allow_hosts &&
    left.nats_retention_days === right.nats_retention_days &&
    left.reason === right.reason
  )
}
