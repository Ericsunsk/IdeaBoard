import { describe, expect, it } from 'vitest'
import { computed, ref } from 'vue'
import { getServicePromptVariables } from './promptVariables'
import { groupedFeatureGroups, type FeatureSlug } from './featurePromptMeta'
import { usePromptConfigFilters } from './filtering'

function createHarness() {
  const groups = ref([...groupedFeatureGroups])
  const configured = new Set<FeatureSlug>(['render'])
  const filters = usePromptConfigFilters({
    groups: computed(() => groups.value),
    isConfigured: (slug) => configured.has(slug),
    getSavedPrompt: (slug) => (configured.has(slug) ? `saved template ${slug}` : ''),
    getPromptVariables: (slug) => getServicePromptVariables(slug),
  })
  return { filters }
}

function selectedSlugs(filters: ReturnType<typeof usePromptConfigFilters>): string[] {
  return filters.filteredFeatureGroups.value.flatMap((group) =>
    group.items.map((item) => item.slug as string),
  )
}

describe('usePromptConfigFilters', () => {
  it('returns all groups without any filter', () => {
    const { filters } = createHarness()

    expect(filters.hasActiveFilter.value).toBe(false)
    expect(filters.filteredFeatureGroups.value).toHaveLength(groupedFeatureGroups.length)
    expect(selectedSlugs(filters)).toContain('cutout')
  })

  it('filters by configuration status', () => {
    const { filters } = createHarness()

    filters.statusFilter.value = 'configured'
    expect(filters.hasActiveFilter.value).toBe(true)
    expect(selectedSlugs(filters)).toEqual(['render'])

    filters.statusFilter.value = 'unconfigured'
    const slugs = selectedSlugs(filters)
    expect(slugs).not.toContain('render')
    expect(slugs).not.toContain('cutout')
    expect(slugs).toHaveLength(groupedFeatureGroups.flatMap((group) => group.items).length - 2)
  })

  it('matches keyword against slugs and saved templates only', () => {
    const { filters } = createHarness()

    filters.searchKeyword.value = 'fusion'
    expect(selectedSlugs(filters)).toEqual(['fusion'])

    filters.searchKeyword.value = 'saved template render'
    expect(selectedSlugs(filters)).toEqual(['render'])

    filters.searchKeyword.value = 'cutout'
    expect(selectedSlugs(filters)).toEqual(['cutout'])

    filters.searchKeyword.value = '抠图'
    expect(selectedSlugs(filters)).toEqual(['cutout'])
  })

  it('matches keyword against variable keys', () => {
    const { filters } = createHarness()

    filters.searchKeyword.value = 'user_prompt'
    expect(selectedSlugs(filters).sort()).toEqual(
      ['fusion', 'generation', 'render', 'variations'].sort(),
    )
  })

  it('clears search and status filters together', () => {
    const { filters } = createHarness()

    filters.searchKeyword.value = 'fusion'
    filters.statusFilter.value = 'configured'
    filters.clearFilters()

    expect(filters.searchKeyword.value).toBe('')
    expect(filters.statusFilter.value).toBe('all')
    expect(filters.hasActiveFilter.value).toBe(false)
    expect(filters.filteredFeatureGroups.value).toHaveLength(groupedFeatureGroups.length)
  })
})
