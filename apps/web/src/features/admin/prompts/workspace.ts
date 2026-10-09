import { computed, onMounted, onScopeDispose, reactive, ref, toRef } from 'vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { Service } from '../../../shared/domain/imageServices'
import {
  getServicePromptVariables,
  type ServicePromptVariable,
} from '../../../shared/domain/serviceRegistry'
import type { AdminPromptConfigFeedback } from './usePromptConfigFeedback'
import {
  createAdminPromptConfigResource,
  type AdminPromptConfigResource,
  type FeaturePrompt,
} from './resource'

export type { FeaturePrompt } from './resource'
export type {
  ServicePromptVariable as FeaturePromptVariable,
  ServicePromptVariableTone as FeaturePromptVariableTone,
} from '../../../shared/domain/serviceRegistry'

type RunAsyncAction = ReturnType<typeof useAsyncAction>['runAsyncAction']

export type PromptDraftValidation = {
  canSave: boolean
  isDirty: boolean
  message: string
  severity: 'neutral' | 'warning' | 'error'
}

export type AdminPromptConfigWorkspaceOptions = {
  feedback: AdminPromptConfigFeedback
  runAsyncAction: RunAsyncAction
  getFeatureTitle?: (slug: string) => string
  resource?: AdminPromptConfigResource
}

export type UseAdminPromptConfigWorkspaceOptions = Omit<
  AdminPromptConfigWorkspaceOptions,
  'runAsyncAction'
>

export function useAdminPromptConfigWorkspace(options: UseAdminPromptConfigWorkspaceOptions) {
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminPromptConfigWorkspace({
    ...options,
    runAsyncAction,
  })

  // Page-leave cancellation: the mount fetch aborts with the page scope so a
  // slow Control read cannot toast `loadFailed` after unmount.
  const loadController = new AbortController()
  onScopeDispose(() => loadController.abort())

  onMounted(() => {
    void workspace.loadPrompts({ signal: loadController.signal })
  })

  return workspace
}

function isAbortError(error: unknown): boolean {
  return (
    (typeof DOMException !== 'undefined' &&
      error instanceof DOMException &&
      error.name === 'AbortError') ||
    (typeof error === 'object' &&
      error !== null &&
      'name' in error &&
      (error as { name?: unknown }).name === 'AbortError')
  )
}

