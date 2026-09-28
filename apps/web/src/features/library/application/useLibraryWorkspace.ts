import {
  computed,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type ComputedRef,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { stringParam } from '../../../shared/router/queryParams'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { i18n } from '../../../shared/i18n'
import {
  assertLibraryFolderId,
  libraryAssetKey,
  requireWorkspaceId,
  type WorkspaceLibraryAsset,
} from '../domain/validators'
import { workspaceLibraryApi } from '../infrastructure/resource'
import { listAllWorkspaceLibraryFolders } from './libraryUseCases'
import {
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
  type LibraryFolderFacts,
  type LibraryFolderView,
  type LibraryMutationPublisher,
} from './folderFacts'
import type { LibraryAssetView } from '../domain/transfer'

export type LibraryFolderTab = {
  queryKey: string
  name: string
  isMutable: boolean
}

export const LIBRARY_ALL_FOLDER_KEY = 'all'

function isLibraryAllFolderKey(value: string): boolean {
  return value === LIBRARY_ALL_FOLDER_KEY
}

function asFolderKey(value: unknown): string {
  if (typeof value !== 'string' || !value) return LIBRARY_ALL_FOLDER_KEY
  if (value === LIBRARY_ALL_FOLDER_KEY) return value
  try {
    return assertLibraryFolderId(value)
  } catch {
    return LIBRARY_ALL_FOLDER_KEY
  }
}

export type LibraryWorkspaceOptions = {
  workspaceId: MaybeRefOrGetter<string | null | undefined>
  syncRoute?: boolean
  autoLoad?: boolean
  initialFolder?: string
  folderFacts?: LibraryFolderFacts
  mutationPublisher?: LibraryMutationPublisher
}

function resolveWorkspaceId(source: LibraryWorkspaceOptions['workspaceId']): string {
  return requireWorkspaceId(toValue(source))
}

async function fetchLibraryAssetPage(workspaceId: string, folderKey: string, cursor: string) {
  return workspaceLibraryApi.listAssets(workspaceId, {
    ...(cursor ? { cursor } : {}),
    pageSize: 24,
    ...(isLibraryAllFolderKey(folderKey) ? {} : { folderId: folderKey }),
  })
}

export function useLibraryWorkspace(options: LibraryWorkspaceOptions): {
  workspaceId: ComputedRef<string | null>
  folders: Readonly<Ref<LibraryFolderView[]>>
  folderTabs: ComputedRef<LibraryFolderTab[]>
  assets: Ref<LibraryAssetView[]>
  selectedFolder: Ref<string>
  loading: Ref<boolean>
  foldersLoading: Readonly<Ref<boolean>>
  errorMessage: Ref<string>
  hasMore: Ref<boolean>
  activeAsset: Ref<LibraryAssetView | null>
  selectFolder: (key: string) => void
  loadAssets: (reset?: boolean) => Promise<void>
  loadFolders: () => Promise<void>
  initialize: () => Promise<void>
  resetWorkspace: () => Promise<void>
  createNewFolder: (name: string) => Promise<void>
  renameFolder: (key: string, name: string) => Promise<void>
  deleteFolder: (key: string) => Promise<boolean>
  moveSelectedAsset: (assetID: string, folderKey: string) => Promise<boolean>
  requestRemove: (asset: LibraryAssetView) => Promise<void>
  retry: () => Promise<void>
  setActiveAsset: (asset: LibraryAssetView | null) => void
} {
  const route = useRoute()
  const router = useRouter()
  const syncRoute = options.syncRoute ?? true
  const autoLoad = options.autoLoad ?? true
  const toast = useAppToast()
  const workspaceId = computed(() => {
    try {
      return resolveWorkspaceId(options.workspaceId)
    } catch {
      return null
    }
  })
  const ownedFolderFacts = !options.folderFacts
  const folderFacts =
    options.folderFacts ??
    createLibraryFolderFacts(() =>
      listAllWorkspaceLibraryFolders(resolveWorkspaceId(options.workspaceId)),
    )
  const mutationPublisher = options.mutationPublisher ?? createLibraryMutationPublisher(folderFacts)
  const folders = folderFacts.folders
  const assets = ref<LibraryAssetView[]>([])
  const loading = ref(true)
  const errorMessage = ref('')
  const nextCursor = ref('')
  const hasMore = ref(false)
  const activeAsset = ref<LibraryAssetView | null>(null)
  let assetRequestID = 0
  const selectedFolder = ref(
    asFolderKey(syncRoute ? stringParam(route.query.folder) : options.initialFolder),
  )
  const folderTabs = computed(() => {
    const tabs = folders.value.map((folder) => ({
      queryKey: folder.id,
      name: folder.name,
      isMutable: true,
    }))
    if (!tabs.some((tab) => tab.queryKey === LIBRARY_ALL_FOLDER_KEY)) {
      tabs.unshift({
        queryKey: LIBRARY_ALL_FOLDER_KEY,
        name: i18n.global.t('library.folders.allAssets'),
        isMutable: false,
      })
    }
    return tabs
  })

  async function loadAssets(reset = true) {
    if (!reset && (loading.value || !hasMore.value)) return
    const requestID = ++assetRequestID
    const requestFolder = selectedFolder.value
    const requestCursor = reset ? '' : nextCursor.value
    loading.value = true
    if (reset) {
      nextCursor.value = ''
      errorMessage.value = ''
    }
    try {
      const wid = resolveWorkspaceId(options.workspaceId)
      const page = await fetchLibraryAssetPage(wid, requestFolder, requestCursor)
      if (requestID !== assetRequestID || requestFolder !== selectedFolder.value) return
      assets.value = reset ? page.items : [...assets.value, ...page.items]
      nextCursor.value = page.nextCursor
      hasMore.value = page.hasMore
    } catch (error) {
      if (requestID !== assetRequestID || requestFolder !== selectedFolder.value) return
      errorMessage.value = userFacingErrorCopy(
        error,
        i18n.global.t('library.errors.loadAssetsFailed'),
      )
    } finally {
      if (requestID === assetRequestID) loading.value = false
    }
  }

  async function loadFolders() {
    errorMessage.value = ''
    const result = await folderFacts.load()
    if (result.status === 'error') {
      errorMessage.value = userFacingErrorCopy(
        result.error,
        i18n.global.t('library.errors.loadFoldersFailed'),
      )
    }
  }

  function selectFolder(key: string) {
    const normalized = asFolderKey(key)
    selectedFolder.value = normalized
    if (syncRoute) {
      void router.replace({
        query: normalized === LIBRARY_ALL_FOLDER_KEY ? {} : { folder: normalized },
      })
    }
    void loadAssets(true)
  }

  async function createNewFolder(name: string) {
    const wid = resolveWorkspaceId(options.workspaceId)
    const folder = await workspaceLibraryApi.createFolder(wid, name)
    await mutationPublisher.publishMutation({
      kind: 'folder-created',
      affectedFolderKeys: [folder.id],
      galleryEffect: 'selection-will-reload',
    })
    selectFolder(folder.id)
    toast.success(i18n.global.t('library.toasts.folderCreated'))
  }

  async function renameFolderByKey(key: string, name: string) {
    const folder = folders.value.find((item) => item.id === key)
    if (!folder || isLibraryAllFolderKey(key)) return
    const wid = resolveWorkspaceId(options.workspaceId)
    const updated = await workspaceLibraryApi.renameFolder(wid, folder.id, name)
    await mutationPublisher.publishMutation({
      kind: 'folder-renamed',
      affectedFolderKeys: [updated.id],
      galleryEffect: 'none',
    })
    toast.success(i18n.global.t('library.toasts.folderRenamed'))
  }

  async function deleteFolderByKey(key: string): Promise<boolean> {
    const folder = folders.value.find((item) => item.id === key)
    if (!folder || isLibraryAllFolderKey(key)) return false
    try {
      const wid = resolveWorkspaceId(options.workspaceId)
      await workspaceLibraryApi.deleteFolder(wid, folder.id)
      const selectedFolderWasDeleted = selectedFolder.value === key
      await mutationPublisher.publishMutation({
        kind: 'folder-deleted',
        affectedFolderKeys: [key],
        galleryEffect: selectedFolderWasDeleted ? 'selection-will-reload' : 'none',
      })
      if (selectedFolder.value === key) selectFolder(LIBRARY_ALL_FOLDER_KEY)
      toast.success(i18n.global.t('library.toasts.folderDeleted'))
      return true
    } catch (error) {
      toast.error(userFacingErrorCopy(error, i18n.global.t('library.errors.deleteFolderFailed')))
      return false
    }
  }

  async function moveSelectedAsset(assetID: string, folderKey: string): Promise<boolean> {
    const asset = assets.value.find((item) => item.id === assetID)
    if (!asset) return false
    const normalized = asFolderKey(folderKey)
    if (isLibraryAllFolderKey(normalized)) return false
    const folder = folders.value.find((item) => item.id === normalized)
    if (!folder || folder.id === asset.folderId) return false
    try {
      const wid = resolveWorkspaceId(options.workspaceId)
      const updated = await workspaceLibraryApi.moveAsset(wid, asset.id, folder.id)
      if (selectedFolder.value !== LIBRARY_ALL_FOLDER_KEY && selectedFolder.value !== normalized) {
        assets.value = assets.value.filter((item) => item.id !== asset.id)
      } else {
        assets.value = assets.value.map((item) => (item.id === updated.id ? updated : item))
      }
      await mutationPublisher.publishMutation({
        kind: 'asset-moved',
        affectedFolderKeys: [
          LIBRARY_ALL_FOLDER_KEY,
          ...(asset.folderId ? [asset.folderId] : []),
          normalized,
        ],
        galleryEffect: 'none',
      })
      toast.success(i18n.global.t('library.toasts.assetMoved'))
      return true
    } catch (error) {
      toast.error(userFacingErrorCopy(error, i18n.global.t('library.errors.moveAssetFailed')))
      return false
    }
  }

  async function requestRemove(asset: LibraryAssetView) {
    try {
      const wid = resolveWorkspaceId(options.workspaceId)
      await workspaceLibraryApi.deleteAsset(wid, asset.id)
      assets.value = assets.value.filter((item) => item.id !== asset.id)
      await mutationPublisher.publishMutation({
        kind: 'asset-removed',
        provider: asset.provider,
        sourceID: asset.sourceId,
        assetKey: libraryAssetKey(asset.provider, asset.sourceId),
        affectedFolderKeys: [
          LIBRARY_ALL_FOLDER_KEY,
          ...(asset.folderId ? [asset.folderId] : []),
        ],
        galleryEffect: 'none',
      })
      toast.success(i18n.global.t('library.toasts.removedFromLibrary'))
    } catch (error) {
      toast.error(userFacingErrorCopy(error, i18n.global.t('library.errors.removeAssetFailed')))
    }
  }

  async function retry() {
    errorMessage.value = ''
    await Promise.all([loadFolders(), loadAssets(true)])
  }

  async function initialize() {
    await Promise.all([loadFolders(), loadAssets(true)])
  }

  async function resetWorkspace() {
    assetRequestID += 1
    selectedFolder.value = LIBRARY_ALL_FOLDER_KEY
    assets.value = []
    nextCursor.value = ''
    hasMore.value = false
    activeAsset.value = null
    folderFacts.reset()
    await initialize()
  }

  if (syncRoute) {
    watch(
      () => stringParam(route.query.folder),
      (value) => {
        const next = asFolderKey(value)
        if (next !== selectedFolder.value) {
          selectedFolder.value = next
          void loadAssets(true)
        }
      },
    )
  }

  if (autoLoad) void initialize()

  if (ownedFolderFacts) onScopeDispose(folderFacts.dispose)

  return {
    workspaceId,
    folders,
    folderTabs,
    assets,
    selectedFolder,
    loading,
    foldersLoading: folderFacts.loading,
    errorMessage,
    hasMore,
    activeAsset,
    selectFolder,
    loadAssets,
    loadFolders,
    initialize,
    resetWorkspace,
    createNewFolder,
    renameFolder: renameFolderByKey,
    deleteFolder: deleteFolderByKey,
    moveSelectedAsset,
    requestRemove,
    retry,
    setActiveAsset: (asset: WorkspaceLibraryAsset | null) => {
      activeAsset.value = asset
    },
  }
}
