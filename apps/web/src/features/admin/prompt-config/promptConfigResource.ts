import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { computed, reactive, watch } from 'vue'
import {
  getListAdminFeaturePromptsQueryKey,
  setAdminFeaturePrompt,
  useListAdminFeaturePrompts,
} from '../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import type {
  FeaturePrompt,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
  SetAdminFeaturePromptBody,
} from '../../../shared/api/generated-query/model'
import type { AdminFeedback } from '../feedback/useAdminFeedback'
import { requireApiData } from '../shared/queries/adminApiResponse'

export type {
  FeaturePrompt,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
}

type SaveFeaturePromptInput = {
  slug: string
} & SetAdminFeaturePromptBody

export async function saveFeaturePrompt(input: SaveFeaturePromptInput): Promise<FeaturePrompt> {
  const { slug, ...payload } = input
  const res = await setAdminFeaturePrompt(slug, payload)
  return requireApiData(res, '保存提示词失败')
}

type PromptConfigFeedback = AdminFeedback['promptConfig']

type PromptConfigResourceOptions = {
  feedback: PromptConfigFeedback
  getFeatureTitle?: (slug: string) => string
}

export function usePromptConfigResource(options: PromptConfigResourceOptions) {
  const queryClient = useQueryClient()
  const promptMap = reactive<Record<string, string>>({})
  const savedPromptMap = reactive<Record<string, string>>({})
  const enabledMap = reactive<Record<string, boolean>>({})
  const promptConfigMap = reactive<Record<string, FeaturePrompt>>({})
  const saving = reactive<Record<string, boolean>>({})
  const promptsQuery = useListAdminFeaturePrompts()
  const featurePrompts = computed(() => promptsQuery.data.value?.data?.prompts)

  const savePromptMutation = useMutation({
    mutationFn: saveFeaturePrompt,
    onSuccess: handlePromptUpdated,
  })

  watch(
    () => promptsQuery.error.value,
    (error) => {
      if (error) {
        options.feedback.loadPromptsFailed(error)
      }
    },
  )

  watch(
    () => featurePrompts.value,
    (prompts) => {
      if (!prompts) return
      for (const config of prompts) {
        syncPromptConfig(config, { preserveDirtyDraft: true })
      }
    },
    { immediate: true },
  )

  async function handlePromptUpdated(config: FeaturePrompt) {
    syncPromptConfig(config, { preserveDirtyDraft: false })
    await invalidatePromptConfig()
  }

  async function invalidatePromptConfig() {
    await queryClient.invalidateQueries({ queryKey: getListAdminFeaturePromptsQueryKey() })
  }

  function syncPromptConfig(config: FeaturePrompt, options: { preserveDirtyDraft: boolean }) {
    const slug = config.slug
    promptConfigMap[slug] = config
    const prompt = config.prompt || ''
    const hadDraft = Object.prototype.hasOwnProperty.call(promptMap, slug)
    const currentDraft = promptMap[slug] || ''
    const previousSaved = savedPromptMap[slug] || ''
    savedPromptMap[slug] = prompt
    if (!options.preserveDirtyDraft || !hadDraft || currentDraft === previousSaved) {
      promptMap[slug] = prompt
      enabledMap[slug] = config.enabled
    }
  }

  function resetDrafts(slugs: readonly string[]) {
    for (const slug of slugs) {
      promptMap[slug] = savedPromptMap[slug] || ''
      enabledMap[slug] = promptConfigMap[slug]?.enabled ?? true
    }
  }

  function getSavedPrompt(slug: string): string {
    return savedPromptMap[slug] || ''
  }

  function getPromptVariables(slug: string): FeaturePromptVariable[] {
    return promptConfigMap[slug]?.variables ?? []
  }

  async function savePrompt(slug: string): Promise<FeaturePrompt | null> {
    saving[slug] = true
    try {
      const config = await savePromptMutation.mutateAsync({
        slug,
        prompt: promptMap[slug] || '',
        enabled: enabledMap[slug] ?? true,
      })
      options.feedback.promptSaved(getFeatureTitle(slug))
      return config
    } catch (error) {
      options.feedback.savePromptFailed(error)
      return null
    } finally {
      saving[slug] = false
    }
  }

  function getFeatureTitle(slug: string) {
    return options.getFeatureTitle?.(slug) ?? slug
  }

  return {
    enabledMap,
    getPromptVariables,
    getSavedPrompt,
    promptMap,
    resetDrafts,
    savePrompt,
    saving,
  }
}
