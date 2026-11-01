<template>
  <Transition name="chrome-presence" appear @after-leave="handleAfterLeave">
    <aside
      v-show="open"
      v-library-drop
      :[LIBRARY_DROP_PANEL_ATTRIBUTE]="true"
      class="chrome-presence library-island border border-page-chrome-border bg-popover shadow-page-chrome z-admin-overlay flex flex-col overflow-hidden rounded-overlay p-5 text-foreground max-w-[calc(100vw-32px)]"
      :class="[
        isCanvasPresentation ? 'relative h-full w-full' : searchIslandClass,
        isPanelHovered || isNativeHovering || isExternalPanelHovered ? 'ring-2 ring-primary' : '',
      ]"
      :aria-label="t('library.title')"
      @click="closeFolderMenu(false)"
      @dragover="onNativeDragOver"
      @dragleave="onNativeDragLeave"
      @drop="onNativeFileDrop"
    >
      <!-- 🌟 1. 常驻顶栏控制区与灵感夹微胶囊 (支持用户主动折叠/展开分类胶囊) -->
      <div class="flex flex-col shrink-0 mb-2">
        <!-- 顶栏：标题 + 折叠微按钮 + 展开/收缩按钮 + 微圆关闭按钮 -->
        <div class="flex items-center justify-between pb-2 shrink-0">
          <div class="flex items-center gap-1.5">
            <h2 class="m-0 text-base font-semibold tracking-tight">{{ t('library.title') }}</h2>
            <button
              type="button"
              :class="[islandIconButtonClass, 'size-6']"
              :aria-label="
                isHeaderCollapsed
                  ? t('library.actions.showFolders')
                  : t('library.actions.hideFolders')
              "
              :title="
                isHeaderCollapsed
                  ? t('library.actions.showFolders')
                  : t('library.actions.hideFolders')
              "
              @click="isHeaderCollapsed = !isHeaderCollapsed"
            >
              <NavArrowDown v-if="isHeaderCollapsed" class="size-3.5" aria-hidden="true" />
              <NavArrowUp v-else class="size-3.5" aria-hidden="true" />
            </button>
          </div>
          <div class="flex items-center gap-1.5">
            <button
              v-if="!isCanvasPresentation"
              type="button"
              :class="[islandIconButtonClass, 'size-8']"
              :aria-label="isExpanded ? t('library.actions.collapse') : t('library.actions.expand')"
              :title="isExpanded ? t('library.actions.collapse') : t('library.actions.expand')"
              @click="isExpanded = !isExpanded"
            >
              <NavArrowLeft v-if="isExpanded" class="size-4" aria-hidden="true" />
              <NavArrowRight v-else class="size-4" aria-hidden="true" />
            </button>
            <button
              type="button"
              :class="[islandIconButtonClass, 'size-8']"
              :aria-label="t('library.actions.close')"
              :title="t('library.actions.close')"
              @click="emit('close')"
            >
              <Xmark class="size-4" aria-hidden="true" />
            </button>
          </div>
        </div>

        <!-- 灵感夹筛选微胶囊 (自然流式换行，支持拖拽落盘，零滚动条) -->
        <div
          v-show="!isHeaderCollapsed"
          class="transition-[opacity,transform] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
        >
          <div v-if="workspace.foldersLoading.value" class="py-1">
            <div class="h-7 w-full animate-pulse rounded-pill bg-muted"></div>
          </div>
          <div v-else class="py-1 shrink-0">
            <div
              class="flex flex-wrap items-center gap-1.5"
              role="tablist"
              :aria-label="t('library.folders.libraryFoldersAria')"
            >
              <div
                v-for="folder in workspace.folderTabs.value"
                :key="folder.folderKey"
                v-library-drop="folder.folderKey"
                :[LIBRARY_DROP_FOLDER_ATTRIBUTE]="folder.folderKey"
                class="group/folder-pill inline-flex h-7 shrink-0 items-center rounded-pill"
                @contextmenu.stop="openFolderMenu($event, folder)"
              >
                <button
                  type="button"
                  role="tab"
                  :aria-selected="workspace.selectedFolder.value === folder.folderKey"
                  :aria-haspopup="folder.isMutable ? 'menu' : undefined"
                  class="h-7 rounded-pill border px-2.5 text-xs font-medium transition-[transform,background-color,border-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] flex items-center gap-1.5 whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
                  :class="[
                    workspace.selectedFolder.value === folder.folderKey
                      ? 'border-transparent bg-primary text-primary-foreground shadow-xs'
                      : 'border-transparent bg-muted text-muted-foreground hover:bg-muted hover:text-foreground',
                    hoveredPillKey === folder.folderKey ||
                    nativeHoveredFolderKey === folder.folderKey
                      ? '!scale-105 !border-primary !bg-primary/10'
                      : '',
                  ]"
                  @click="selectFolder(folder.folderKey)"
                >
                  <ReportColumns
                    v-if="folder.folderKey === 'all'"
                    class="size-3 opacity-80"
                    aria-hidden="true"
                  />
                  <Folder v-else class="size-3 opacity-80" aria-hidden="true" />
                  <span>{{ folder.name }}</span>
                </button>
              </div>

              <!-- + New Folder 极简微胶囊 -->
              <button
                type="button"
                class="h-7 shrink-0 rounded-pill border border-dashed border-border bg-transparent px-2.5 text-xs font-medium text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-primary hover:text-primary flex items-center gap-1 whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
                @click="folderDialogVisible = true"
              >
                <Plus class="size-3" aria-hidden="true" />
                <span>{{ t('library.actions.new') }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 错误状态提示 -->
      <LoadErrorPanel
        v-if="workspace.errorMessage.value"
        class="mt-2 shrink-0"
        :message="workspace.errorMessage.value"
        :retry-label="t('common.actions.retry')"
        @retry="workspace.retry"
      />

      <div
        v-if="saveOperations.length"
        class="mb-2 max-h-28 shrink-0 overflow-y-auto"
        aria-live="polite"
      >
        <div
          v-for="operation in saveOperations"
          :key="operation.id"
          class="flex items-center justify-between gap-3 rounded-panel border border-border px-3 py-2 text-xs"
        >
          <span>
            {{ operation.items[0]?.title }} ·
            {{
              operation.status === 'running'
                ? t('common.states.saving')
                : t('search.toasts.savedBatchPartial', {
                    successCount: operation.savedCount,
                    failureCount: operation.failedCount,
                  })
            }}
            <span v-if="operation.status === 'running'">
              {{ operation.savedCount }}/{{ operation.items.length }}
            </span>
          </span>
          <button
            v-if="operation.status !== 'running'"
            type="button"
            class="shrink-0 cursor-pointer rounded-pill bg-primary px-3 py-1.5 font-semibold text-primary-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            @click="retrySave(operation.id)"
          >
            {{ t('common.actions.retry') }}
          </button>
        </div>
      </div>

      <LibraryAssetGallery
        ref="galleryRef"
        :assets="workspace.assets.value"
        :is-loading="workspace.loading.value"
        :error-message="workspace.errorMessage.value"
        :has-more="workspace.hasMore.value"
        :is-expanded="isExpanded"
        :is-canvas-presentation="isCanvasPresentation"
        :reference-keys="referenceKeys"
        :window-width="windowWidth"
        @scroll="closeFolderMenu"
        @load-more="workspace.loadAssets(false)"
        @open-asset="openAsset"
        @remove-asset="requestRemove"
        @add-to-canvas="emit('add-to-canvas', $event)"
        @select-reference="selectReference"
        @asset-pointer-down="handleAssetPointerDown"
      />
    </aside>
  </Transition>

  <!-- 用户 Folder 的右键操作菜单 -->
  <LibraryFolderContextMenu
    ref="folderContextMenuRef"
    :context-menu="contextMenu"
    :surface-class="appOverlayMenuSurfaceClass"
    :item-class="appOverlayMenuItemClass"
    :danger-item-class="appOverlayMenuDangerItemClass"
    @rename="openRenameFolder"
    @delete="deleteFolder"
    @escape="closeFolderMenu(true)"
  />

  <!-- 新建与重命名文件夹弹窗 -->
  <LibraryFolderDialogs
    v-model:create-open="folderDialogVisible"
    v-model:rename-open="renameDialogVisible"
    :rename-initial-name="renameInitialName"
    :on-create="handleCreateFolder"
    :on-rename="handleRenameFolder"
    @focus-scope-change="emit('focus-scope-change', $event)"
  />

  <!-- 沉浸式全景大图 Lightbox 弹窗 -->
  <LibraryAssetLightbox
    :asset="workspace.activeAsset.value"
    @close="workspace.setActiveAsset(null)"
  />

  <!-- 二次确认对话框 -->
  <ConfirmDialog
    surface="user"
    :is-visible="confirmDialog.visible.value"
    :title="confirmDialog.title.value"
    :message="confirmDialog.message.value"
    :confirm-label="confirmDialog.confirmLabel.value"
    :intent="confirmDialog.intent.value"
    :is-loading="confirmDialog.loading.value"
    @update:is-visible="confirmDialog.close"
    @confirm="confirmDialog.confirm"
  />
</template>

<script setup lang="ts">
import { onClickOutside, onKeyStroke } from '@vueuse/core'
import { resolveMediaDisplayUrl } from '../../../core/media/mediaDisplay'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Folder,
  NavArrowDown,
  NavArrowLeft,
  NavArrowRight,
  NavArrowUp,
  Plus,
  ReportColumns,
  Xmark,
} from '@iconoir/vue'
import { useLibraryWorkspace, type LibraryFolderTab } from '../session/useLibraryWorkspace'
import type { LibraryFolderFacts, LibraryMutationPublisher } from '../session/folderFacts'
import { toCanvasLibraryImportDescriptor, type LibraryAssetView } from '../core/transfer'
import { createPointerDropTargets } from '../../../core/interaction/pointerDropTargets'
import { startPointerDrag } from '../../../core/interaction/pointerDrag'
import type { CanvasLibraryImportDescriptor } from '../core/transfer'
import ConfirmDialog from '../../../core/components/ConfirmDialog.vue'
import LoadErrorPanel from '../../../core/components/LoadErrorPanel.vue'
import { useConfirmDialog } from '../../../core/composables/useConfirmDialog'
import {
  appOverlayMenuDangerItemClass,
  appOverlayMenuItemClass,
  appOverlayMenuSurfaceClass,
} from '../../../theme/appOverlayMenu'
import { deleteFolderConfirmation, removeAssetConfirmation } from './confirmations'
import LibraryAssetGallery from './LibraryAssetGallery.vue'
import LibraryAssetLightbox from './LibraryAssetLightbox.vue'
import LibraryFolderDialogs from './LibraryFolderDialogs.vue'
import LibraryFolderContextMenu from './LibraryFolderContextMenu.vue'
import { useLibraryIslandDnD } from '../session/useLibraryIslandDnD'
import { useLibraryNativeFileDrop } from './useLibraryNativeFileDrop'
import {
  LIBRARY_DROP_FOLDER_ATTRIBUTE,
  LIBRARY_DROP_PANEL_ATTRIBUTE,
  type LibraryDropHit,
} from './libraryDropHitTest'
import { useAppToast } from '../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'
import type { LibraryDragPayload, LibraryDropTargets } from '../core/drag'
import type { LibrarySaveModule } from '../session/librarySaveModule'
import { useLibraryDropTargets } from './useLibraryDropTargets'

