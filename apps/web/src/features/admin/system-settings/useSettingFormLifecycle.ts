import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { QueryClient, QueryKey } from '@tanstack/vue-query'
import { computed, ref, watch } from 'vue'
import type { Ref } from 'vue'

type ReadonlyValue<T> = {
  readonly value: T
}

type SettingFormLifecycleFeedback = {
  loadFailed: (error: unknown) => void
  validationFailed?: (message: string) => void
  saved?: () => void
  saveFailed?: (error: unknown) => void
}

type SettingResourceQuery = {
  error: ReadonlyValue<unknown>
  isFetching: ReadonlyValue<boolean>
  isLoading: ReadonlyValue<boolean>
}

type SettingQueryCacheEntry<TSetting> = {
  code?: number
  msg?: string
  data?: TSetting
}

export function useSettingResourceLifecycle<TSetting extends object>(options: {
  defaultSetting: TSetting
  extraInvalidateQueryKeys?: QueryKey[]
  feedback: SettingFormLifecycleFeedback
  isSame: (current: TSetting, synced: TSetting) => boolean
  normalize: (setting: Partial<TSetting> | null | undefined) => TSetting
  query: SettingResourceQuery
  queryKey: QueryKey
  save: (setting: TSetting) => Promise<TSetting>
  source: ReadonlyValue<Partial<TSetting> | null | undefined>
  validate: (setting: TSetting) => string
  writeAdditionalQueryCache?: (queryClient: QueryClient, setting: TSetting) => void
}) {
  const queryClient = useQueryClient()
  const form = ref<TSetting>(cloneSetting(options.defaultSetting)) as Ref<TSetting>
  const lastSyncedForm = ref<TSetting>(cloneSetting(options.defaultSetting)) as Ref<TSetting>
  const loading = computed(() => options.query.isLoading.value || options.query.isFetching.value)

  function invalidateQuery() {
    return Promise.all([
      queryClient.invalidateQueries({ queryKey: options.queryKey }),
      ...(options.extraInvalidateQueryKeys ?? []).map((queryKey) =>
        queryClient.invalidateQueries({ queryKey }),
      ),
    ])
  }

  function writeQueryCache(setting: TSetting) {
    queryClient.setQueryData<SettingQueryCacheEntry<TSetting>>(options.queryKey, (previous) => ({
      code: previous?.code ?? 0,
      msg: previous?.msg ?? 'ok',
      data: setting,
    }))
    options.writeAdditionalQueryCache?.(queryClient, setting)
  }

  function getSaveFeedback() {
    const { validationFailed, saved, saveFailed } = options.feedback
    if (!validationFailed || !saved || !saveFailed) {
      throw new Error('Setting form save feedback is not configured')
    }
    return { validationFailed, saved, saveFailed }
  }

  function hydrate(setting: Partial<TSetting> | null | undefined) {
    const normalizedSetting = options.normalize(setting)
    lastSyncedForm.value = cloneSetting(normalizedSetting)
    form.value = cloneSetting(normalizedSetting)
    return normalizedSetting
  }

  const saveMutation = useMutation({
    mutationFn: options.save,
    onSuccess: async (setting) => {
      const normalizedSetting = hydrate(setting)
      writeQueryCache(normalizedSetting)
      await invalidateQuery()
    },
  })
  const saving = computed(() => saveMutation.isPending.value)
  const isDirty = computed(() => !options.isSame(form.value, lastSyncedForm.value))

  async function save(): Promise<boolean> {
    const feedback = getSaveFeedback()
    const validationError = options.validate(form.value)
    if (validationError) {
      feedback.validationFailed(validationError)
      return false
    }

    try {
      await saveMutation.mutateAsync(options.normalize(form.value))
      feedback.saved()
      return true
    } catch (error) {
      feedback.saveFailed(error)
      return false
    }
  }

  function reset() {
    form.value = cloneSetting(lastSyncedForm.value)
  }

  watch(
    () => options.query.error.value,
    (error) => {
      if (error) options.feedback.loadFailed(error)
    },
  )

  watch(
    () => options.source.value,
    (setting) => {
      if (!setting || saveMutation.isPending.value || isDirty.value) return
      hydrate(setting)
    },
    { immediate: true },
  )

  return {
    form,
    isDirty,
    loading,
    reset,
    save,
    saving,
  }
}

function cloneSetting<TSetting extends object>(setting: TSetting): TSetting {
  return { ...setting }
}
