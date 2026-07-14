import type { QueryClient } from '@tanstack/vue-query'
import { computed } from 'vue'
import type { ApiResponseOperationalSettingsState } from '../../../shared/api/generated-query/model'
import type { AdminOperationalSettingsFeedback } from './useAdminSystemSettingsFeedback'
import {
  DEFAULT_OPERATIONAL_SETTINGS_FORM,
  normalizeOperationalSettingsForm,
  operationalSettingsFormToRequest,
  validateOperationalSettingsForm,
  type OperationalSettingsForm,
  type OperationalSettingsState,
} from './operationalSettingsContract'
import {
  operationalSettingsQueryKey,
  useOperationalSettingsResource,
} from './operationalSettingsResource'
import { useSettingResourceLifecycle } from './useSettingFormLifecycle'

export function useAdminOperationalSettings(options: {
  feedback: AdminOperationalSettingsFeedback
}) {
  const resource = useOperationalSettingsResource()
  const state = computed(() => resource.query.data.value)
  const source = computed(() => state.value && operationalSettingsStateToForm(state.value))
  let savedState: OperationalSettingsState | null = null
  const lifecycle = useSettingResourceLifecycle<OperationalSettingsForm>({
    defaultSetting: DEFAULT_OPERATIONAL_SETTINGS_FORM,
    feedback: options.feedback,
    isSame: isSameForm,
    normalize: normalizeOperationalSettingsFormValue,
    query: resource.query,
    queryKey: operationalSettingsQueryKey,
    save: async (form) => {
      savedState = await resource.update(operationalSettingsFormToRequest(form))
      return operationalSettingsStateToForm(savedState)
    },
    source,
    validate: validateOperationalSettingsForm,
    writeQueryCache: (queryClient) => writeOperationalSettingsQueryCache(queryClient, savedState),
  })

  return {
    error: computed(() => resource.query.error.value),
    form: lifecycle.form,
    loading: lifecycle.loading,
    refetch: resource.query.refetch,
    reset: lifecycle.reset,
    save: lifecycle.save,
    saving: lifecycle.saving,
    state,
    validationError: lifecycle.validationError,
  }
}

function operationalSettingsStateToForm(state: OperationalSettingsState): OperationalSettingsForm {
  return normalizeOperationalSettingsForm({
    task_runtime: state.task_runtime.configured,
    outbound_policy: state.outbound_policy.configured,
    event_retention: state.event_retention.configured,
  })
}

function normalizeOperationalSettingsFormValue(
  form: Partial<OperationalSettingsForm> | null | undefined,
): OperationalSettingsForm {
  return {
    task_runtime: {
      worker_concurrency:
        form?.task_runtime?.worker_concurrency ??
        DEFAULT_OPERATIONAL_SETTINGS_FORM.task_runtime.worker_concurrency,
      task_timeout_seconds:
        form?.task_runtime?.task_timeout_seconds ??
        DEFAULT_OPERATIONAL_SETTINGS_FORM.task_runtime.task_timeout_seconds,
      no_capacity_retry_seconds:
        form?.task_runtime?.no_capacity_retry_seconds ??
        DEFAULT_OPERATIONAL_SETTINGS_FORM.task_runtime.no_capacity_retry_seconds,
      max_retry:
        form?.task_runtime?.max_retry ?? DEFAULT_OPERATIONAL_SETTINGS_FORM.task_runtime.max_retry,
    },
    outbound_allow_hosts: form?.outbound_allow_hosts ?? '',
    nats_retention_days:
      form?.nats_retention_days ?? DEFAULT_OPERATIONAL_SETTINGS_FORM.nats_retention_days,
    reason: form?.reason ?? '',
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

function writeOperationalSettingsQueryCache(
  queryClient: QueryClient,
  state: OperationalSettingsState | null,
) {
  if (!state) return
  queryClient.setQueryData<ApiResponseOperationalSettingsState>(operationalSettingsQueryKey, {
    code: 0,
    msg: 'ok',
    data: state,
  })
}