const props = defineProps<{
  open: boolean
  workspaceId: string | null
  folderFacts: LibraryFolderFacts
  mutationPublisher: LibraryMutationPublisher
  saveModule: LibrarySaveModule
  referenceKeys?: string[]
  presentation: 'search' | 'canvas'
  dropTargets?: LibraryDropTargets
}>()

const emit = defineEmits<{
  close: []
  'reference-selected': [LibraryAssetView]
  'add-to-canvas': [CanvasLibraryImportDescriptor]
  'focus-scope-change': [focused: boolean]
}>()

const isExpanded = defineModel<boolean>('expanded', { default: false })
const { t } = useI18n()

const islandIconButtonClass =
  'grid shrink-0 place-items-center rounded-pill bg-muted text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer'

const isCanvasPresentation = computed(() => props.presentation === 'canvas')
const searchIslandClass = computed(() =>
  isExpanded.value
    ? 'fixed top-[84px] bottom-4 left-4 sm:bottom-6 sm:left-6 w-[calc(100vw-32px)] sm:w-[calc(100vw-48px)]'
    : 'fixed top-[84px] bottom-4 left-4 sm:bottom-6 sm:left-6 w-[calc(100vw-32px)] sm:w-[350px]',
)

const confirmDialog = useConfirmDialog()
const toast = useAppToast()
const saveOperations = computed(() =>
  props.saveModule.operations.value
    .filter((operation) => operation.status !== 'succeeded' && operation.status !== 'abandoned')
    .map((operation) => ({
      ...operation,
      savedCount: operation.items.filter((item) => item.status === 'saved').length,
      failedCount: operation.items.filter((item) => item.status === 'failed').length,
    })),
)

