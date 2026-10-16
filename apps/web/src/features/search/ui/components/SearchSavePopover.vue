<template>
  <Popover :open="isOpen" @update:open="onOpenChange">
    <PopoverAnchor v-if="anchorEl" :reference="anchorEl" />
    <PopoverContent
      :class="searchSavePopoverContentClass"
      align="center"
      side="bottom"
      :side-offset="4"
    >
      <div class="w-48 select-none text-foreground" @click.stop>
        <div class="flex items-center gap-2 px-2 py-1.5">
          <Search class="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
          <input
            ref="searchInputRef"
            v-model="searchQuery"
            type="text"
            :placeholder="t('search.savePopover.searchPlaceholder')"
            :aria-label="t('search.savePopover.searchAria')"
            class="w-full bg-transparent text-xs font-normal text-foreground placeholder:text-muted-foreground outline-none"
            @keydown.enter.prevent="handleEnterCreate"
          />
        </div>

        <div v-if="folderFactsReady && canCreateFromQuery" class="my-0.5 px-1">
          <button
            type="button"
            class="flex w-full cursor-pointer items-center justify-between rounded-control border-none bg-primary/10 px-2 py-1.5 text-xs font-medium text-primary transition duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-primary/20"
            :disabled="saving || !folderFactsReady"
            @click="handleEnterCreate"
          >
            <span class="truncate">{{
              t('search.savePopover.createNamedFolder', { name: searchQuery.trim() })
            }}</span>
            <span class="font-mono text-admin-micro text-primary">↵</span>
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
            class="py-2.5 text-center text-xs text-muted-foreground"
          >
            {{ t('search.savePopover.noMatchingFolders') }}
          </div>

          <button
            v-if="showQuickSaves"
            type="button"
            class="group/fitem flex w-full cursor-pointer items-center justify-between rounded-control border-none bg-transparent px-2.5 py-1.5 text-left text-xs text-foreground transition duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted hover:text-primary disabled:opacity-50"
            :disabled="saving || !folderFactsReady"
            @click="saveTo()"
          >
            <div class="flex min-w-0 items-center gap-2">
              <Bookmark class="size-3.5 shrink-0 text-primary" aria-hidden="true" />
              <span class="truncate font-medium">{{ t('search.savePopover.quickSaves') }}</span>
            </div>
          </button>

          <button
            v-for="folder in filteredFolders"
            :key="folder.id"
            type="button"
            class="group/fitem flex w-full cursor-pointer items-center justify-between rounded-control border-none bg-transparent px-2.5 py-1.5 text-left text-xs text-foreground transition duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted hover:text-primary disabled:opacity-50"
            :disabled="saving || !folderFactsReady"
            @click="saveTo(folder.id)"
          >
            <div class="flex min-w-0 items-center gap-2">
              <Folder
                class="size-3.5 shrink-0 text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] group-hover/fitem:text-primary"
                aria-hidden="true"
              />
              <span class="truncate font-medium">{{ folder.name }}</span>
            </div>
          </button>
        </div>
      </div>
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
import { Bookmark, Folder, Search } from '@iconoir/vue'
import type { SearchResult } from '../../core/models'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { useSearchLibraryCoordinator } from '../../session/adapters/libraryCoordinator'
import type { SearchSavePopoverRequest } from '../browser/savePopoverContract'
import { resolveSavePopoverAnchor } from '../browser/savePopoverContract'

type OpenRequest = SearchSavePopoverRequest & { event: Event; anchor: HTMLElement }

const emit = defineEmits<{
  'auth-required': [result: SearchResult]
  'anchor-changed': [anchor: HTMLElement | null]
}>()

const toast = useAppToast()
const { t } = useI18n()
const isOpen = ref(false)
const anchorEl = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement>()
const activeRequest = shallowRef<OpenRequest | null>(null)
const saving = ref(false)
const searchQuery = ref('')
const isOpenOrOpening = ref(false)
const folderFactsReady = ref(false)
const library = useSearchLibraryCoordinator()
const { folders } = library.folderFacts
let pendingOpen: OpenRequest | null = null

const searchSavePopoverContentClass =
  'search-save-popover page-chrome-surface rounded-control p-1.5'

const displayFolders = computed(() =>
  folders.value.filter(
    (folder) =>
      folder.name.toLowerCase() !== 'all assets' && folder.name.toLowerCase() !== 'quick saves',
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
  anchorEl.value = request.anchor
  isOpen.value = true
  handlePopoverShow()
  void loadFolders()
}

function hide() {
  if (!isOpen.value && !isOpenOrOpening.value) return
  isOpen.value = false
  handlePopoverHide()
}

function onOpenChange(open: boolean) {
  if (isOpen.value === open) return
  isOpen.value = open
  if (open) {
    handlePopoverShow()
  } else {
    handlePopoverHide()
  }
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
  if (!request.isAuthenticated) {
    emit('auth-required', request.result)
    return
  }
  if (request.isSaved) {
    void handleUnsave(request)
    return
  }

  const anchor = resolveSavePopoverAnchor(event)
  if (!anchor) return
  const nextRequest: OpenRequest = { ...request, event, anchor }
  const activeKey =
    activeRequest.value && `${activeRequest.value.result.provider}:${activeRequest.value.result.id}`
  const nextKey = `${request.result.provider}:${request.result.id}`
  if (isOpenOrOpening.value) {
    if (activeKey === nextKey) {
      pendingOpen = null
      hide()
      return
    }
    pendingOpen = nextRequest
    hide()
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
  anchorEl.value = null
  emit('anchor-changed', null)
}

async function persist(folderId?: string, newFolderName?: string) {
  const request = activeRequest.value
  if (!request || saving.value) return
  saving.value = true
  try {
    // The shared save module owns the success/error notification (single
    // toast per operation); the popover only closes on success. The target
    // copy is computed up front: a new folder name is known before creation.
    const trimmedName = newFolderName?.trim()
    const targetName =
      trimmedName ||
      (folderId ? folders.value.find((folder) => folder.id === folderId)?.name : undefined) ||
      t('search.savePopover.quickSaves')
    await library.saveReference(
      request.result,
      folderId,
      newFolderName,
      t('search.savePopover.savedToast', { name: targetName }),
    )
    hide()
  } finally {
    saving.value = false
  }
}

async function saveTo(folderId?: string) {
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

defineExpose({ toggle, hide })
</script>
