import { createRequestFingerprint } from '../core/visionReferencePolicy'
import {
  VISION_REFERENCE_MAX_BYTES,
  VISION_REFERENCE_MAX_COUNT,
  isVisionReferenceFile,
} from '../../../core/media/visionIngressPolicy'
import { computed, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import { useRouter, type Router } from 'vue-router'
import { USER_SEARCH_PATH } from '../../workspace/routePaths'
import { formatIdeaAmount } from '../../../core/i18n/userIdeaCurrency'
import { useAppToast } from '../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'
import { i18n } from '../../../core/i18n'
import { createUniqueId } from '../../../core/utils/ids'
import { workspaceUploadApi } from '@/api/media/upload'
import { libraryAssetKeyFor, type LibraryAssetView } from '../../library'
import type { SearchAuthAction } from '../core/authAction'
import {
  clearCachedVisionReferences,
  getCachedVisionReferences,
  setCachedVisionReferences,
} from './adapters/visionReferencePersistence'
import {
  searchReferenceKey,
  searchResultToVisionReference,
  toVisionReference,
} from './adapters/libraryBridge'
import { type SearchResult, type VisionReference } from '../core/models'
import { isSearchQueryWithinLimit, normalizeSearchQuery } from '../core/queryPolicy'
import { workspaceSearchApi } from './adapters/workspace'
import { createVisionReferenceCollection } from './visionReferenceCollection'
import { fetchVisionSearchPrice } from './adapters/visionBillingResource'

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
  workspaceId?: MaybeRefOrGetter<string | null | undefined>
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
    : cachedReferences
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

  async function stageVisionFiles(
    workspaceId: string,
    files: File[],
    key: string,
    signal: AbortSignal,
  ): Promise<string[]> {
    const uploadIds: string[] = []
    for (const [index, file] of files.entries()) {
      const subtle = globalThis.crypto?.subtle
      if (!subtle) throw new Error(i18n.global.t('search.visionSearchFailed'))
      const digest = await subtle.digest('SHA-256', await file.arrayBuffer())
      const checksumSha256 = [...new Uint8Array(digest)]
        .map((byte) => byte.toString(16).padStart(2, '0'))
        .join('')
      const mimeType = file.type || 'application/octet-stream'
      const session = await workspaceUploadApi.createSession(
        workspaceId,
        { filename: file.name, mimeType, sizeBytes: file.size, checksumSha256, mediaKind: 'image' },
        { idempotencyKey: `${key}:upload:create:${index}`, signal },
      )
      await workspaceUploadApi.stageBytes(workspaceId, session.id, file, {
        contentType: mimeType,
        idempotencyKey: `${key}:upload:bytes:${index}`,
        signal,
      })
      await workspaceUploadApi.completeSession(
        workspaceId,
        session.id,
        { checksumSha256, sizeBytes: file.size, mimeType, mediaKind: 'image' },
        { idempotencyKey: `${key}:upload:complete:${index}`, signal },
      )
      uploadIds.push(session.id)
    }
    return uploadIds
  }

  async function createVisionQuery(
    workspaceId: string,
    query: string,
    key: string,
    signal: AbortSignal,
  ) {
    const uploadIds = await stageVisionFiles(
      workspaceId,
      references.value.flatMap((reference) => (reference.file ? [reference.file] : [])),
      key,
      signal,
    )
    const libraryAssetIds = references.value.flatMap((reference) =>
      reference.libraryAssetID ? [reference.libraryAssetID] : [],
    )
    const imageUrls = references.value.flatMap((reference) => {
      const url = reference.externalReference?.preview_url?.trim() ?? ''
      return url ? [url] : []
    })
    return workspaceSearchApi.createVisionQuery(
      { workspaceId, userText: query, libraryAssetIds, uploadIds, imageUrls },
      { idempotencyKey: key, signal },
    )
  }

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
        const workspaceId = (toValue(options.workspaceId) ?? '').trim()
        if (!workspaceId) throw new Error(i18n.global.t('search.visionSearchFailed'))
        const extraction = await createVisionQuery(
          workspaceId,
          query,
          idempotencyKey,
          controller.signal,
        )
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
    const room = VISION_REFERENCE_MAX_COUNT - referenceSlotCount.value
    if (files.length > room) {
      toast.warning(i18n.global.t('search.errors.maxReferenceImages'))
      return
    }
    let totalBytes = collection.currentBytes()
    for (const file of files) {
      if (!isVisionReferenceFile(file)) {
        toast.warning(i18n.global.t('search.errors.onlyImageFilesSupported'))
        return
      }
      totalBytes += file.size
      if (totalBytes > VISION_REFERENCE_MAX_BYTES) {
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
    const key = libraryAssetKeyFor(asset)
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
    // Workspace-scoped quote: without a selection the price stays
    // unavailable (same outcome as the old failing request, minus the 404).
    const workspaceId = (toValue(options.workspaceId) ?? '').trim()
    if (!workspaceId) return
    const controller = new AbortController()
    priceController = controller
    void fetchVisionSearchPrice(workspaceId, controller.signal)
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

  watch(collection.persistableReferences, (current) =>
    setCachedVisionReferences(current, cacheScope),
  )
  watch(() => references.value.map((reference) => reference.key), refreshPrice, { immediate: true })

  return {
    references,
    refreshReferencePresentation: collection.refreshPresentation,
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
