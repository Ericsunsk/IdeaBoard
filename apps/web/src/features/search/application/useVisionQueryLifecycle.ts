import {
  MAX_REFERENCE_BYTES,
  MAX_REFERENCE_COUNT,
  createRequestFingerprint,
} from '../domain/visionReferencePolicy'
import { computed, ref, watch, type Ref } from 'vue'
import { useRouter, type Router } from 'vue-router'
import { USER_SEARCH_PATH } from '../../../shared/constants/routePaths'
import { formatIdeaAmount } from '../../../shared/i18n/userIdeaCurrency'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { i18n } from '../../../shared/i18n'
import { createUniqueId } from '../../../shared/utils/ids'
import type { LibraryAssetView } from '../../library'
import type { SearchAuthAction } from '../domain/authAction'
import {
  clearCachedVisionReferences,
  getCachedVisionReferences,
  setCachedVisionReferences,
} from '../infrastructure/visionReferencePersistence'
import {
  searchReferenceKey,
  searchResultToVisionReference,
  toVisionReference,
} from '../infrastructure/libraryBridge'
import { type SearchResult, type VisionReference } from '../domain/models'
import { isSearchQueryWithinLimit, normalizeSearchQuery } from '../domain/queryPolicy'
import { extractVisionSearch } from '../infrastructure/resource'
import { createVisionReferenceCollection } from './visionReferenceCollection'
import { fetchVisionSearchPrice } from '../infrastructure/visionBillingResource'

type VisionQueryLifecycleOptions = {
  draftQuery: Ref<string>
  submittedQuery: Ref<string>
  queryError: Ref<string>
  keepResultsOnNextLoad: Ref<boolean>
  loadResults: () => Promise<void>
  isAuthenticated: () => boolean
  requireLogin: (action: SearchAuthAction) => void
  cacheScope?: string | undefined
  initialReferences?: VisionReference[] | undefined
  router?: Router | undefined
  toast?: ReturnType<typeof useAppToast> | undefined
}

/**
 * Owns the full paid Vision query lifecycle: reference ingress, cached edit
 * drafts, price feedback, extraction, and result handoff.
 */