async function retrySave(operationId: string) {
  try {
    await props.saveModule.retryOperation(operationId)
  } catch (error) {
    toast.error(userFacingErrorCopy(error, t('library.errors.saveAssetFailed')))
  }
}

const workspace = useLibraryWorkspace({
  workspaceId: () => props.workspaceId,
  syncRoute: false,
  autoLoad: false,
  folderFacts: props.folderFacts,
  mutationPublisher: props.mutationPublisher,
})

const dropTargets = props.dropTargets ?? createPointerDropTargets<LibraryDragPayload>()
const hoveredPillKey = ref<string | null>(null)
const isExternalPanelHovered = ref(false)
const { receiveDrop } = useLibraryIslandDnD({
  workspace,
  saveModule: props.saveModule,
})
const {
  isNativeHovering,
  nativeHoveredFolderKey,
  onNativeDragOver,
  onNativeDragLeave,
  onNativeDrop: onNativeFileDrop,
} = useLibraryNativeFileDrop({
  isOpen: () => props.open,
  canReceive: () => true,
  selectedFolder: () => workspace.selectedFolder.value,
  isFolderKnown: (key) => workspace.folders.value.some((folder) => folder.id === key),
  selectFolder: (key) => selectFolder(key),
  saveModule: props.saveModule,
  notifyWarning: (message) => toast.warning(message),
})

