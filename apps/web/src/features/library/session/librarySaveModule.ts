import { ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import { uploadApi } from '../../../api/upload'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'
import { i18n } from '../../../core/i18n'
import { createUniqueId } from '../../../core/utils/ids'
import {
  toWorkspaceLibraryAssetInput,
  toWorkspaceLibraryMediaAssetInput,
  type LibrarySearchReference,
  type WorkspaceLibraryAsset,
  type WorkspaceLibraryAssetInput,
} from '../core/validators'
import {
  buildLibraryAssetSavedOutcome,
  LIBRARY_ALL_FOLDER_KEY,
  type LibraryGalleryEffect,
  type LibraryMutationPublisher,
} from './folderFacts'
import { workspaceLibraryApi, type WorkspaceLibraryRequestOptions } from './resource'

/**
 * Unified Library save orchestration.
 *
 * Search references, Canvas image bytes, and local files all save through
 * this module. Callers only supply the source and the target; the module owns
 * the operation lifecycle, media ingestion (upload once, retry reuses the
 * uploaded identity), per-step idempotency, batch mutation publication, and
 * notifications. Input gestures stay in their own adapters.
 */

export const LIBRARY_SAVE_UPLOAD_CONCURRENCY = 4

export type LibrarySaveSourceInput =
  | {
      source: 'external'
      reference: LibrarySearchReference
    }
  | ({ source: 'media'; title?: string | undefined } & (
      | { mediaAssetId: string }
      | {
          file: Blob | (() => Promise<File>)
          filename?: string | undefined
        }
    ))

export type LibrarySaveSubmitOptions = {
  /** Resolved target folder id; absent means unfiled (Quick Saves semantic). */
  folderId?: string | undefined
  galleryEffect?: LibraryGalleryEffect | undefined
  /** Caller-formatted success copy (owns folder-name knowledge); else default. */
  successMessage?: string | undefined
  signal?: AbortSignal | null | undefined
  newFolderName?: string | undefined
}

export type LibrarySaveItemStatus =
  'pending' | 'preparing' | 'uploading' | 'saving' | 'saved' | 'failed'

export type LibrarySaveItemState = {
  id: string
  kind: 'external' | 'media'
  title: string
  status: LibrarySaveItemStatus
  attempts: number
  error?: string | undefined
  asset?: WorkspaceLibraryAsset | undefined
}

export type LibrarySaveOperationStatus =
  'running' | 'succeeded' | 'partial' | 'failed' | 'abandoned'

export type LibrarySaveOperationState = {
  id: string
  status: LibrarySaveOperationStatus
  items: LibrarySaveItemState[]
}

export type LibrarySaveSubmitResult = {
  operationId: string
  status: LibrarySaveOperationStatus
  items: LibrarySaveItemState[]
  /** Present when this submit created the target folder. */
  createdFolderId?: string | undefined
}

export type LibrarySaveUploadPort = (
  file: File,
  workspaceId: string,
) => Promise<{ mediaAssetId: string }>

export type LibrarySaveAssetPort = (
  workspaceId: string,
  input: WorkspaceLibraryAssetInput,
  options?: WorkspaceLibraryRequestOptions,
) => Promise<WorkspaceLibraryAsset>

export type LibrarySaveNotifyPort = {
  success: (message: string) => void
  warning: (message: string) => void
  error: (message: string) => void
}

export type LibrarySaveModuleOptions = {
  workspaceId: MaybeRefOrGetter<string | null | undefined>
  mutationPublisher: Pick<LibraryMutationPublisher, 'publishMutations'>
  notify: LibrarySaveNotifyPort
  uploadFile?: LibrarySaveUploadPort | undefined
  saveAsset?: LibrarySaveAssetPort | undefined
}

export type LibrarySaveModule = {
  operations: Ref<LibrarySaveOperationState[]>
  submit: (
    inputs: LibrarySaveSourceInput | LibrarySaveSourceInput[],
    options?: LibrarySaveSubmitOptions,
  ) => Promise<LibrarySaveSubmitResult>
  retryOperation: (operationId: string) => Promise<LibrarySaveSubmitResult>
  /** Drops operation snapshots (workspace switch); in-flight steps self-abandon. */
  clear: () => void
  dispose: () => void
}

type ItemRuntime = {
  state: LibrarySaveItemState
  input: LibrarySaveSourceInput
  uploadFile?: File | undefined
  mediaAssetId?: string | undefined
  mediaTitle?: string | undefined
  saveKey: string
}

type OperationRuntime = {
  id: string
  workspaceId: string
  isBatch: boolean
  folderId?: string | undefined
  newFolderName?: string | undefined
  galleryEffect: LibraryGalleryEffect
  successMessage?: string | undefined
  signal?: AbortSignal | null | undefined
  createdFolderId?: string | undefined
  completion?: Promise<LibrarySaveSubmitResult> | undefined
  items: ItemRuntime[]
}

function defaultUploadFile(file: File, workspaceId: string): Promise<{ mediaAssetId: string }> {
  return uploadApi.upload(file, workspaceId).then((result) => {
    const mediaAssetId = result.asset_id?.trim()
    if (!mediaAssetId) throw new Error(i18n.global.t('common.errors.uploadProcessingFailed'))
    return { mediaAssetId }
  })
}

async function runWithConcurrency(
  tasks: Array<() => Promise<void>>,
  concurrency: number,
): Promise<void> {
  let nextIndex = 0
  const workers = Array.from({ length: Math.min(concurrency, tasks.length) }, async () => {
    while (true) {
      const task = tasks[nextIndex++]
      if (!task) return
      await task()
    }
  })
  await Promise.all(workers)
}

/**
 * Target-folder priority shared by every save entry: an explicitly hovered
 * folder pill wins, then the selected folder, then unfiled. `all` (or an
 * unknown pill) never invents a folder: unfiled assets surface under `all`,
 * which is the Quick Saves semantic (no real Quick Saves folder id exists).
 */
export function resolveLibrarySaveFolderId(
  hoveredFolderKey: string | null | undefined,
  selectedFolderKey: string | null | undefined,
  isFolderKnown: (key: string) => boolean,
): { kind: 'folder'; folderId?: string | undefined } | { kind: 'invalid' } {
  if (hoveredFolderKey != null) {
    if (hoveredFolderKey === LIBRARY_ALL_FOLDER_KEY) return { kind: 'folder' }
    return isFolderKnown(hoveredFolderKey)
      ? { kind: 'folder', folderId: hoveredFolderKey }
      : { kind: 'invalid' }
  }
  if (
    selectedFolderKey != null &&
    selectedFolderKey !== LIBRARY_ALL_FOLDER_KEY &&
    isFolderKnown(selectedFolderKey)
  ) {
    return { kind: 'folder', folderId: selectedFolderKey }
  }
  return { kind: 'folder' }
}

export function createLibrarySaveModule(options: LibrarySaveModuleOptions): LibrarySaveModule {
  const operations = ref<LibrarySaveOperationState[]>([])
  const runtimes = new Map<string, OperationRuntime>()
  let isDisposed = false
  const uploadPort = options.uploadFile ?? defaultUploadFile
  const savePort = options.saveAsset ?? workspaceLibraryApi.saveAsset

  function currentWorkspaceId(): string | null {
    try {
      const value = toValue(options.workspaceId)
      return typeof value === 'string' && value ? value : null
    } catch {
      return null
    }
  }

  function isCurrent(runtime: OperationRuntime): boolean {
    return (
      !isDisposed &&
      runtimes.get(runtime.id) === runtime &&
      currentWorkspaceId() === runtime.workspaceId &&
      !runtime.signal?.aborted
    )
  }

  function syncOperation(id: string, status: LibrarySaveOperationStatus): LibrarySaveSubmitResult {
    const runtime = runtimes.get(id)
    if (!runtime || !isCurrent(runtime)) {
      if (runtime) {
        runtimes.delete(id)
        operations.value = operations.value.filter((operation) => operation.id !== id)
      }
      return { operationId: id, status: 'abandoned', items: [] }
    }
    const items = runtime.items.map((item) => ({ ...item.state }))
    const createdFolderId = runtime.createdFolderId
    const existing = operations.value.find((operation) => operation.id === id)
    const snapshot: LibrarySaveOperationState = { id, status, items }
    operations.value = existing
      ? operations.value.map((operation) => (operation.id === id ? snapshot : operation))
      : [...operations.value, snapshot]
    return {
      operationId: id,
      status,
      items,
      ...(createdFolderId == null ? {} : { createdFolderId }),
    }
  }

  function itemTitle(input: LibrarySaveSourceInput): string {
    if (input.source === 'external') {
      return input.reference.title || i18n.global.t('search.fallbackImageTitle')
    }
    return (
      input.title?.trim() ||
      ('filename' in input ? input.filename?.trim() : undefined) ||
      i18n.global.t('library.states.savedReference')
    )
  }

  function buildItem(input: LibrarySaveSourceInput): ItemRuntime {
    const title = itemTitle(input)
    const state: LibrarySaveItemState = {
      id: createUniqueId(),
      kind: input.source,
      title,
      status: 'pending',
      attempts: 0,
    }
    if (input.source === 'external') {
      return { state, input, saveKey: '' }
    }
    if ('mediaAssetId' in input) {
      return {
        state,
        input,
        mediaAssetId: input.mediaAssetId,
        mediaTitle: input.title?.trim() || undefined,
        saveKey: '',
      }
    }
    const filename = input.filename?.trim() || `${title || 'image'}.jpg`
    const file =
      typeof input.file === 'function'
        ? undefined
        : input.file instanceof File
          ? input.file
          : new File([input.file], filename, { type: input.file.type || 'image/jpeg' })
    return {
      state,
      input,
      uploadFile: file,
      mediaTitle: input.title?.trim() || undefined,
      saveKey: '',
    }
  }

  async function uploadItemMedia(runtime: OperationRuntime, item: ItemRuntime): Promise<boolean> {
    if (item.input.source !== 'media' || item.mediaAssetId != null) return true
    if (!item.uploadFile && 'file' in item.input && typeof item.input.file === 'function') {
      item.state.status = 'preparing'
      syncOperation(runtime.id, 'running')
      item.uploadFile = await item.input.file()
      if (!isCurrent(runtime)) return false
    }
    if (!item.uploadFile) throw new Error(i18n.global.t('library.errors.saveAssetFailed'))
    item.state.status = 'uploading'
    syncOperation(runtime.id, 'running')
    const uploaded = await uploadPort(item.uploadFile, runtime.workspaceId)
    // The uploaded identity is kept: retries never re-upload bytes and
    // never create a second Media asset.
    item.mediaAssetId = uploaded.mediaAssetId
    return isCurrent(runtime)
  }

  function buildItemSaveInput(
    runtime: OperationRuntime,
    item: ItemRuntime,
  ): WorkspaceLibraryAssetInput {
    if (item.input.source === 'external') {
      return toWorkspaceLibraryAssetInput(item.input.reference, runtime.folderId)
    }
    return toWorkspaceLibraryMediaAssetInput(
      item.mediaAssetId as string,
      item.mediaTitle == null ? {} : { title: item.mediaTitle },
      runtime.folderId,
    )
  }

  async function runItem(runtime: OperationRuntime, item: ItemRuntime): Promise<void> {
    if (!isCurrent(runtime)) return
    item.state.attempts += 1
    try {
      if (!(await uploadItemMedia(runtime, item)) || !isCurrent(runtime)) return
      item.state.status = 'saving'
      syncOperation(runtime.id, 'running')
      if (!item.saveKey) item.saveKey = `library:asset-save:${createUniqueId()}`
      // Same step replays the same idempotency key: a lost save response
      // converges on the server-committed asset instead of duplicating it.
      const asset = await savePort(runtime.workspaceId, buildItemSaveInput(runtime, item), {
        idempotencyKey: item.saveKey,
        ...(runtime.signal ? { signal: runtime.signal } : {}),
      })
      if (!isCurrent(runtime)) return
      item.state.status = 'saved'
      item.state.error = undefined
      item.state.asset = asset
      syncOperation(runtime.id, 'running')
    } catch (error) {
      if (!isCurrent(runtime)) return
      item.state.status = 'failed'
      item.state.error = userFacingErrorCopy(
        error,
        i18n.global.t('library.errors.saveAssetFailed'),
        // The publish gate is the only save path rendering
        // board_forbidden: surface the publish-specific denial copy.
        { board_forbidden: 'library.errors.saveRestrictedDenied' },
      )
      syncOperation(runtime.id, 'running')
    }
  }

  async function driveOperation(operationId: string): Promise<LibrarySaveSubmitResult> {
    const runtime = runtimes.get(operationId)
    if (!runtime) throw new Error(i18n.global.t('library.errors.saveAssetFailed'))
    if (!isCurrent(runtime)) return syncOperation(operationId, 'abandoned')
    if (runtime.newFolderName && runtime.createdFolderId == null) {
      try {
        const created = await workspaceLibraryApi.createFolder(
          runtime.workspaceId,
          runtime.newFolderName,
          {
            idempotencyKey: `library:folder-create:${runtime.id}`,
            ...(runtime.signal ? { signal: runtime.signal } : {}),
          },
        )
        if (!isCurrent(runtime)) return syncOperation(operationId, 'abandoned')
        runtime.createdFolderId = created.id
        runtime.folderId = created.id
      } catch (error) {
        if (!isCurrent(runtime)) return syncOperation(operationId, 'abandoned')
        const message = userFacingErrorCopy(
          error,
          i18n.global.t('library.errors.saveAssetFailed'),
          { board_forbidden: 'library.errors.saveRestrictedDenied' },
        )
        for (const item of runtime.items) {
          if (item.state.status === 'saved') continue
          item.state.status = 'failed'
          item.state.error = message
        }
        return finishOperation(operationId)
      }
    }
    syncOperation(operationId, 'running')
    const pending = runtime.items.filter((item) => item.state.status !== 'saved')
    await runWithConcurrency(
      pending.map((item) => () => runItem(runtime, item)),
      LIBRARY_SAVE_UPLOAD_CONCURRENCY,
    )
    if (!isCurrent(runtime)) return syncOperation(operationId, 'abandoned')
    await options.mutationPublisher.publishMutations(
      pending.flatMap((item) =>
        item.state.asset
          ? [buildLibraryAssetSavedOutcome(item.state.asset, runtime.galleryEffect)]
          : [],
      ),
    )
    if (!isCurrent(runtime)) return syncOperation(operationId, 'abandoned')
    return finishOperation(operationId)
  }

  function executeOperation(runtime: OperationRuntime): Promise<LibrarySaveSubmitResult> {
    runtime.completion = driveOperation(runtime.id).finally(() => {
      runtime.completion = undefined
    })
    return runtime.completion
  }

  function finishOperation(operationId: string): LibrarySaveSubmitResult {
    const runtime = runtimes.get(operationId)
    const items = runtime?.items ?? []
    const savedCount = items.filter((item) => item.state.status === 'saved').length
    const failedCount = items.filter((item) => item.state.status === 'failed').length
    const status: LibrarySaveOperationStatus =
      savedCount > 0 && failedCount > 0 ? 'partial' : savedCount > 0 ? 'succeeded' : 'failed'
    const result = syncOperation(operationId, status)
    if (status === 'succeeded') {
      options.notify.success(
        runtime?.successMessage ??
          (runtime?.isBatch
            ? i18n.global.t('search.toasts.savedBatchSuccess', { count: savedCount })
            : i18n.global.t('search.savePopover.savedToLibraryToast')),
      )
    } else if (status === 'partial') {
      options.notify.warning(
        i18n.global.t('search.toasts.savedBatchPartial', {
          successCount: savedCount,
          failureCount: failedCount,
        }),
      )
    } else {
      const firstError = items.find((item) => item.state.error)?.state.error
      options.notify.error(firstError ?? i18n.global.t('library.errors.saveAssetFailed'))
    }
    return result
  }

  async function submit(
    inputs: LibrarySaveSourceInput | LibrarySaveSourceInput[],
    submitOptions: LibrarySaveSubmitOptions = {},
  ): Promise<LibrarySaveSubmitResult> {
    if (isDisposed) throw new Error(i18n.global.t('library.errors.saveAssetFailed'))
    const list = Array.isArray(inputs) ? inputs : [inputs]
    if (list.length === 0) throw new Error(i18n.global.t('library.errors.saveAssetFailed'))
    const workspaceId = currentWorkspaceId()
    if (!workspaceId) throw new Error(i18n.global.t('library.errors.saveAssetFailed'))
    const operationId = createUniqueId()
    const runtime: OperationRuntime = {
      id: operationId,
      workspaceId,
      isBatch: Array.isArray(inputs),
      ...(submitOptions.folderId == null ? {} : { folderId: submitOptions.folderId }),
      newFolderName: submitOptions.newFolderName?.trim() || undefined,
      galleryEffect: submitOptions.galleryEffect ?? 'reload-current',
      ...(submitOptions.successMessage == null
        ? {}
        : { successMessage: submitOptions.successMessage }),
      ...(submitOptions.signal == null ? {} : { signal: submitOptions.signal }),
      items: list.map(buildItem),
    }
    runtimes.set(operationId, runtime)
    syncOperation(operationId, 'running')
    try {
      return await executeOperation(runtime)
    } catch (error) {
      if (!isCurrent(runtime)) return syncOperation(operationId, 'abandoned')
      throw error
    }
  }

  async function retryOperation(operationId: string): Promise<LibrarySaveSubmitResult> {
    const runtime = runtimes.get(operationId)
    if (!runtime) throw new Error(i18n.global.t('library.errors.saveAssetFailed'))
    if (!isCurrent(runtime)) return syncOperation(operationId, 'abandoned')
    if (runtime.completion) return runtime.completion
    // Retries replay the pinned scope: uploaded Media identities and save
    // idempotency keys are reused, bytes are never re-sent.
    const failed = runtime.items.filter((item) => item.state.status === 'failed')
    // Replaying a settled operation must not duplicate saves, publications,
    // or notifications.
    if (failed.length === 0) {
      const isSaved = runtime.items.every((item) => item.state.status === 'saved')
      return syncOperation(operationId, isSaved ? 'succeeded' : 'failed')
    }
    for (const item of failed) {
      item.state.status = 'pending'
      item.state.error = undefined
    }
    runtime.galleryEffect = 'reload-current'
    return executeOperation(runtime)
  }

  function dispose(): void {
    isDisposed = true
    stopWorkspaceWatch()
    clear()
  }

  function clear(): void {
    runtimes.clear()
    operations.value = []
  }

  const stopWorkspaceWatch = watch(currentWorkspaceId, clear, { flush: 'sync' })

  return { operations, submit, retryOperation, clear, dispose }
}
