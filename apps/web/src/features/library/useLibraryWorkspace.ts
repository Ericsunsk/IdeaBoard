import { computed, onScopeDispose, ref, watch, type ComputedRef, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppToast } from '../../shared/composables/useAppToast'
import {
  createFolder,
  getLibraryAssets,
  libraryAssetKey,
  moveAsset,
  removeAsset,
  removeFolder,
  renameFolder,
  type LibraryAssetView,
  type LibraryFolderView,
} from './libraryResource'
import {
  createLibraryFolderFacts,
  createLibraryMutationPublisher,
  type LibraryFolderFacts,
  type LibraryMutationPublisher,
} from './libraryFolderFacts'

export type LibraryFolderTab = {
  queryKey: string
  name: string
  assetCount: number
  isMutable: boolean
}

export type LibraryWorkspaceOptions = {
  syncRoute?: boolean
  autoLoad?: boolean
  initialFolder?: string
  folderFacts?: LibraryFolderFacts
  mutationPublisher?: LibraryMutationPublisher
}

function normalizeFolderQuery(value: unknown): string {
  if (typeof value !== 'string' || !value) return 'all'
  if (value === 'quick-saves') return value
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? String(parsed) : 'all'
}

export function useLibraryWorkspace(options: LibraryWorkspaceOptions = {}): {
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
  createNewFolder: (name: string) => Promise<void>
  renameFolder: (key: string, name: string) => Promise<void>
  deleteFolder: (key: string) => Promise<boolean>
  moveSelectedAsset: (assetID: number, folderKey: string) => Promise<boolean>
  requestRemove: (asset: LibraryAssetView) => Promise<void>
  retry: () => Promise<void>
  setActiveAsset: (asset: LibraryAssetView | null) => void
} {
  const route = useRoute()
  const router = useRouter()
  const syncRoute = options.syncRoute ?? true
  const autoLoad = options.autoLoad ?? true
  const toast = useAppToast()
  const ownedFolderFacts = !options.folderFacts
  const folderFacts = options.folderFacts ?? createLibraryFolderFacts()
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
    normalizeFolderQuery(syncRoute ? route.query.folder : options.initialFolder),
  )
  const folderTabs = computed(() => {
    const tabs = folders.value.map((folder) => ({
      queryKey: folder.query_key,
      name: folder.name,
      assetCount: folder.asset_count,
      isMutable: !folder.is_system && !folder.is_virtual,
    }))
    if (!tabs.some((tab) => tab.queryKey === 'all')) {
      tabs.unshift({ queryKey: 'all', name: 'All Assets', assetCount: 0, isMutable: false })
    }
    if (!tabs.some((tab) => tab.queryKey === 'quick-saves')) {
      tabs.splice(1, 0, {
        queryKey: 'quick-saves',
        name: 'Quick Saves',
        assetCount: 0,
        isMutable: false,
      })
    }
    return tabs
  })
  function assetParams(folderKey: string, cursor?: string) {
    const numericFolderID = Number(folderKey)
    return {
      folder_id:
        Number.isSafeInteger(numericFolderID) && numericFolderID > 0
          ? numericFolderID
          : (folderKey as 'all' | 'quick-saves'),
      cursor: cursor || undefined,
      limit: 24,
    }
  }

  async function loadAssets(reset = true) {
    if (!reset && (loading.value || !hasMore.value)) return
    const requestID = ++assetRequestID
    const requestFolder = selectedFolder.value
    const requestCursor = reset ? '' : nextCursor.value
    if (reset) {
      loading.value = true
      nextCursor.value = ''
      errorMessage.value = ''
    } else {
      loading.value = true
    }
    try {
      const page = await getLibraryAssets(assetParams(requestFolder, requestCursor))
      if (requestID !== assetRequestID || requestFolder !== selectedFolder.value) return
      assets.value = reset ? page.items : [...assets.value, ...page.items]
      nextCursor.value = page.nextCursor
      hasMore.value = page.hasMore
    } catch (error) {
      if (requestID !== assetRequestID || requestFolder !== selectedFolder.value) return
      errorMessage.value = error instanceof Error ? error.message : '加载素材失败'
    } finally {
      if (requestID === assetRequestID) loading.value = false
    }
  }

  async function loadFolders() {
    errorMessage.value = ''
    const result = await folderFacts.load()
    if (result.status === 'error') errorMessage.value = result.error.message
  }

  function selectFolder(key: string) {
    selectedFolder.value = key
    if (syncRoute) void router.replace({ query: key === 'all' ? {} : { folder: key } })
    void loadAssets(true)
  }

  async function createNewFolder(name: string) {
    const folder = await createFolder(name)
    await mutationPublisher.publishMutation({
      kind: 'folder-created',
      affectedFolderKeys: [folder.query_key],
      galleryEffect: 'selection-will-reload',
    })
    selectFolder(folder.query_key)
    toast.success('Folder created')
  }

  async function renameFolderByKey(key: string, name: string) {
    const folder = folders.value.find((item) => item.query_key === key)
    if (!folder || folder.is_system || folder.is_virtual) return
    const updated = await renameFolder(folder.id, name)
    await mutationPublisher.publishMutation({
      kind: 'folder-renamed',
      affectedFolderKeys: [updated.query_key],
      galleryEffect: 'none',
    })
    toast.success('Folder renamed')
  }

  async function deleteFolder(key: string): Promise<boolean> {
    const folder = folders.value.find((item) => item.query_key === key)
    if (!folder || folder.is_system || folder.is_virtual) return false
    try {
      await removeFolder(folder.id)
      const selectedFolderWasDeleted = selectedFolder.value === key
      await mutationPublisher.publishMutation({
        kind: 'folder-deleted',
        affectedFolderKeys: [key],
        galleryEffect: selectedFolderWasDeleted ? 'selection-will-reload' : 'none',
      })
      if (selectedFolder.value === key) selectFolder('quick-saves')
      toast.success('Folder deleted')
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '删除收藏夹失败')
      return false
    }
  }

  async function moveSelectedAsset(assetID: number, folderKey: string): Promise<boolean> {
    const asset = assets.value.find((item) => item.id === assetID)
    if (!asset) return false
    const folder = folders.value.find((item) => item.query_key === folderKey)
    if (!folder || folder.is_virtual || folder.id === asset.folder_id) return false
    try {
      const updated = await moveAsset(asset.id, folder.id)
      if (selectedFolder.value !== 'all' && selectedFolder.value !== folderKey) {
        assets.value = assets.value.filter((item) => item.id !== asset.id)
      } else {
        assets.value = assets.value.map((item) => (item.id === updated.id ? updated : item))
      }
      await mutationPublisher.publishMutation({
        kind: 'asset-moved',
        affectedFolderKeys: ['all', String(asset.folder_id), folderKey],
        galleryEffect: 'none',
      })
      toast.success('Asset moved')
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '移动素材失败')
      return false
    }
  }

  async function requestRemove(asset: LibraryAssetView) {
    try {
      await removeAsset(asset.id)
      assets.value = assets.value.filter((item) => item.id !== asset.id)
      await mutationPublisher.publishMutation({
        kind: 'asset-removed',
        provider: asset.provider,
        sourceID: asset.source_id,
        assetKey: libraryAssetKey(asset.provider, asset.source_id),
        affectedFolderKeys: ['all', String(asset.folder_id)],
        galleryEffect: 'none',
      })
      toast.success('Removed from Library')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '移除素材失败')
    }
  }

  async function retry() {
    errorMessage.value = ''
    await Promise.all([loadFolders(), loadAssets(true)])
  }

  async function initialize() {
    await Promise.all([loadFolders(), loadAssets(true)])
  }

  if (syncRoute) {
    watch(
      () => route.query.folder,
      (value) => {
        const next = normalizeFolderQuery(value)
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
    createNewFolder,
    renameFolder: renameFolderByKey,
    deleteFolder,
    moveSelectedAsset,
    requestRemove,
    retry,
    setActiveAsset: (asset) => {
      activeAsset.value = asset
    },
  }
}
