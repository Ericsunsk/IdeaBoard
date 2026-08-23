import { onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { useRouter } from 'vue-router'
import { USER_SEARCH_PATH } from '../../shared/constants/routePaths'
import { useAppToast } from '../../shared/composables/useAppToast'
import type { LibraryAssetView } from '../library/libraryResource'
import {
  extractVisionSearch,
  normalizeSearchQuery,
  type SearchResult,
  type VisionReference,
} from './searchResource'
import { searchReferenceKey, toVisionReference } from './searchLibraryBridge'
import {
  clearCachedVisionReferences,
  getCachedVisionReferences,
  setCachedVisionReferences,
} from './searchCache'
import { createVisionReferenceCollection, MAX_REFERENCE_BYTES, MAX_REFERENCE_COUNT } from './visionReferenceCollection'

export function useVisionSearchFlow(options: {
  draftQuery: Ref<string>
  submittedQuery: Ref<string>
  queryError: Ref<string>
  keepResultsOnNextLoad: Ref<boolean>
  loadCatalog: () => Promise<void>
  cacheScope?: string
  initialReferences?: VisionReference[]
}) {
  const router = useRouter()
  const toast = useAppToast()
  const cacheScope = options.cacheScope ?? 'anonymous'
  const cachedRefs = options.initialReferences ?? getCachedVisionReferences(cacheScope)
  const collection = createVisionReferenceCollection(
    cachedRefs.map((cached) => ({
      key: cached.key,
      previewUrl: cached.previewUrl,
      libraryAssetID: cached.libraryAssetID,
    })),
  )
  const { references } = collection
  const fileInput = ref<HTMLInputElement | null>(null)
  const visionState = ref<'idle' | 'extracting' | 'ready'>('idle')
  const visionError = ref('')
  const hasPendingReferences = collection.hasPending
  const referenceSlotCount = collection.slotCount
  const canAddReference = collection.canAdd
  let extractionController: AbortController | null = null
  let extractionGeneration = 0
  let idempotencyKey: string | null = null

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
    idempotencyKey ??= crypto.randomUUID()
    try {
      const extraction = await extractVisionSearch({
        references: references.value,
        userText: options.draftQuery.value,
        idempotencyKey,
        signal: controller.signal,
      })
      if (generation !== extractionGeneration) return
      if (extraction.status !== 'succeeded') {
        visionState.value = 'idle'
        visionError.value = '搜索正在处理中，请稍后重试'
        return
      }
      const extractedQuery = extraction.query?.trim()
      if (!extractedQuery) {
        visionState.value = 'idle'
        visionError.value = '视觉搜索结果无效，请重试'
        return
      }
      options.submittedQuery.value = extractedQuery
      visionState.value = 'ready'
      options.keepResultsOnNextLoad.value = true
      await options.loadCatalog()
    } catch (error) {
      if (generation !== extractionGeneration || isAbortError(error)) return
      visionState.value = 'idle'
      visionError.value = error instanceof Error ? error.message : 'Vision Search failed'
    } finally {
      if (extractionController === controller) extractionController = null
    }
  }

  function removeReference(key: string) {
    collection.remove(key)
    if (references.value.length > 0) {
      markVisionStale()
    } else {
      clearVisionExtraction()
      options.submittedQuery.value = normalizeSearchQuery(options.draftQuery.value)
      void router.replace({
        path: USER_SEARCH_PATH,
        query: options.draftQuery.value ? { q: normalizeSearchQuery(options.draftQuery.value) } : {},
      })
    }
  }

  function clearAllReferences() {
    collection.clear()
    clearCachedVisionReferences(cacheScope)
    clearVisionExtraction()
  }

  watch(
    references,
    (current) => {
      setCachedVisionReferences(current, cacheScope)
    },
    { deep: true },
  )

  function handleFileInput(event: Event) {
    const input = event.target as HTMLInputElement
    const files = Array.from(input.files ?? [])
    input.value = ''
    if (files.length === 0) return
    const room = MAX_REFERENCE_COUNT - referenceSlotCount.value
    if (files.length > room) {
      toast.warning('You can add up to 4 reference images')
      return
    }
    let totalBytes = collection.currentBytes()
    for (const file of files) {
      if (!file.type.startsWith('image/')) {
        toast.warning('Only image files are supported')
        return
      }
      totalBytes += file.size
      if (totalBytes > MAX_REFERENCE_BYTES) {
        toast.warning('Total reference images size cannot exceed 10MB')
        return
      }
    }
    options.queryError.value = ''
    for (const file of files) {
      collection.add({
        file,
        previewUrl: URL.createObjectURL(file),
        key: `local:${crypto.randomUUID()}`,
      })
    }
    markVisionStale()
  }

  function openFilePicker() {
    fileInput.value?.click()
  }

  function handleLibraryReference(asset: LibraryAssetView) {
    const key = searchReferenceKey(asset.provider, asset.source_id)
    if (collection.has(key) || collection.isPending(key)) {
      return
    }
    if (!canAddReference.value) {
      toast.warning('You can add up to 4 reference images')
      return
    }
    collection.add(toVisionReference(asset))
    markVisionStale()
    options.queryError.value = ''
  }

  async function handleSearchResultReference(result: SearchResult) {
    const key = searchReferenceKey(result.provider, result.id)
    if (collection.has(key) || collection.isPending(key)) {
      return
    }
    if (!canAddReference.value) {
      toast.warning('You can add up to 4 reference images')
      return
    }
    collection.begin(key)
    markVisionStale()
    options.queryError.value = ''

    try {
      const responsePromise = fetch(result.preview_url, { cache: 'force-cache' }).catch(() => null)
      await decodePreviewImage(result.preview_url)
      if (!collection.isPending(key)) return
      collection.appendPreview(key, result.preview_url)

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
      if (!collection.isPending(key)) return
      if (!collection.reserveBytes(key, blob.size)) {
        throw new Error('reference previews exceed the request size limit')
      }
      const file = new File([blob], `${result.provider}-${result.id}.jpg`, {
        type: contentType,
      })
      collection.materialize(key, file)
    } catch {
      const isStillSelected = collection.fail(key)
      if (isStillSelected) options.queryError.value = '该图片无法作为搜索参考，请重试'
    } finally {
      collection.finish(key)
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
    return collection.isPending(key)
  }

  function clearVisionExtraction() {
    cancelVisionExtraction()
    idempotencyKey = null
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
    collection.clear()
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