export function createAdminPromptConfigWorkspace(options: AdminPromptConfigWorkspaceOptions) {
  const resource = options.resource ?? createAdminPromptConfigResource()
  const loading = ref(false)
  const loadFailed = ref(false)
  const promptMap = reactive<Record<string, string>>({})
  const savedPromptMap = reactive<Record<string, string>>({})
  const promptConfigMap = reactive<Record<string, FeaturePrompt>>({})
  const saving = reactive<Record<string, boolean>>({})
  const promptList = ref<FeaturePrompt[]>([])

  const validations = computed<Record<string, PromptDraftValidation>>(() => {
    const slugs = new Set([...Object.keys(promptConfigMap), ...Object.keys(promptMap)])
    return Object.fromEntries([...slugs].map((slug) => [slug, resolveDraftValidation(slug)]))
  })

  async function refreshPromptData(input: { preserveDirtyDraft: boolean; signal?: AbortSignal }) {
    const prompts = await resource.fetchPrompts(input.signal)
    loadFailed.value = false
    syncPromptConfigs(prompts, input)
    return prompts
  }

  async function loadPrompts(input: { preserveDirtyDraft?: boolean; signal?: AbortSignal } = {}) {
    return options.runAsyncAction(
      () =>
        refreshPromptData({
          preserveDirtyDraft: input.preserveDirtyDraft ?? true,
          ...(input.signal == null ? {} : { signal: input.signal }),
        }),
      {
        loading,
        feedback: {
          error: (error) => {
            // Page-leave cancellation is silent: never toast or latch
            // `loadFailed` for an aborted mount fetch.
            if (isAbortError(error)) return
            loadFailed.value = true
            options.feedback.loadPromptsFailed(error)
          },
        },
      },
    )
  }

  function refreshPrompts() {
    return loadPrompts({ preserveDirtyDraft: true })
  }

  function syncPromptConfigs(
    configs: FeaturePrompt[],
    syncOptions: { preserveDirtyDraft: boolean },
  ) {
    const incomingSlugs = new Set(configs.map((config) => config.feature_slug))
    for (const slug of Object.keys(promptConfigMap)) {
      if (incomingSlugs.has(slug)) continue
      delete promptConfigMap[slug]
      delete savedPromptMap[slug]
      delete promptMap[slug]
    }

    promptList.value = configs
    for (const config of configs) {
      syncPromptConfig(config, syncOptions)
    }
  }

  function syncPromptConfig(config: FeaturePrompt, syncOptions: { preserveDirtyDraft: boolean }) {
    const slug = config.feature_slug
    const prompt = config.prompt || ''
    const previousSavedPrompt = savedPromptMap[slug] || ''
    const hadPromptDraft = hasOwn(promptMap, slug)
    const isPromptDirty = hadPromptDraft && (promptMap[slug] || '') !== previousSavedPrompt
    const shouldPreserveDraft = syncOptions.preserveDirtyDraft && isPromptDirty

    promptConfigMap[slug] = config
    savedPromptMap[slug] = prompt

    if (!shouldPreserveDraft) {
      promptMap[slug] = prompt
    }
  }

  function updatePromptDraft(slug: string, prompt: string) {
    if (promptMap[slug] === prompt) return
    promptMap[slug] = prompt
  }

  function insertVariableIntoDraft(slug: string, variableKey: string, position?: number) {
    const current = promptMap[slug] || ''
    const insertAt = normalizeInsertPosition(position, current.length)
    promptMap[slug] = current.slice(0, insertAt) + variableKey + current.slice(insertAt)
  }

  function resetDrafts(slugs: readonly string[]) {
    for (const slug of slugs) {
      promptMap[slug] = savedPromptMap[slug] || ''
    }
  }

  function getSavedPrompt(slug: string): string {
    return savedPromptMap[slug] || ''
  }

  function getPromptVariables(slug: string): readonly ServicePromptVariable[] {
    return getServicePromptVariables(slug as Service)
  }

  function getDraftValidation(slug: string): PromptDraftValidation {
    return validations.value[slug] ?? resolveDraftValidation(slug)
  }

  async function savePrompt(slug: string): Promise<FeaturePrompt | null> {
    const validation = getDraftValidation(slug)
    if (!validation.canSave) {
      options.feedback.promptDraftInvalid(validation.message)
      return null
    }

    // Routine config write (backend ReasonOptional): save directly
    // without the confirm dialog; the audit records an empty reason.
    const config = await options.runAsyncAction(
      async () => {
        const savedConfig = await resource.savePrompt(
          {
            slug,
            prompt: promptMap[slug] || '',
          },
          { operatorReason: '' },
        )
        syncPromptConfig(savedConfig, { preserveDirtyDraft: false })
        await refreshPromptData({ preserveDirtyDraft: true })
        return savedConfig
      },
      {
        loading: toRef(saving, slug),
        feedback: {
          success: () => options.feedback.promptSaved(getFeatureTitle(slug)),
          error: options.feedback.savePromptFailed,
        },
      },
    )
    return config ?? null
  }

  function resolveDraftValidation(slug: string): PromptDraftValidation {
    if (!promptConfigMap[slug]) {
      return {
        canSave: false,
        isDirty: false,
        message: '提示词配置已刷新，请重新选择',
        severity: 'error',
      }
    }

    const prompt = promptMap[slug] || ''
    const isDirty = prompt !== (savedPromptMap[slug] || '')

    if (!prompt.trim()) {
      return {
        canSave: true,
        isDirty,
        message: '提示词为空，保存后将使用空模板',
        severity: 'warning',
      }
    }

    return {
      canSave: true,
      isDirty,
      message: isDirty ? '可保存' : '无未保存更改',
      severity: 'neutral',
    }
  }

  function getFeatureTitle(slug: string) {
    return options.getFeatureTitle?.(slug) ?? slug
  }

  return {
    getDraftValidation,
    getPromptVariables,
    getSavedPrompt,
    insertVariableIntoDraft,
    loadFailed,
    loadPrompts,
    loading,
    promptMap,
    prompts: computed(() => promptList.value),
    refreshPrompts,
    resetDrafts,
    savePrompt,
    saving,
    updatePromptDraft,
    validationMap: validations,
  }
}

function hasOwn(target: Record<string, unknown>, key: string) {
  return Object.prototype.hasOwnProperty.call(target, key)
}

function normalizeInsertPosition(position: number | undefined, fallback: number) {
  if (position == null || !Number.isFinite(position)) return fallback
  return Math.max(0, Math.min(Math.trunc(position), fallback))
}
