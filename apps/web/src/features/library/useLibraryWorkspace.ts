import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppToast } from '../../shared/composables/useAppToast'
import {
  createFolder,
  getLibraryAssets,
  getLibraryFolders,
  moveAsset,
  removeAsset,
  removeFolder,
  renameFolder,
  type LibraryAssetView,
  type LibraryFolderView,
} from './libraryResource'

export type LibraryFolderTab = {
  queryKey: string
  name: string
  assetCount: number
  isMutable: boolean
}

function normalizeFolderQuery(value: unknown): string {
  if (typeof value !== 'string' || !value) return 'all'
  if (value === 'quick-saves') return value
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? String(parsed) : 'all'
}

export function useLibraryWorkspace(): {
  folders: Ref<LibraryFolderView[]>
  folderTabs: ComputedRef<LibraryFolderTab[]>
  assets: Ref<LibraryAssetView[]>
  selectedFolder: Ref<string>
  loading: Ref<boolean>
  foldersLoading: Ref<boolean>
  errorMessage: Ref<string>
  hasMore: Ref<boolean>
  activeAsset: Ref<LibraryAssetView | null>
  selectFolder: (key: string) => void
  loadAssets: (reset?: boolean) => Promise<void>
  createNewFolder: (name: string) => Promise<void>
  renameSelectedFolder: (name: string) => Promise<void>
  deleteSelectedFolder: () => Promise<boolean>
  moveSelectedAsset: (asset: LibraryAssetView, folderKey: string) => Promise<boolean>
  requestRemove: (asset: LibraryAssetView) => Promise<void>
  retry: () => Promise<void>
  setActiveAsset: (asset: LibraryAssetView | null) => void
} {
  const route = useRoute()
  const router = useRouter()
  const toast = useAppToast()
  const folders = ref<LibraryFolderView[]>([])
  const assets = ref<LibraryAssetView[]>([])
  const loading = ref(true)
  const foldersLoading = ref(true)
  const errorMessage = ref('')
  const nextCursor = ref('')
  const hasMore = ref(false)
  const activeAsset = ref<LibraryAssetView | null>(null)
  let assetRequestID = 0
  let folderRequestID = 0
  const selectedFolder = ref(normalizeFolderQuery(route.query.folder))
  const folderTabs = computed(() =>
    folders.value.map((folder) => ({
      queryKey: folder.query_key,
      name: folder.name,
      assetCount: folder.asset_count,
      isMutable: !folder.is_system && !folder.is_virtual,
    })),
  )
  const selectedFolderRecord = computed(() =>
    folders.value.find((folder) => folder.query_key === selectedFolder.value),
  )

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
    const requestID = ++folderRequestID
    foldersLoading.value = true
    try {
      const nextFolders = await getLibraryFolders()
      if (requestID === folderRequestID) folders.value = nextFolders
    } catch (error) {
      if (requestID === folderRequestID) {
        errorMessage.value = error instanceof Error ? error.message : '加载收藏夹失败'
      }
    } finally {
      if (requestID === folderRequestID) foldersLoading.value = false
    }
  }

  function selectFolder(key: string) {
    selectedFolder.value = key
    void router.replace({ query: key === 'all' ? {} : { folder: key } })
    void loadAssets(true)
  }

  async function createNewFolder(name: string) {
    const folder = await createFolder(name)
    folders.value = [...folders.value, folder]
    selectFolder(folder.query_key)
    toast.success('Folder created')
  }

  async function renameSelectedFolder(name: string) {
    const folder = selectedFolderRecord.value
    if (!folder || folder.is_system || folder.is_virtual) return
    const updated = await renameFolder(folder.id, name)
    folders.value = folders.value.map((item) => (item.id === updated.id ? updated : item))
    toast.success('Folder renamed')
  }

  async function deleteSelectedFolder(): Promise<boolean> {
    const folder = selectedFolderRecord.value
    if (!folder || folder.is_system || folder.is_virtual) return false
    try {
      await removeFolder(folder.id)
      await loadFolders()
      selectFolder('quick-saves')
      toast.success('Folder deleted')
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '删除收藏夹失败')
      return false
    }
  }

  async function moveSelectedAsset(asset: LibraryAssetView, folderKey: string): Promise<boolean> {
    const folder = folders.value.find((item) => item.query_key === folderKey)
    if (!folder || folder.is_virtual || folder.id === asset.folder_id) return false
    try {
      const updated = await moveAsset(asset.id, folder.id)
      if (selectedFolder.value !== 'all' && selectedFolder.value !== folderKey) {
        assets.value = assets.value.filter((item) => item.id !== asset.id)
      } else {
        assets.value = assets.value.map((item) => (item.id === updated.id ? updated : item))
      }
      await loadFolders()
      toast.success('Asset moved')
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '移动素材失败')
      return false
    }
  }

  async function requestRemove(asset: LibraryAssetView) {
    if (!window.confirm('Remove this saved reference?')) return
    try {
      await removeAsset(asset.id)
      assets.value = assets.value.filter((item) => item.id !== asset.id)
      await loadFolders()
      toast.success('Removed from Library')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '移除素材失败')
    }
  }

  async function retry() {
    errorMessage.value = ''
    await Promise.all([loadFolders(), loadAssets(true)])
  }

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

  void Promise.all([loadFolders(), loadAssets(true)])

  return {
    folders,
    folderTabs,
    assets,
    selectedFolder,
    loading,
    foldersLoading,
    errorMessage,
    hasMore,
    activeAsset,
    selectFolder,
    loadAssets,
    createNewFolder,
    renameSelectedFolder,
    deleteSelectedFolder,
    moveSelectedAsset,
    requestRemove,
    retry,
    setActiveAsset: (asset) => {
      activeAsset.value = asset
    },
  }
}
