<template>
  <Popover
    ref="popover"
    unstyled
    :pt="searchSavePopoverPt"
    @show="handlePopoverShow"
    @hide="handlePopoverHide"
  >
    <div class="w-48 select-none text-board-page-text" @click.stop>
      <div class="flex items-center gap-2 px-2 py-1.5">
        <Search class="size-3.5 shrink-0 text-board-page-text-muted" aria-hidden="true" />
        <input
          ref="searchInputRef"
          v-model="searchQuery"
          type="text"
          :placeholder="t('search.savePopover.searchPlaceholder')"
          :aria-label="t('search.savePopover.searchAria')"
          class="w-full bg-transparent text-xs font-normal text-board-page-text placeholder:text-board-page-text-muted outline-none"
          @keydown.enter.prevent="handleEnterCreate"
        />
      </div>

      <div v-if="folderFactsReady && canCreateFromQuery" class="my-0.5 px-1">
        <button
          type="button"
          class="flex w-full cursor-pointer items-center justify-between rounded-lg border-none bg-board-primary/10 px-2 py-1.5 text-xs font-medium text-board-primary transition hover:bg-board-primary/20"
          :disabled="saving || !folderFactsReady"
          @click="handleEnterCreate"
        >
          <span class="truncate">{{
            t('search.savePopover.createNamedFolder', { name: searchQuery.trim() })
          }}</span>
          <span class="font-mono text-[10px] text-board-primary">↵</span>
        </button>
      </div>

      <div class="app-scrollbar max-h-48 space-y-0.5 overflow-y-auto pt-1">
        <div
          v-if="
            folderFactsReady &&
            !showQuickSaves &&
            filteredFolders.length === 0 &&
            !canCreateFromQuery
          "
          class="py-2.5 text-center text-xs text-board-page-text-muted"
        >
          {{ t('search.savePopover.noMatchingFolders') }}
        </div>

        <button
          v-if="showQuickSaves"
          type="button"
          class="group/fitem flex w-full cursor-pointer items-center justify-between rounded-lg border-none bg-transparent px-2.5 py-1.5 text-left text-xs text-board-page-text transition hover:bg-ib-surface-hover hover:text-board-primary disabled:opacity-50"
          :disabled="saving || !folderFactsReady"
          @click="saveTo()"
        >
          <div class="flex min-w-0 items-center gap-2">
            <Bookmark class="size-3.5 shrink-0 text-board-primary" aria-hidden="true" />
            <span class="truncate font-medium">{{ t('search.savePopover.quickSaves') }}</span>
          </div>
          <span class="shrink-0 font-sans text-[11px] text-board-page-text-muted">{{
            quickSavesCount
          }}</span>
        </button>

        <button
          v-for="folder in filteredFolders"
          :key="folder.id"
          type="button"
          class="group/fitem flex w-full cursor-pointer items-center justify-between rounded-lg border-none bg-transparent px-2.5 py-1.5 text-left text-xs text-board-page-text transition hover:bg-ib-surface-hover hover:text-board-primary disabled:opacity-50"
          :disabled="saving || !folderFactsReady"
          @click="saveTo(folder.id)"
        >
          <div class="flex min-w-0 items-center gap-2">
            <Folder
              class="size-3.5 shrink-0 text-board-page-text-muted transition-colors group-hover/fitem:text-board-primary"
              aria-hidden="true"
            />
            <span class="truncate font-medium">{{ folder.name }}</span>
          </div>
          <span class="shrink-0 font-sans text-[11px] text-board-page-text-muted">{{
            folder.asset_count
          }}</span>
        </button>
      </div>
    </div>
  </Popover>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import Popover from 'primevue/popover'
import { Bookmark, Folder, Search } from '@iconoir/vue'
import type { SearchResult } from '../searchResource'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { createPopoverMotionPt } from '../../../shared/motion/popoverMotion'
import type { PopoverPassThroughOptions } from 'primevue/popover'
import { useSearchLibraryCoordinator } from '../searchLibraryCoordinator'

export type SearchSavePopoverRequest = {
  result: SearchResult
  saved: boolean
  authenticated: boolean
}

export type SearchSavePopoverController = {
  toggle: (event: Event, request: SearchSavePopoverRequest) => void
  hide: () => void
}

type OpenRequest = SearchSavePopoverRequest & { event: Event; anchor: HTMLElement }
type PrimePopover = {
  show: (event: Event, target?: HTMLElement) => void
  hide: () => void
}

const emit = defineEmits<{
  'auth-required': [result: SearchResult]
  'anchor-changed': [anchor: HTMLElement | null]
}>()

const toast = useAppToast()
const { t } = useI18n()
const popover = ref<PrimePopover | null>(null)
const searchInputRef = ref<HTMLInputElement>()
const activeRequest = shallowRef<OpenRequest | null>(null)
const saving = ref(false)
const searchQuery = ref('')
const isOpenOrOpening = ref(false)
const folderFactsReady = ref(false)
const library = useSearchLibraryCoordinator()
const { folders } = library.folderFacts
let pendingOpen: OpenRequest | null = null

