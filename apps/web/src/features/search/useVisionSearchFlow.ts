import { computed, onBeforeUnmount, ref, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { USER_SEARCH_PATH } from '../../shared/constants/routePaths'
import type { LibraryAssetView } from '../library/libraryResource'
import {
  extractVisionSearch,
  normalizeSearchQuery,
  type SearchResult,
  type VisionReference,
} from './searchResource'
import { searchReferenceKey, toVisionReference } from './searchLibraryBridge'

const MAX_REFERENCE_COUNT = 4
const MAX_REFERENCE_BYTES = 10 * 1024 * 1024

export function useVisionSearchFlow(options: {
  draftQuery: Ref<string>
  submittedQuery: Ref<string>
  queryError: Ref<string>
  keepResultsOnNextLoad: Ref<boolean>
  loadCatalog: () => Promise<void>
}) {
  const route = useRoute()
  const router = useRouter()
  const references = ref<VisionReference[]>([])
  const fileInput = ref<HTMLInputElement | null>(null)
  const visionState = ref<'idle' | 'extracting' | 'ready'>('idle')
  const visionError = ref('')
  const pendingReferenceKeys = ref(new Set<string>())
  const pendingReferenceBytes = new Map<string, number>()
  const hasPendingReferences = computed(() => pendingReferenceKeys.value.size > 0)
  const referenceSlotCount = computed(() => {
    const keys = new Set(references.value.map((reference) => reference.key))
    for (const key of pendingReferenceKeys.value) keys.add(key)
    return keys.size
  })
  const canAddReference = computed(() => referenceSlotCount.value < MAX_REFERENCE_COUNT)
  let extractionController: AbortController | null = null
  let extractionGeneration = 0

  async function runVisionSearch() {
    if (hasPendingReferences.value) {
      options.queryError.value = '正在准备参考图，请稍候'
      return
    }
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
      visionState.value = 'ready'
      options.keepResultsOnNextLoad.value = true
      const currentRoute = route.fullPath
      await router.replace({
        path: USER_SEARCH_PATH,
        query: {
          q: extraction.query,
        },
      })
      if (generation !== extractionGeneration) return
      if (route.fullPath === currentRoute) await options.loadCatalog()
    } catch (error) {
      if (generation !== extractionGeneration || isAbortError(error)) return
      visionState.value = 'idle'
      visionError.value = error instanceof Error ? error.message : 'Vision Search failed'
    } finally {
      if (extractionController === controller) extractionController = null
    }
  }

  function removeReference(key: string) {
    const reference = references.value.find((item) => item.key === key)
    if (reference?.file && reference.previewUrl.startsWith('blob:')) {
      URL.revokeObjectURL(reference.previewUrl)
    }
    references.value = references.value.filter((item) => item.key !== key)
    const nextPendingKeys = new Set(pendingReferenceKeys.value)
    nextPendingKeys.delete(key)
    pendingReferenceKeys.value = nextPendingKeys
    pendingReferenceBytes.delete(key)
    if (references.value.length > 0) markVisionStale()
    else clearVisionExtraction()
    if (references.value.length === 0) {
      void router.replace({
        path: USER_SEARCH_PATH,
        query: options.draftQuery.value ? { q: normalizeSearchQuery(options.draftQuery.value) } : {},
      })
    }
  }

  function clearAllReferences() {
    for (const reference of references.value) {
      if (reference.file && reference.previewUrl.startsWith('blob:')) {
        URL.revokeObjectURL(reference.previewUrl)
      }
    }
    references.value = []
    pendingReferenceKeys.value = new Set()
    pendingReferenceBytes.clear()
    clearVisionExtraction()
    void router.replace({
      path: USER_SEARCH_PATH,
      query: options.draftQuery.value ? { q: normalizeSearchQuery(options.draftQuery.value) } : {},
    })
  }

  function handleFileInput(event: Event) {
    const input = event.target as HTMLInputElement
    const files = Array.from(input.files ?? [])
    input.value = ''
    if (files.length === 0) return
    const room = MAX_REFERENCE_COUNT - referenceSlotCount.value
    if (files.length > room) {
      options.queryError.value = '最多添加 4 张参考图'
      return
    }
    let totalBytes = currentReferenceBytes()
    for (const file of files) {
      if (!file.type.startsWith('image/')) {
        options.queryError.value = '仅支持图片格式'
        return
      }
      totalBytes += file.size
      if (totalBytes > MAX_REFERENCE_BYTES) {
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
    const key = searchReferenceKey(asset.provider, asset.source_id)
    if (
      references.value.some((reference) => reference.key === key) ||
      pendingReferenceKeys.value.has(key)
    ) {
      return
    }
    if (!canAddReference.value) {
      options.queryError.value = '最多添加 4 张参考图'
      return
    }
    references.value = [
      ...references.value,
      toVisionReference(asset),
    ]
    markVisionStale()
    options.queryError.value = ''
  }

  async function handleSearchResultReference(result: SearchResult) {
    const key = searchReferenceKey(result.provider, result.id)
    if (
      references.value.some((reference) => reference.key === key) ||
      pendingReferenceKeys.value.has(key)
    ) {
      return
    }
    if (!canAddReference.value) {
      options.queryError.value = '最多添加 4 张参考图'
      return
    }
    pendingReferenceKeys.value = new Set([...pendingReferenceKeys.value, key])
    markVisionStale()
    options.queryError.value = ''

    try {
      const responsePromise = fetch(result.preview_url, { cache: 'force-cache' }).catch(() => null)
      await decodePreviewImage(result.preview_url)
      if (!pendingReferenceKeys.value.has(key)) return
      references.value = [
        ...references.value,
        {
          previewUrl: result.preview_url,
          key,
        },
      ]

      const res = await responsePromise
      if (!res) throw new Error('reference preview request failed')
      if (!res.ok) throw new Error('reference preview request failed')
      const blob = await res.blob()
      const contentType = blob.type || res.headers.get('content-type')?.split(';', 1)[0] || ''
      if (
        !contentType.startsWith('image/') ||
        blob.size === 0 ||
        blob.size > MAX_REFERENCE_BYTES
      ) {
        throw new Error('reference preview is not a valid image')
      }
      if (!pendingReferenceKeys.value.has(key)) return
      if (currentReferenceBytes() + blob.size > MAX_REFERENCE_BYTES) {
        throw new Error('reference previews exceed the request size limit')
      }
      pendingReferenceBytes.set(key, blob.size)
      const file = new File([blob], `${result.provider}-${result.id}.jpg`, {
        type: contentType,
      })
      references.value = references.value.map((ref) =>
        ref.key === key ? { ...ref, file } : ref,
      )
      pendingReferenceBytes.delete(key)
    } catch {
      const isStillSelected =
        pendingReferenceKeys.value.has(key) ||
        references.value.some((reference) => reference.key === key)
      references.value = references.value.filter((ref) => ref.key !== key)
      if (isStillSelected) options.queryError.value = '该图片无法作为搜索参考，请重试'
    } finally {
      pendingReferenceBytes.delete(key)
      const nextPendingKeys = new Set(pendingReferenceKeys.value)
      nextPendingKeys.delete(key)
      pendingReferenceKeys.value = nextPendingKeys
      if (!hasPendingReferences.value && options.queryError.value === '正在准备参考图，请稍候') {
        options.queryError.value = ''
      }
    }
  }

  async function decodePreviewImage(src: string): Promise<void> {
    const image = new Image()
    image.src = src
    if (typeof image.decode === 'function') await image.decode()
  }

  function isReferencePending(key: string): boolean {
    return pendingReferenceKeys.value.has(key)
  }

  function currentReferenceBytes(): number {
    const storedBytes = references.value.reduce(
      (total, reference) => total + (reference.file?.size ?? 0),
      0,
    )
    const reservedBytes = [...pendingReferenceBytes.values()].reduce(
      (total, size) => total + size,
      0,
    )
    return storedBytes + reservedBytes
  }

  function clearVisionExtraction() {
    cancelVisionExtraction()
    visionState.value = 'idle'
    visionError.value = ''
  }

  function markVisionStale() {
    clearVisionExtraction()
    options.keepResultsOnNextLoad.value = true
  }

  function cancelVisionExtraction() {
    extractionGeneration += 1
    extractionController?.abort()
    extractionController = null
  }

  function isAbortError(error: unknown) {
    return error instanceof DOMException && error.name === 'AbortError'
  }

  onBeforeUnmount(() => {
    cancelVisionExtraction()
    for (const reference of references.value) {
      if (reference.file && reference.previewUrl.startsWith('blob:')) {
        URL.revokeObjectURL(reference.previewUrl)
      }
    }
  })

  return {
    references,
    hasPendingReferences,
    referenceSlotCount,
    canAddReference,
    isReferencePending,
    fileInput,
    visionState,
    visionError,
    runVisionSearch,
    removeReference,
    clearAllReferences,
    handleFileInput,
    openFilePicker,
    handleLibraryReference,
    handleSearchResultReference,
    clearVisionExtraction,
  }
}
