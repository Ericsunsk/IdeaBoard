import { computed, ref, type ComputedRef } from 'vue'
import type { FeaturePromptVariable } from '../session/workspace'
import {
  getFeaturePromptStatus,
  type FeatureGroup,
  type FeatureItem,
  type FeatureSlug,
} from './featurePromptMeta'

export type PromptConfigStatusFilter = 'all' | 'configured' | 'unconfigured'

const PROMPT_CONFIG_STATUS_FILTER_OPTIONS: ReadonlyArray<{
  label: string
  value: PromptConfigStatusFilter
}> = [
  { label: '全部状态', value: 'all' },
  { label: '已设置', value: 'configured' },
  { label: '未设置', value: 'unconfigured' },
]

export type UsePromptConfigFiltersOptions = {
  groups: ComputedRef<readonly FeatureGroup[]> | { readonly value: readonly FeatureGroup[] }
  isConfigured: (slug: FeatureSlug) => boolean
  getSavedPrompt: (slug: FeatureSlug) => string
  getPromptVariables: (slug: FeatureSlug) => readonly FeaturePromptVariable[]
}

function matchesItem(
  item: FeatureItem,
  query: string,
  filter: PromptConfigStatusFilter,
  groupTitle: string,
  options: Pick<
    UsePromptConfigFiltersOptions,
    'isConfigured' | 'getSavedPrompt' | 'getPromptVariables'
  >,
): boolean {
  const status = getFeaturePromptStatus(item.slug, options.isConfigured(item.slug))
  if (filter !== 'all' && filter !== status) return false
  if (!query) return true

  if (groupTitle.toLowerCase().includes(query)) return true
  if (item.title.toLowerCase().includes(query)) return true
  if (item.label.toLowerCase().includes(query)) return true
  if (item.slug.toLowerCase().includes(query)) return true
  if (status === 'unsupported') return false
  // 搜索只匹配已保存提示词, 避免未保存 draft 泄漏到表格预览/搜索结果。
  const promptContent = options.getSavedPrompt(item.slug).toLowerCase()
  if (promptContent.includes(query)) return true
  const vars = options.getPromptVariables(item.slug)
  return vars.some((variable) => variable.key.toLowerCase().includes(query))
}

export function usePromptConfigFilters(options: UsePromptConfigFiltersOptions) {
  const searchKeyword = ref('')
  const statusFilter = ref<PromptConfigStatusFilter>('all')

  const hasActiveFilter = computed(() =>
    Boolean(searchKeyword.value.trim() || statusFilter.value !== 'all'),
  )

  function clearFilters() {
    searchKeyword.value = ''
    statusFilter.value = 'all'
  }

  const filteredFeatureGroups = computed(() => {
    const query = searchKeyword.value.trim().toLowerCase()
    const filter = statusFilter.value
    const sourceGroups = options.groups.value

    if (!query && filter === 'all') return sourceGroups

    const results: FeatureGroup[] = []
    for (const group of sourceGroups) {
      const matchedItems = group.items.filter((item) =>
        matchesItem(item, query, filter, group.title, options),
      )

      if (matchedItems.length > 0) {
        results.push({
          ...group,
          items: matchedItems,
        })
      }
    }
    return results
  })

  return {
    searchKeyword,
    statusFilter,
    statusFilterOptions: PROMPT_CONFIG_STATUS_FILTER_OPTIONS,
    hasActiveFilter,
    clearFilters,
    filteredFeatureGroups,
  }
}