/**
 * External (non-Pointer-adapter) drag hover, e.g. Canvas node drags. Only
 * touches the island's own hover state — never Canvas state.
 */
function setExternalDragHover(hit: LibraryDropHit | null) {
  hoveredPillKey.value = hit?.kind === 'folder' ? hit.folderKey : null
  isExternalPanelHovered.value = hit?.kind === 'panel'
}

defineExpose({
  setExternalDragHover,
  getSelectedFolderKey: () => workspace.selectedFolder.value,
  isFolderKnown: (key: string) => workspace.folders.value.some((folder) => folder.id === key),
})
let disposeAssetDrag: (() => void) | null = null
function handleAssetPointerDown(event: PointerEvent, asset: LibraryAssetView) {
  disposeAssetDrag?.()
  disposeAssetDrag = startPointerDrag(event, {
    payload: { kind: 'asset', asset: toCanvasLibraryImportDescriptor(asset) },
    previewUrl: resolveMediaDisplayUrl(asset.display, 256),
    targets: dropTargets,
  })
}
watch(
  () => [props.workspaceId, props.open],
  () => disposeAssetDrag?.(),
  { flush: 'sync' },
)
onBeforeUnmount(() => disposeAssetDrag?.())

const { directive: vLibraryDrop, isPanelHovered } = useLibraryDropTargets({
  targets: () => dropTargets,
  isOpen: () => props.open,
  onFolderHover: (key) => {
    hoveredPillKey.value = key
  },
  onDrop: (payload, folderKey) => {
    void receiveDrop(payload, folderKey ?? workspace.selectedFolder.value)
  },
})

const initialized = ref(false)
const isHeaderCollapsed = ref(false)
const windowWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1200)
const galleryRef = ref<InstanceType<typeof LibraryAssetGallery> | null>(null)
const folderContextMenuRef = ref<InstanceType<typeof LibraryFolderContextMenu> | null>(null)
const contextMenu = ref<{
  folder: LibraryFolderTab
  x: number
  y: number
  trigger: HTMLElement | null
} | null>(null)

onClickOutside(
  () =>
    folderContextMenuRef.value?.$el instanceof HTMLElement ? folderContextMenuRef.value.$el : null,
  () => {
    if (contextMenu.value) {
      closeFolderMenu(false)
    }
  },
)

onKeyStroke('Escape', (event) => {
  if (contextMenu.value) {
    event.preventDefault()
    event.stopPropagation()
    closeFolderMenu(true)
  }
})

// 文件夹弹窗状态
const folderDialogVisible = ref(false)
const renameDialogVisible = ref(false)
const renameFolderKey = ref('')
const renameInitialName = ref('')

function openAsset(asset: LibraryAssetView) {
  workspace.setActiveAsset(asset)
}

