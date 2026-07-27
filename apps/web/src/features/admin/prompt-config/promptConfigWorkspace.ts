import { computed, onMounted, reactive, ref } from 'vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminWorkflowFeatureFeedback } from './useAdminPromptConfigFeedback'
import {
  createAdminWorkflowFeatureResource,
  type AdminWorkflowFeatureResource,
  type WorkflowFeature,
  type WorkflowFeatureVariable,
} from './promptConfigResource'
import type { WorkflowFeatureMergePolicy } from '../../../shared/api/generated-query/model'

export type {
  WorkflowFeature,
  WorkflowFeatureVariable,
  WorkflowFeatureVariableTone,
} from './promptConfigResource'

type RunAsyncAction = ReturnType<typeof useAsyncAction>['runAsyncAction']

export type WorkflowFeatureDraft = {
  canonicalModel: string
  systemPrompt: string
  enabled: boolean
  variables: WorkflowFeatureVariable[]
  userPromptMergePolicy: WorkflowFeatureMergePolicy
}

export type WorkflowFeatureDraftValidation = {
  canSave: boolean
  isDirty: boolean
  message: string
  severity: 'neutral' | 'warning' | 'error'
}

export type AdminWorkflowFeatureWorkspaceOptions = {
  feedback: AdminWorkflowFeatureFeedback
  runAsyncAction: RunAsyncAction
  getFeatureTitle?: (slug: string) => string
  resource?: AdminWorkflowFeatureResource
}

export type UseAdminWorkflowFeatureWorkspaceOptions = Omit<
  AdminWorkflowFeatureWorkspaceOptions,
  'runAsyncAction'
>

export function useAdminWorkflowFeatureWorkspace(options: UseAdminWorkflowFeatureWorkspaceOptions) {
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminWorkflowFeatureWorkspace({
    ...options,
    runAsyncAction,
  })

  onMounted(() => {
    void workspace.loadWorkflowFeatures()
  })

  return workspace
}