export function useVisionQueryLifecycle(options: VisionQueryLifecycleOptions) {
  const router = options.router ?? useRouter()
  const toast = options.toast ?? useAppToast()
  const cacheScope = options.cacheScope ?? 'anonymous'
  const cachedReferences = getCachedVisionReferences(cacheScope)
  // A pending login continuation is explicit user intent. Never merge it with a
  // persisted edit draft that belongs to the identity being entered.
  const initialReferences = options.initialReferences?.length
    ? options.initialReferences
    : cachedReferences.map((cached) => ({
        key: cached.key,
        previewUrl: cached.previewUrl,
        libraryAssetID: cached.libraryAssetID,
        externalReference: cached.externalReference,
      }))
  const collection = createVisionReferenceCollection(initialReferences)
  const { references } = collection
  const fileInput = ref<HTMLInputElement | null>(null)
  const state = ref<'idle' | 'extracting' | 'ready'>('idle')
  const error = ref('')
  const price = ref<number | null>(null)
  const referenceSlotCount = collection.slotCount
  const canAddReference = collection.canAdd
  const buttonTitle = computed(() => {
    if (state.value === 'extracting') return i18n.global.t('search.buttons.searching')
    if (references.value.length === 0 || price.value === null)
      return i18n.global.t('search.buttons.search')
    return i18n.global.t('search.buttons.searchWithPrice', {
      price: price.value === 0 ? i18n.global.t('search.priceFree') : formatIdeaAmount(price.value),
    })
  })
  let extractionController: AbortController | null = null
  let priceController: AbortController | null = null
  let extractionGeneration = 0
  let idempotencyKey: string | null = null
  let idempotencyFingerprint: string | null = null
  let activeExtraction: { fingerprint: string; promise: Promise<void> } | null = null

  function resetExtraction() {
    cancelExtraction()
    idempotencyKey = null
    idempotencyFingerprint = null
    state.value = 'idle'
    error.value = ''
  }

  function markStale() {
    resetExtraction()
    options.keepResultsOnNextLoad.value = true
  }

  function cancelExtraction() {
    extractionGeneration += 1
    extractionController?.abort()
    extractionController = null
  }

  function submit(): Promise<void> {
    const query = normalizeSearchQuery(options.draftQuery.value)
    if (!isSearchQueryWithinLimit(query)) {
      options.queryError.value = i18n.global.t('search.queryTooLong')
      return Promise.resolve()
    }
    options.queryError.value = ''
    if (references.value.length === 0) return Promise.resolve()
    if (!options.isAuthenticated()) {
      options.requireLogin({ kind: 'vision-search', query })
      return Promise.resolve()
    }

    const fingerprint = createRequestFingerprint(query, references.value)
    if (activeExtraction?.fingerprint === fingerprint) return activeExtraction.promise
    if (idempotencyFingerprint !== fingerprint) resetExtraction()
    idempotencyFingerprint = fingerprint
    idempotencyKey ??= createUniqueId()
    const controller = new AbortController()
    extractionController = controller
    const generation = ++extractionGeneration
    state.value = 'extracting'
    error.value = ''
    const promise = (async () => {
      try {
        const extraction = await extractVisionSearch({
          references: references.value,
          userText: query,
          idempotencyKey,
          signal: controller.signal,
        })
        if (generation !== extractionGeneration) return
        if (extraction.status !== 'succeeded') {
          state.value = 'idle'
          error.value = i18n.global.t('search.stillProcessing')
          return
        }
        const extractedQuery = extraction.query?.trim()
        if (!extractedQuery) {
          state.value = 'idle'
          error.value = i18n.global.t('search.invalidVisionResult')
          return
        }
        options.submittedQuery.value = extractedQuery
        state.value = 'ready'
        options.keepResultsOnNextLoad.value = true
        await options.loadResults()
      } catch (cause) {
        if (generation !== extractionGeneration || isAbortError(cause)) return
        state.value = 'idle'
        error.value = userFacingErrorCopy(cause, i18n.global.t('search.visionSearchFailed'))
      } finally {
        if (extractionController === controller) extractionController = null
      }
    })()
    activeExtraction = { fingerprint, promise }
    void promise.then(
      () => {
        if (activeExtraction?.promise === promise) activeExtraction = null
      },
      () => {
        if (activeExtraction?.promise === promise) activeExtraction = null
      },
    )
    return promise
  }

  async function resumeAfterAuthentication(): Promise<void> {
    // Cached references restore an edit draft only. This method is called after
    // the explicit in-memory Vision action has survived login.
    await submit()
  }

  function removeReference(key: string) {
    collection.remove(key)
    if (references.value.length > 0) {
      markStale()
      return
    }
    resetExtraction()
    options.submittedQuery.value = normalizeSearchQuery(options.draftQuery.value)
    void router.replace({
      path: USER_SEARCH_PATH,
      query: options.draftQuery.value ? { q: normalizeSearchQuery(options.draftQuery.value) } : {},
    })
  }

  function clearReferences() {
    collection.clear()
    clearCachedVisionReferences(cacheScope)
    resetExtraction()
  }

  function handleFileInput(event: Event) {
    const input = event.target as HTMLInputElement
    const files = Array.from(input.files ?? [])
    input.value = ''
    if (files.length === 0) return
    const room = MAX_REFERENCE_COUNT - referenceSlotCount.value
    if (files.length > room) {
      toast.warning(i18n.global.t('search.errors.maxReferenceImages'))
      return
    }
    let totalBytes = collection.currentBytes()
    for (const file of files) {
      if (!file.type.startsWith('image/')) {
        toast.warning(i18n.global.t('search.errors.onlyImageFilesSupported'))
        return
      }
      totalBytes += file.size
      if (totalBytes > MAX_REFERENCE_BYTES) {
        toast.warning(i18n.global.t('search.errors.maxReferenceSizeExceeded'))
        return
      }
    }
    options.queryError.value = ''
    for (const file of files) {
      collection.add({
        file,
        previewUrl: URL.createObjectURL(file),
        key: `local:${createUniqueId()}`,
      })
    }
    markStale()
  }

  function openFilePicker() {
    fileInput.value?.click()
  }

  function addLibraryAsset(asset: LibraryAssetView) {
    const key = searchReferenceKey(asset.provider, asset.source_id)
    if (collection.has(key)) return
    if (!canAddReference.value) {
      toast.warning(i18n.global.t('search.errors.maxReferenceImages'))
      return
    }
    collection.add(toVisionReference(asset))
    markStale()
    options.queryError.value = ''
  }

  function addSearchResult(result: SearchResult) {
    const key = searchReferenceKey(result.provider, result.id)
    if (collection.has(key)) return
    if (!canAddReference.value) {
      toast.warning(i18n.global.t('search.errors.maxReferenceImages'))
      return
    }
    collection.add(searchResultToVisionReference(result))
    markStale()
    options.queryError.value = ''
  }

  function addSearchResults(results: SearchResult[]) {
    for (const result of results) {
      addSearchResult(result)
    }
  }

  function refreshPrice() {
    priceController?.abort()
    priceController = null
    price.value = null
    if (!options.isAuthenticated() || references.value.length === 0) return
    const controller = new AbortController()
    priceController = controller
    void fetchVisionSearchPrice(controller.signal)
      .then((nextPrice) => {
        if (!controller.signal.aborted) price.value = nextPrice
      })
      .catch(() => {
        // Price feedback is supplementary; the paid submit path remains authoritative.
      })
  }

  function transferReferences(): VisionReference[] {
    cancelExtraction()
    return collection.take()
  }

  function dispose() {
    priceController?.abort()
    priceController = null
    cancelExtraction()
    collection.clear()
  }

  watch(references, (current) => setCachedVisionReferences(current, cacheScope), { deep: true })
  watch(references, refreshPrice, { deep: true, immediate: true })

  return {
    references,
    referenceSlotCount,
    canAddReference,
    fileInput,
    state,
    error,
    price,
    buttonTitle,
    submit,
    resumeAfterAuthentication,
    removeReference,
    clearReferences,
    handleFileInput,
    openFilePicker,
    addLibraryAsset,
    addSearchResult,
    addSearchResults,
    resetExtraction,
    transferReferences,
    dispose,
  }
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}