const searchSavePopoverPt = {
  ...createPopoverMotionPt(),
  root: {
    class:
      'search-save-popover ib-page-chrome-surface rounded-[var(--radius-ib-control)] p-1.5',
  },
} satisfies PopoverPassThroughOptions

const quickSavesFolder = computed(() =>
  folders.value.find((folder) => folder.name.toLowerCase() === 'quick saves' || folder.is_default),
)
const quickSavesCount = computed(() => quickSavesFolder.value?.asset_count ?? 0)
const displayFolders = computed(() =>
  folders.value.filter(
    (folder) =>
      !folder.is_system &&
      !folder.is_default &&
      folder.name.toLowerCase() !== 'all assets' &&
      folder.name.toLowerCase() !== 'quick saves',
  ),
)
const showQuickSaves = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return !query || 'quick saves'.includes(query)
})
const filteredFolders = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return query
    ? displayFolders.value.filter((folder) => folder.name.toLowerCase().includes(query))
    : displayFolders.value
})
const canCreateFromQuery = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return Boolean(
    query &&
    query !== 'quick saves' &&
    query !== 'all assets' &&
    !displayFolders.value.some((folder) => folder.name.toLowerCase() === query),
  )
})

async function loadFolders(): Promise<boolean> {
  const snapshot = await library.folderFacts.load()
  if (snapshot.status === 'current') {
    folderFactsReady.value = true
    return true
  }
  folderFactsReady.value = false
  if (snapshot.status === 'error') toast.error(t('search.savePopover.loadFoldersFailed'))
  return false
}

function show(request: OpenRequest) {
  if (!request.anchor.isConnected) return
  activeRequest.value = request
  isOpenOrOpening.value = true
  folderFactsReady.value = false
  emit('anchor-changed', request.anchor)
  popover.value?.show(request.event, request.anchor)
  void loadFolders()
}

async function handleUnsave(request: SearchSavePopoverRequest) {
  if (saving.value) return
  saving.value = true
  try {
    await library.removeReference(request.result.provider, request.result.id)
    toast.success(t('search.savePopover.removedToast'))
  } catch (error) {
    toast.error(userFacingErrorCopy(error, t('library.errors.removeAssetFailed')))
  } finally {
    saving.value = false
  }
}

function toggle(event: Event, request: SearchSavePopoverRequest) {
  if (saving.value) return
  if (!request.authenticated) {
    emit('auth-required', request.result)
    return
  }
  if (request.saved) {
    void handleUnsave(request)
    return
  }

  const anchor = event.currentTarget as HTMLElement | null
  if (!anchor) return
  const nextRequest: OpenRequest = { ...request, event, anchor }
  const activeKey =
    activeRequest.value && `${activeRequest.value.result.provider}:${activeRequest.value.result.id}`
  const nextKey = `${request.result.provider}:${request.result.id}`
  if (isOpenOrOpening.value) {
    if (activeKey === nextKey) {
      pendingOpen = null
      popover.value?.hide()
      return
    }
    pendingOpen = nextRequest
    popover.value?.hide()
    return
  }
  show(nextRequest)
}

function handlePopoverShow() {
  searchQuery.value = ''
  void nextTick(() => {
    searchInputRef.value?.focus()
  })
}

function handlePopoverHide() {
  searchQuery.value = ''
  isOpenOrOpening.value = false
  const nextRequest = pendingOpen
  pendingOpen = null
  activeRequest.value = null
  if (nextRequest) {
    void nextTick(() => show(nextRequest))
    return
  }
  emit('anchor-changed', null)
}

async function persist(folderId?: number, newFolderName?: string) {
  const request = activeRequest.value
  if (!request || saving.value) return
  saving.value = true
  try {
    await library.saveReference(request.result, folderId, newFolderName)
    popover.value?.hide()
    const targetName =
      newFolderName ||
      folders.value.find((folder) => folder.id === folderId)?.name ||
      t('search.savePopover.quickSaves')
    toast.success(t('search.savePopover.savedToast', { name: targetName }))
  } catch (error) {
    toast.error(userFacingErrorCopy(error, t('library.errors.saveAssetFailed')))
  } finally {
    saving.value = false
  }
}

async function saveTo(folderId?: number) {
  if (!folderFactsReady.value && !(await loadFolders())) return
  void persist(folderId)
}

async function handleEnterCreate() {
  const name = searchQuery.value.trim()
  const normalizedName = name.toLowerCase()
  if (!normalizedName || saving.value || normalizedName === 'all assets') return
  if (!(await loadFolders())) return
  if (normalizedName === 'quick saves') return saveTo()
  const existingFolder = displayFolders.value.find(
    (folder) => folder.name.toLowerCase() === normalizedName,
  )
  if (existingFolder) return saveTo(existingFolder.id)
  if (canCreateFromQuery.value) void persist(undefined, name)
}

defineExpose({ toggle, hide: () => popover.value?.hide() })
</script>