function selectReference(asset: LibraryAssetView) {
  if (isCanvasPresentation.value) return
  emit('reference-selected', asset)
}

function selectFolder(folderKey: string) {
  closeFolderMenu(false)
  galleryRef.value?.scrollToTop()
  workspace.selectFolder(folderKey)
}

function closeFolderMenu(restoreFocus = false) {
  const trigger = contextMenu.value?.trigger
  contextMenu.value = null
  if (restoreFocus && trigger?.isConnected) {
    void requestAnimationFrame(() => trigger.focus())
  }
}

function openFolderMenu(event: MouseEvent, folder: LibraryFolderTab) {
  if (!folder.isMutable) return
  event.preventDefault()
  const trigger =
    event.currentTarget instanceof HTMLElement
      ? event.currentTarget.querySelector<HTMLElement>('button')
      : null
  const menuWidth = 160
  const menuHeight = 84
  const edge = 8
  const maxX = Math.max(edge, window.innerWidth - menuWidth - edge)
  const maxY = Math.max(edge, window.innerHeight - menuHeight - edge)
  contextMenu.value = {
    folder,
    x: Math.min(Math.max(event.clientX, edge), maxX),
    y: Math.min(Math.max(event.clientY, edge), maxY),
    trigger,
  }
}

function openRenameFolder(folder: LibraryFolderTab) {
  // Let the follow-up Dialog own focus; restoring the folder trigger here
  // would race its autofocus and move focus back behind the modal.
  closeFolderMenu(false)
  renameFolderKey.value = folder.folderKey
  renameInitialName.value = folder.name
  renameDialogVisible.value = true
}

async function handleCreateFolder(name: string) {
  try {
    await workspace.createNewFolder(name)
  } catch (error) {
    toast.error(userFacingErrorCopy(error, t('library.errors.createFolderFailed')))
    throw error
  }
}

async function handleRenameFolder(name: string) {
  if (!renameFolderKey.value) return
  try {
    await workspace.renameFolder(renameFolderKey.value, name)
  } catch (error) {
    toast.error(userFacingErrorCopy(error, t('library.errors.renameFolderFailed')))
    throw error
  }
}

async function deleteFolder(folder: LibraryFolderTab) {
  // The confirmation Dialog takes over focus after the context menu closes.
  closeFolderMenu(false)
  if (!folder.isMutable) return
  try {
    const confirmed = await confirmDialog.confirmAsync(deleteFolderConfirmation(folder.name))
    if (!confirmed) return
    await workspace.deleteFolder(folder.folderKey)
  } catch (error) {
    toast.error(userFacingErrorCopy(error, t('library.errors.deleteFolderFailed')))
  }
}

async function requestRemove(asset: LibraryAssetView) {
  try {
    const confirmed = await confirmDialog.confirmAsync(removeAssetConfirmation())
    if (!confirmed) return
    await workspace.requestRemove(asset)
  } catch (error) {
    toast.error(userFacingErrorCopy(error, t('library.errors.removeAssetFailed')))
  }
}

function handleWindowResize() {
  closeFolderMenu()
  windowWidth.value = window.innerWidth
}

function handleAfterLeave() {
  if (!props.open && !isCanvasPresentation.value) isExpanded.value = false
}

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) {
      closeFolderMenu()
    }
    if (isOpen && !initialized.value) {
      initialized.value = true
      void workspace.initialize()
    }
  },
  { immediate: true },
)

watch(
  () => props.workspaceId,
  (next, previous) => {
    if (next !== previous && props.open && initialized.value) {
      void workspace.resetWorkspace()
    }
  },
)

let unsubscribeMutation = () => {}
onMounted(() => {
  unsubscribeMutation = props.mutationPublisher.subscribe((outcomes) => {
    if (
      !props.open ||
      !initialized.value ||
      !outcomes.some(
        (outcome) =>
          outcome.galleryEffect === 'reload-current' &&
          outcome.affectedFolderKeys.includes(workspace.selectedFolder.value),
      )
    ) {
      return
    }
    void workspace.loadAssets(true)
  })
  window.addEventListener('resize', handleWindowResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleWindowResize)
  unsubscribeMutation()
})
</script>

<style scoped>
.library-island {
  --chrome-presence-hidden-transform: translateX(calc(-100% - 96px));
}
</style>
