import { onBeforeUnmount, ref, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { USER_SEARCH_PATH } from '../../shared/constants/routePaths'
import type { LibraryAssetView } from '../library/libraryResource'
import {
  extractVisionSearch,
  normalizeSearchQuery,
  type VisionReference,
} from './searchResource'
import { searchReferenceKey, toVisionReference } from './searchLibraryBridge'

export function useVisionSearchFlow(options: {
  draftQuery: Ref<string>
  submittedQuery: Ref<string>
  queryError: Ref<string>
  keepResultsOnNextLoad: Ref<boolean>
  loadCatalog: () => Promise<void>
  selectedPalette?: Ref<Set<string>>
  selectedTags?: Ref<Set<string>>
}) {
  const route = useRoute()
  const router = useRouter()
  const references = ref<VisionReference[]>([])
  const fileInput = ref<HTMLInputElement | null>(null)
  const visionState = ref<'idle' | 'extracting' | 'ready' | 'error'>('idle')
  const visionError = ref('')
  const palette = ref<string[]>([])
  const atmosphereTags = ref<string[]>([])
  const selectedPalette = options.selectedPalette ?? ref(new Set<string>())
  const selectedTags = options.selectedTags ?? ref(new Set<string>())
  let refinementTimer: ReturnType<typeof setTimeout> | null = null
  let extractionController: AbortController | null = null
  let extractionGeneration = 0

  async function runVisionSearch() {
    if (references.value.length === 0) return
    extractionController?.abort()
    const controller = new AbortController()
    extractionController = controller
    const generation = ++extractionGeneration
    visionState.value = 'extracting'
    visionError.value = ''
    try {
      const extraction = await extractVisionSearch({
        references: references.value,
        userText: options.draftQuery.value,
        signal: controller.signal,
      })
      if (generation !== extractionGeneration) return
      options.submittedQuery.value = extraction.query
      options.draftQuery.value = extraction.query
      palette.value = extraction.palette ?? []
      atmosphereTags.value = extraction.atmosphere_tags ?? []
      selectedPalette.value = new Set(palette.value)
      selectedTags.value = new Set(atmosphereTags.value)
      visionState.value = 'ready'
      options.keepResultsOnNextLoad.value = true
      const currentRoute = route.fullPath
      await router.replace({
        path: USER_SEARCH_PATH,
        query: {
          q: extraction.query,
          ...(palette.value.length ? { palette: palette.value } : {}),
          ...(atmosphereTags.value.length ? { atmosphere_tag: atmosphereTags.value } : {}),
        },
      })
      if (generation !== extractionGeneration) return
      if (route.fullPath === currentRoute) await options.loadCatalog()
    } catch (error) {
      if (generation !== extractionGeneration || isAbortError(error)) return
      visionState.value = 'error'
      visionError.value = error instanceof Error ? error.message : 'Vision Search failed'
    } finally {
      if (extractionController === controller) extractionController = null
    }
  }

  function togglePalette(color: string) {
    const next = new Set(selectedPalette.value)
    if (next.has(color)) next.delete(color)
    else next.add(color)
    selectedPalette.value = next
    scheduleRefinementSearch({
      palette: [...next],
      atmosphere_tag: [...selectedTags.value],
    })
  }

  function toggleTag(tag: string) {
    const next = new Set(selectedTags.value)
    if (next.has(tag)) next.delete(tag)
    else next.add(tag)
    selectedTags.value = next
    scheduleRefinementSearch({
      palette: [...selectedPalette.value],
      atmosphere_tag: [...next],
    })
  }

  function scheduleRefinementSearch(refinement: { palette: string[]; atmosphere_tag: string[] }) {
    options.keepResultsOnNextLoad.value = true
    if (refinementTimer) clearTimeout(refinementTimer)
    refinementTimer = setTimeout(() => {
      const query = { ...route.query } as Record<string, string | string[]>
      if (refinement.palette.length) query.palette = refinement.palette
      else delete query.palette
      if (refinement.atmosphere_tag.length) query.atmosphere_tag = refinement.atmosphere_tag
      else delete query.atmosphere_tag
      void router.replace({ query })
      refinementTimer = null
    }, 180)
  }

  function removeReference(key: string) {
    const reference = references.value.find((item) => item.key === key)
    if (reference?.file && reference.previewUrl.startsWith('blob:')) {
      URL.revokeObjectURL(reference.previewUrl)
    }
    references.value = references.value.filter((item) => item.key !== key)
    if (references.value.length > 0) markVisionStale()
    else clearVisionExtraction()
    if (references.value.length === 0) {
      void router.replace({
        path: USER_SEARCH_PATH,
        query: options.draftQuery.value ? { q: normalizeSearchQuery(options.draftQuery.value) } : {},
      })
    }
  }

  function handleFileInput(event: Event) {
    const input = event.target as HTMLInputElement
    const files = Array.from(input.files ?? [])
    input.value = ''
    if (files.length === 0) return
    const room = 4 - references.value.length
    if (files.length > room) {
      options.queryError.value = '最多添加 4 张参考图'
      return
    }
    let totalBytes = references.value.reduce((sum, item) => sum + (item.file?.size ?? 0), 0)
    for (const file of files) {
      if (!file.type.startsWith('image/')) {
        options.queryError.value = '仅支持图片格式'
        return
      }
      totalBytes += file.size
      if (totalBytes > 10 * 1024 * 1024) {
        options.queryError.value = '参考图总大小不能超过 10MB'
        return
      }
    }
    options.queryError.value = ''
    references.value = [
      ...references.value,
      ...files.map((file) => ({
        file,
        previewUrl: URL.createObjectURL(file),
        key: `local:${crypto.randomUUID()}`,
      })),
    ]
    markVisionStale()
  }

  function openFilePicker() {
    fileInput.value?.click()
  }

  function handleLibraryReference(asset: LibraryAssetView) {
    if (references.value.length >= 4) {
      options.queryError.value = '最多添加 4 张参考图'
      return
    }
    const key = searchReferenceKey(asset.provider, asset.source_id)
    if (references.value.some((reference) => reference.key === key)) return
    references.value = [
      ...references.value,
      toVisionReference(asset),
    ]
    markVisionStale()
    options.queryError.value = ''
  }

  function clearVisionExtraction() {
    cancelVisionExtraction()
    clearRefinementTimer()
    visionState.value = 'idle'
    visionError.value = ''
    palette.value = []
    atmosphereTags.value = []
    selectedPalette.value = new Set()
    selectedTags.value = new Set()
  }

  function markVisionStale() {
    clearVisionExtraction()
    options.keepResultsOnNextLoad.value = true
    const query = { ...route.query } as Record<string, string | string[]>
    delete query.palette
    delete query.atmosphere_tag
    void router.replace({ query })
  }

  function queryArrayValue(key: string): string[] {
    const value = route.query[key]
    return Array.isArray(value) ? value.map(String) : value ? [String(value)] : []
  }

  function clearRefinementTimer() {
    if (!refinementTimer) return
    clearTimeout(refinementTimer)
    refinementTimer = null
  }

  function cancelVisionExtraction() {
    extractionGeneration += 1
    extractionController?.abort()
    extractionController = null
  }

  function isAbortError(error: unknown) {
    return error instanceof DOMException && error.name === 'AbortError'
  }

  function syncFromRoute() {
    selectedPalette.value = new Set(queryArrayValue('palette'))
    selectedTags.value = new Set(queryArrayValue('atmosphere_tag'))
  }

  onBeforeUnmount(() => {
    clearRefinementTimer()
    cancelVisionExtraction()
    for (const reference of references.value) {
      if (reference.file && reference.previewUrl.startsWith('blob:')) {
        URL.revokeObjectURL(reference.previewUrl)
      }
    }
  })

  return {
    references,
    fileInput,
    visionState,
    visionError,
    palette,
    atmosphereTags,
    selectedPalette,
    selectedTags,
    runVisionSearch,
    togglePalette,
    toggleTag,
    removeReference,
    handleFileInput,
    openFilePicker,
    handleLibraryReference,
    clearVisionExtraction,
    syncFromRoute,
  }
}