export function createAdminWorkflowFeatureWorkspace(options: AdminWorkflowFeatureWorkspaceOptions) {
  const resource = options.resource ?? createAdminWorkflowFeatureResource()
  const loading = ref(false)
  const loadFailed = ref(false)
  const draftMap = reactive<Record<string, WorkflowFeatureDraft>>({})
  const savedDraftMap = reactive<Record<string, WorkflowFeatureDraft>>({})
  const workflowFeatureMap = reactive<Record<string, WorkflowFeature>>({})
  const saving = reactive<Record<string, boolean>>({})
  const workflowFeatureList = ref<WorkflowFeature[]>([])

  const validations = computed<Record<string, WorkflowFeatureDraftValidation>>(() => {
    const slugs = new Set([...Object.keys(workflowFeatureMap), ...Object.keys(draftMap)])
    return Object.fromEntries([...slugs].map((slug) => [slug, resolveDraftValidation(slug)]))
  })

  async function loadWorkflowFeatures(input: { preserveDirtyDraft?: boolean } = {}) {
    return options.runAsyncAction(
      async () => {
        const workflowFeatures = await resource.fetchWorkflowFeatures()
        loadFailed.value = false
        syncWorkflowFeatures(workflowFeatures, {
          preserveDirtyDraft: input.preserveDirtyDraft ?? true,
        })
        return workflowFeatures
      },
      {
        loading,
        feedback: {
          error: (error) => {
            loadFailed.value = true
            options.feedback.loadWorkflowFeaturesFailed(error)
          },
        },
      },
    )
  }

  function refreshWorkflowFeatures() {
    return loadWorkflowFeatures({ preserveDirtyDraft: true })
  }

  function syncWorkflowFeatures(
    configs: WorkflowFeature[],
    syncOptions: { preserveDirtyDraft: boolean },
  ) {
    const incomingSlugs = new Set(configs.map((config) => config.feature_slug))
    for (const slug of Object.keys(workflowFeatureMap)) {
      if (incomingSlugs.has(slug)) continue
      delete workflowFeatureMap[slug]
      delete savedDraftMap[slug]
      delete draftMap[slug]
    }

    workflowFeatureList.value = configs
    for (const config of configs) {
      syncWorkflowFeature(config, syncOptions)
    }
  }

  function syncWorkflowFeature(
    config: WorkflowFeature,
    syncOptions: { preserveDirtyDraft: boolean },
  ) {
    const slug = config.feature_slug
    const nextSavedDraft = toWorkflowFeatureDraft(config)
    const previousSavedDraft = savedDraftMap[slug]
    const isDirty = Boolean(
      draftMap[slug] && previousSavedDraft && !areDraftsEqual(draftMap[slug], previousSavedDraft),
    )

    workflowFeatureMap[slug] = config
    savedDraftMap[slug] = cloneDraft(nextSavedDraft)

    if (!syncOptions.preserveDirtyDraft || !isDirty) {
      draftMap[slug] = cloneDraft(nextSavedDraft)
    }
  }

  function updateCanonicalModelDraft(slug: string, canonicalModel: string) {
    ensureDraft(slug).canonicalModel = canonicalModel
  }

  function updateSystemPromptDraft(slug: string, systemPrompt: string) {
    ensureDraft(slug).systemPrompt = systemPrompt
  }

  function updateEnabledDraft(slug: string, enabled: boolean) {
    ensureDraft(slug).enabled = enabled
  }

  function insertVariableIntoDraft(slug: string, variableKey: string, position?: number) {
    const draft = ensureDraft(slug)
    const insertAt = normalizeInsertPosition(position, draft.systemPrompt.length)
    draft.systemPrompt =
      draft.systemPrompt.slice(0, insertAt) + variableKey + draft.systemPrompt.slice(insertAt)
  }

  function resetDrafts(slugs: readonly string[]) {
    for (const slug of slugs) {
      const savedDraft = savedDraftMap[slug]
      if (savedDraft) draftMap[slug] = cloneDraft(savedDraft)
    }
  }

  function getDraft(slug: string): WorkflowFeatureDraft {
    return draftMap[slug] ?? emptyDraft()
  }

  function getSavedWorkflowFeature(slug: string): WorkflowFeature | undefined {
    return workflowFeatureMap[slug]
  }

  function getWorkflowFeatureVariables(slug: string): WorkflowFeatureVariable[] {
    return workflowFeatureMap[slug]?.variables ?? []
  }

  function getDraftValidation(slug: string): WorkflowFeatureDraftValidation {
    return validations.value[slug] ?? resolveDraftValidation(slug)
  }

  async function saveWorkflowFeature(slug: string): Promise<WorkflowFeature | null> {
    const validation = getDraftValidation(slug)
    if (!validation.canSave) {
      options.feedback.workflowFeatureDraftInvalid(validation.message)
      return null
    }
    if (saving[slug]) return null

    const draft = draftMap[slug]
    if (!draft) return null

    saving[slug] = true
    try {
      const config = await resource.saveWorkflowFeature({
        slug,
        canonical_model: draft.canonicalModel.trim(),
        system_prompt: draft.systemPrompt,
        enabled: draft.enabled,
        variables: draft.variables.map((variable) => ({ ...variable })),
        user_prompt_merge_policy: { ...draft.userPromptMergePolicy },
      })
      syncWorkflowFeature(config, { preserveDirtyDraft: false })
      options.feedback.workflowFeatureSaved(getFeatureTitle(slug))
      await refreshWorkflowFeatures()
      return config
    } catch (error) {
      options.feedback.saveWorkflowFeatureFailed(error)
      return null
    } finally {
      saving[slug] = false
    }
  }

  function resolveDraftValidation(slug: string): WorkflowFeatureDraftValidation {
    const savedDraft = savedDraftMap[slug]
    const draft = draftMap[slug]
    if (!workflowFeatureMap[slug] || !savedDraft || !draft) {
      return {
        canSave: false,
        isDirty: false,
        message: '工作流配置已刷新，请重新选择',
        severity: 'error',
      }
    }

    const isDirty = !areDraftsEqual(draft, savedDraft)
    if (!draft.canonicalModel.trim()) {
      return {
        canSave: false,
        isDirty,
        message: '请输入 Canonical Model',
        severity: 'error',
      }
    }

    if (!draft.systemPrompt.trim()) {
      return {
        canSave: true,
        isDirty,
        message: 'System Prompt 为空，执行时将不注入系统提示词',
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

  function ensureDraft(slug: string) {
    if (!draftMap[slug]) draftMap[slug] = emptyDraft()
    return draftMap[slug]
  }

  function getFeatureTitle(slug: string) {
    return options.getFeatureTitle?.(slug) ?? slug
  }

  return {
    draftMap,
    getDraft,
    getDraftValidation,
    getSavedWorkflowFeature,
    getWorkflowFeatureVariables,
    insertVariableIntoDraft,
    loadFailed,
    loadWorkflowFeatures,
    loading,
    refreshWorkflowFeatures,
    resetDrafts,
    saveWorkflowFeature,
    saving,
    updateCanonicalModelDraft,
    updateEnabledDraft,
    updateSystemPromptDraft,
    validationMap: validations,
    workflowFeatures: computed(() => workflowFeatureList.value),
  }
}

function toWorkflowFeatureDraft(config: WorkflowFeature): WorkflowFeatureDraft {
  return {
    canonicalModel: config.canonical_model,
    systemPrompt: config.system_prompt,
    enabled: config.enabled,
    variables: config.variables.map((variable) => ({ ...variable })),
    userPromptMergePolicy: { ...config.user_prompt_merge_policy },
  }
}

function cloneDraft(draft: WorkflowFeatureDraft): WorkflowFeatureDraft {
  return {
    ...draft,
    variables: draft.variables.map((variable) => ({ ...variable })),
    userPromptMergePolicy: { ...draft.userPromptMergePolicy },
  }
}

function emptyDraft(): WorkflowFeatureDraft {
  return {
    canonicalModel: '',
    systemPrompt: '',
    enabled: false,
    variables: [],
    userPromptMergePolicy: {
      append_base_prompt_without_template: true,
      append_user_prompt_when_template_misses_placeholder: true,
    },
  }
}

function areDraftsEqual(left: WorkflowFeatureDraft, right: WorkflowFeatureDraft) {
  return JSON.stringify(left) === JSON.stringify(right)
}

function normalizeInsertPosition(position: number | undefined, fallback: number) {
  if (position == null || !Number.isFinite(position)) return fallback
  return Math.max(0, Math.min(Math.trunc(position), fallback))
}
