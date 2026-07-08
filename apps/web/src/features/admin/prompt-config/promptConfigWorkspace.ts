import { computed, onMounted, reactive, ref } from 'vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminPromptConfigFeedback } from '../feedback/useAdminFeedback'
import {
  createAdminPromptConfigResource,
  type AdminPromptConfigResource,
  type FeaturePrompt,
  type FeaturePromptEvalReport,
  type FeaturePromptVariable,
} from './promptConfigResource'

export type {
  FeaturePrompt,
  FeaturePromptEvalFixtureResult,
  FeaturePromptEvalReport,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
} from './promptConfigResource'

type RunAsyncAction = ReturnType<typeof useAsyncAction>['runAsyncAction']
const promptEvalFeatureSlug = 'pattern-gen'

export type PromptRevisionState = {
  schemaVersion: number
  publishedAt: string | null
  updatedAt: string | null
}

export type PromptDraftValidation = {
  canPublish: boolean
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

  onMounted(() => {
    void workspace.loadPrompts()
  })

  return workspace
}

export function createAdminPromptConfigWorkspace(options: AdminPromptConfigWorkspaceOptions) {
  const resource = options.resource ?? createAdminPromptConfigResource()
  const loading = ref(false)
  const promptMap = reactive<Record<string, string>>({})
  const savedPromptMap = reactive<Record<string, string>>({})
  const enabledMap = reactive<Record<string, boolean>>({})
  const savedEnabledMap = reactive<Record<string, boolean>>({})
  const promptConfigMap = reactive<Record<string, FeaturePrompt>>({})
  const evalReportMap = reactive<Record<string, FeaturePromptEvalReport | null>>({})
  const evalReportDraftKeyMap = reactive<Record<string, string>>({})
  const publishing = reactive<Record<string, boolean>>({})
  const evaluating = reactive<Record<string, boolean>>({})
  const promptList = ref<FeaturePrompt[]>([])

  const validations = computed<Record<string, PromptDraftValidation>>(() => {
    const slugs = new Set([
      ...Object.keys(promptConfigMap),
      ...Object.keys(promptMap),
      ...Object.keys(enabledMap),
    ])
    return Object.fromEntries([...slugs].map((slug) => [slug, resolveDraftValidation(slug)]))
  })

  async function loadPrompts(input: { preserveDirtyDraft?: boolean } = {}) {
    return options.runAsyncAction(
      async () => {
        const prompts = await resource.fetchPrompts()
        syncPromptConfigs(prompts, {
          preserveDirtyDraft: input.preserveDirtyDraft ?? true,
        })
        return prompts
      },
      {
        loading,
        feedback: {
          error: options.feedback.loadPromptsFailed,
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
    promptList.value = configs
    for (const config of configs) {
      syncPromptConfig(config, syncOptions)
    }
  }

  function syncPromptConfig(config: FeaturePrompt, syncOptions: { preserveDirtyDraft: boolean }) {
    const slug = config.slug
    const prompt = config.prompt || ''
    const previousSavedPrompt = savedPromptMap[slug] || ''
    const previousSavedEnabled = getSavedEnabled(slug)
    const hadPromptDraft = hasOwn(promptMap, slug)
    const hadEnabledDraft = hasOwn(enabledMap, slug)
    const isPromptDirty = hadPromptDraft && (promptMap[slug] || '') !== previousSavedPrompt
    const isEnabledDirty = hadEnabledDraft && (enabledMap[slug] ?? true) !== previousSavedEnabled
    const shouldPreserveDraft = syncOptions.preserveDirtyDraft && (isPromptDirty || isEnabledDirty)

    promptConfigMap[slug] = config
    savedPromptMap[slug] = prompt
    savedEnabledMap[slug] = config.enabled

    if (!shouldPreserveDraft) {
      promptMap[slug] = prompt
      enabledMap[slug] = config.enabled
      setPromptEvalReport(slug, config.eval_report ?? null)
      return
    }
    clearPromptEvalReport(slug)
  }

  function updatePromptDraft(slug: string, prompt: string) {
    if (promptMap[slug] === prompt) return
    promptMap[slug] = prompt
    clearPromptEvalReport(slug)
  }

  function updateEnabledDraft(slug: string, enabled: boolean) {
    if (enabledMap[slug] === enabled) return
    enabledMap[slug] = enabled
    clearPromptEvalReport(slug)
  }

  function insertVariableIntoDraft(slug: string, variableKey: string, position?: number) {
    const current = promptMap[slug] || ''
    const insertAt = normalizeInsertPosition(position, current.length)
    promptMap[slug] = current.slice(0, insertAt) + variableKey + current.slice(insertAt)
    clearPromptEvalReport(slug)
  }

  function resetDrafts(slugs: readonly string[]) {
    for (const slug of slugs) {
      promptMap[slug] = savedPromptMap[slug] || ''
      enabledMap[slug] = getSavedEnabled(slug)
      setPromptEvalReport(slug, promptConfigMap[slug]?.eval_report ?? null)
    }
  }

  function getSavedPrompt(slug: string): string {
    return savedPromptMap[slug] || ''
  }

  function getPromptVariables(slug: string): FeaturePromptVariable[] {
    return promptConfigMap[slug]?.variables ?? []
  }

  function getCurrentRevision(slug: string): PromptRevisionState | null {
    const config = promptConfigMap[slug]
    if (!config) return null
    return {
      schemaVersion: config.schema_version,
      publishedAt: config.published_at ?? null,
      updatedAt: config.updated_at ?? null,
    }
  }

  function canEvaluatePrompt(slug: string): boolean {
    return slug === promptEvalFeatureSlug
  }

  function getPromptEvalReport(slug: string): FeaturePromptEvalReport | null {
    const report = evalReportMap[slug] ?? null
    if (!report) return null
    return evalReportDraftKeyMap[slug] === createPromptEvalDraftKey(slug) ? report : null
  }

  function getDraftValidation(slug: string): PromptDraftValidation {
    return validations.value[slug] ?? resolveDraftValidation(slug)
  }

  async function runPromptEval(slug: string): Promise<FeaturePromptEvalReport | null> {
    if (!canEvaluatePrompt(slug)) {
      options.feedback.promptDraftInvalid('该功能暂未配置 Prompt eval')
      return null
    }
    if (!promptConfigMap[slug]) {
      options.feedback.promptDraftInvalid('提示词配置已刷新，请重新选择')
      return null
    }
    if (evaluating[slug]) return null

    const evalDraftKey = createPromptEvalDraftKey(slug)
    evaluating[slug] = true
    try {
      const report = await resource.evaluatePrompt({
        slug,
        prompt: promptMap[slug] || '',
        enabled: enabledMap[slug] ?? true,
      })
      if (createPromptEvalDraftKey(slug) === evalDraftKey) {
        setPromptEvalReport(slug, report, evalDraftKey)
      }
      return report
    } catch (error) {
      options.feedback.savePromptFailed(error)
      return null
    } finally {
      evaluating[slug] = false
    }
  }

  async function publishPrompt(slug: string): Promise<FeaturePrompt | null> {
    const validation = getDraftValidation(slug)
    if (!validation.canPublish) {
      options.feedback.promptDraftInvalid(validation.message)
      return null
    }
    if (publishing[slug]) return null

    publishing[slug] = true
    try {
      if (shouldRunPromptEvalBeforePublish(slug)) {
        const evalDraftKey = createPromptEvalDraftKey(slug)
        const report = await runPromptEval(slug)
        if (!report) return null
        if (createPromptEvalDraftKey(slug) !== evalDraftKey) {
          options.feedback.promptDraftInvalid('Prompt eval 已过期，请重新运行后发布')
          return null
        }
        if (!report.passed) {
          options.feedback.promptDraftInvalid(`Prompt eval 未通过：${report.summary}`)
          return null
        }
      }

      const config = await resource.publishPrompt({
        slug,
        prompt: promptMap[slug] || '',
        enabled: enabledMap[slug] ?? true,
      })
      syncPromptConfig(config, { preserveDirtyDraft: false })
      options.feedback.promptSaved(getFeatureTitle(slug))
      await refreshPrompts()
      return config
    } catch (error) {
      options.feedback.savePromptFailed(error)
      return null
    } finally {
      publishing[slug] = false
    }
  }

  function resolveDraftValidation(slug: string): PromptDraftValidation {
    if (!promptConfigMap[slug]) {
      return {
        canPublish: false,
        isDirty: false,
        message: '提示词配置已刷新，请重新选择',
        severity: 'error',
      }
    }

    const prompt = promptMap[slug] || ''
    const isDirty =
      prompt !== (savedPromptMap[slug] || '') ||
      (enabledMap[slug] ?? true) !== getSavedEnabled(slug)

    if (!prompt.trim()) {
      return {
        canPublish: true,
        isDirty,
        message: '提示词为空，发布后将使用空模板',
        severity: 'warning',
      }
    }

    return {
      canPublish: true,
      isDirty,
      message: isDirty ? '可发布' : '无未保存更改',
      severity: 'neutral',
    }
  }

  function getFeatureTitle(slug: string) {
    return options.getFeatureTitle?.(slug) ?? slug
  }

  function getSavedEnabled(slug: string) {
    return savedEnabledMap[slug] ?? promptConfigMap[slug]?.enabled ?? true
  }

  function shouldRunPromptEvalBeforePublish(slug: string) {
    return canEvaluatePrompt(slug) && (enabledMap[slug] ?? true)
  }

  function createPromptEvalDraftKey(slug: string) {
    return JSON.stringify({
      enabled: enabledMap[slug] ?? true,
      prompt: promptMap[slug] || '',
    })
  }

  function setPromptEvalReport(
    slug: string,
    report: FeaturePromptEvalReport | null,
    draftKey = createPromptEvalDraftKey(slug),
  ) {
    evalReportMap[slug] = report
    evalReportDraftKeyMap[slug] = draftKey
  }

  function clearPromptEvalReport(slug: string) {
    evalReportMap[slug] = null
    delete evalReportDraftKeyMap[slug]
  }

  return {
    canEvaluatePrompt,
    enabledMap,
    evaluating,
    getCurrentRevision,
    getDraftValidation,
    getPromptEvalReport,
    getPromptVariables,
    getSavedPrompt,
    insertVariableIntoDraft,
    loadPrompts,
    loading,
    promptMap,
    prompts: computed(() => promptList.value),
    publishPrompt,
    refreshPrompts,
    resetDrafts,
    runPromptEval,
    savePrompt: publishPrompt,
    saving: publishing,
    updateEnabledDraft,
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

export type AdminPromptConfigWorkspace = ReturnType<typeof createAdminPromptConfigWorkspace>
