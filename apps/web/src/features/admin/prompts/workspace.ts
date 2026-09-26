import { computed, onMounted, reactive, ref, toRef } from 'vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminConfirmPort } from '../shared/composables/useAdminConfirm'
import { confirmSettingsSave } from '../shared/lib/confirmSettingsSave'
import type { WorkflowFeature } from '../../../shared/domain/workflowFeatures'
import {
  getWorkflowFeaturePromptVariables,
  type WorkflowFeaturePromptVariable,
} from '../../../shared/domain/workflowFeatureRegistry'
import type { AdminPromptConfigFeedback } from './usePromptConfigFeedback'
import {
  createAdminPromptConfigResource,
  type AdminPromptConfigResource,
  type FeaturePrompt,
} from './resource'

export type { FeaturePrompt } from './resource'
export type {
  WorkflowFeaturePromptVariable as FeaturePromptVariable,
  WorkflowFeaturePromptVariableTone as FeaturePromptVariableTone,
} from '../../../shared/domain/workflowFeatureRegistry'

type RunAsyncAction = ReturnType<typeof useAsyncAction>['runAsyncAction']

export type PromptDraftValidation = {
  canSave: boolean
  isDirty: boolean
  message: string
  severity: 'neutral' | 'warning' | 'error'
}

export type AdminPromptConfigWorkspaceOptions = {
  feedback: AdminPromptConfigFeedback
  confirm: AdminConfirmPort
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

  onMounted(() => {
    void workspace.loadPrompts()
  })

  return workspace
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

  async function refreshPromptData(input: { preserveDirtyDraft: boolean }) {
    const prompts = await resource.fetchPrompts()
    loadFailed.value = false
    syncPromptConfigs(prompts, input)
    return prompts
  }

  async function loadPrompts(input: { preserveDirtyDraft?: boolean } = {}) {
    return options.runAsyncAction(
      () =>
        refreshPromptData({
          preserveDirtyDraft: input.preserveDirtyDraft ?? true,
        }),
      {
        loading,
        feedback: {
          error: (error) => {
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

  function getPromptVariables(slug: string): readonly WorkflowFeaturePromptVariable[] {
    return getWorkflowFeaturePromptVariables(slug as WorkflowFeature)
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

    // Control writes require an operator reason (audit); a cancelled
    // dialog aborts silently.
    const operatorReason = await confirmSettingsSave(options.confirm, {
      title: '保存提示词',
      message: `确定保存 ${getFeatureTitle(slug)} 的提示词模板吗？该操作将记录操作人与原因。`,
      confirmLabel: '保存',
    })
    if (!operatorReason) return null

    const config = await options.runAsyncAction(
      async () => {
        const savedConfig = await resource.savePrompt(
          {
            slug,
            prompt: promptMap[slug] || '',
          },
          { operatorReason },
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
