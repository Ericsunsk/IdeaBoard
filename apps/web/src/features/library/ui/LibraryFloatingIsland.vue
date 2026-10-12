<template>
  <Transition name="library-island">
    <aside
      v-if="presentation === 'canvas' || open"
      v-show="open"
      class="page-chrome-surface z-40 flex flex-col overflow-hidden rounded-[28px] p-5 text-foreground will-change-[width,transform] transition-[width,transform] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] max-w-[calc(100vw-32px)]"
      :class="isCanvasPresentation ? 'relative h-full w-full' : searchIslandClass"
      :aria-label="t('library.title')"
      @click="closeFolderMenu(false)"
      @dragover.prevent="handleIslandDragOver"
      @drop.prevent="handleIslandDrop"
    >
      <!-- 🌟 1. 常驻顶栏控制区与灵感夹微胶囊 (支持用户主动折叠/展开分类胶囊) -->
      <div class="flex flex-col shrink-0 mb-2">
        <!-- 顶栏：标题 + 折叠微按钮 + 展开/收缩按钮 + 微圆关闭按钮 -->
        <div class="flex items-center justify-between pb-2 shrink-0">
          <div class="flex items-center gap-1.5">
            <h2 class="m-0 text-base font-semibold tracking-tight">{{ t('library.title') }}</h2>
            <button
              type="button"
              class="grid size-6 shrink-0 place-items-center rounded-full bg-muted text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
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
              class="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
              :aria-label="isExpanded ? t('library.actions.collapse') : t('library.actions.expand')"
              :title="isExpanded ? t('library.actions.collapse') : t('library.actions.expand')"
              @click="isExpanded = !isExpanded"
            >
              <NavArrowLeft v-if="isExpanded" class="size-4" aria-hidden="true" />
              <NavArrowRight v-else class="size-4" aria-hidden="true" />
            </button>
            <button
              type="button"
              class="grid size-8 shrink-0 place-items-center rounded-full bg-muted text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
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
            <div class="h-7 w-full animate-pulse rounded-full bg-muted"></div>
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
                class="group/folder-pill inline-flex h-7 shrink-0 items-center rounded-full"
                @dragover.prevent.stop="handlePillDragOver($event, folder.folderKey)"
                @dragleave.stop="handlePillDragLeave"
                @drop.prevent.stop="handlePillDrop($event, folder)"
                @contextmenu.stop="openFolderMenu($event, folder)"
              >
                <button
                  type="button"
                  role="tab"
                  :aria-selected="workspace.selectedFolder.value === folder.folderKey"
                  :aria-haspopup="folder.isMutable ? 'menu' : undefined"
                  class="h-7 rounded-full border px-2.5 text-xs font-medium transition-[transform,background-color,border-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] flex items-center gap-1.5 whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
                  :class="[
                    workspace.selectedFolder.value === folder.folderKey
                      ? 'border-transparent bg-primary text-primary-foreground shadow-xs'
                      : 'border-transparent bg-muted text-muted-foreground hover:bg-muted hover:text-foreground',
                    hoveredPillKey === folder.folderKey
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
                  <Bookmark
                    v-else-if="folder.folderKey === 'quick-saves'"
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
                class="h-7 shrink-0 rounded-full border border-dashed border-border bg-transparent px-2.5 text-xs font-medium text-muted-foreground transition-colors hover:border-primary hover:text-primary flex items-center gap-1 whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
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
      <div
        v-if="workspace.errorMessage.value"
        class="mt-2 flex shrink-0 items-center justify-between gap-3 rounded-xl border border-destructive/40 bg-card px-3 py-2 text-xs"
        role="alert"
      >
        <span class="font-semibold">{{ workspace.errorMessage.value }}</span>
        <button
          type="button"
          class="shrink-0 rounded-full bg-primary px-3 py-1.5 font-semibold text-primary-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
          @click="workspace.retry"
        >
          {{ t('common.actions.retry') }}
        </button>
      </div>

      <!-- 🌟 2. 纯净 18px 物理圆角裁剪画廊视口 (双列/多列瀑布流) -->
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
        @asset-dragstart="handleAssetDragStart"
      />
    </aside>
  </Transition>

  <!-- 用户 Folder 的右键操作菜单 -->
  <LibraryFolderContextMenu
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
  <UserConfirmDialog
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Bookmark,
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
import type { LibraryAssetView } from '../core/transfer'
import type { CanvasLibraryImportDescriptor } from '../core/transfer'
import UserConfirmDialog from '../../../core/components/UserConfirmDialog.vue'
import { useUserConfirmDialog } from '../../../core/composables/useUserConfirmDialog'
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
import { useAppToast } from '../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'

const props = defineProps<{
  open: boolean
  workspaceId: string | null
  folderFacts: LibraryFolderFacts
  mutationPublisher: LibraryMutationPublisher
  referenceKeys?: string[]
  presentation: 'search' | 'canvas'
}>()

const emit = defineEmits<{
  close: []
  'reference-selected': [LibraryAssetView]
  'add-to-canvas': [CanvasLibraryImportDescriptor]
  'focus-scope-change': [focused: boolean]
}>()

const isExpanded = defineModel<boolean>('expanded', { default: false })
const { t } = useI18n()

const isCanvasPresentation = computed(() => props.presentation === 'canvas')
const searchIslandClass = computed(() =>
  isExpanded.value
    ? 'fixed top-[84px] bottom-4 left-4 sm:bottom-6 sm:left-6 w-[calc(100vw-32px)] sm:w-[calc(100vw-48px)]'
    : 'fixed top-[84px] bottom-4 left-4 sm:bottom-6 sm:left-6 w-[calc(100vw-32px)] sm:w-[350px]',
)

const confirmDialog = useUserConfirmDialog()
const toast = useAppToast()
const workspace = useLibraryWorkspace({
  workspaceId: () => props.workspaceId,
  syncRoute: false,
  autoLoad: false,
  folderFacts: props.folderFacts,
  mutationPublisher: props.mutationPublisher,
})

const {
  hoveredPillKey,
  handleAssetDragStart,
  handleIslandDragOver,
  handleIslandDrop,
  handlePillDragOver,
  handlePillDragLeave,
  handlePillDrop,
} = useLibraryIslandDnD({
  workspace,
  mutationPublisher: props.mutationPublisher,
})

const initialized = ref(false)
const isHeaderCollapsed = ref(false)
const windowWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1200)
const galleryRef = ref<InstanceType<typeof LibraryAssetGallery> | null>(null)
const contextMenu = ref<{
  folder: LibraryFolderTab
  x: number
  y: number
  trigger: HTMLElement | null
} | null>(null)

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

function handleDocumentPointerDown(event: PointerEvent) {
  if ((event.target as HTMLElement | null)?.closest('[data-library-folder-menu]')) return
  closeFolderMenu(false)
}

function handleDocumentKeydown(event: KeyboardEvent) {
  if (!contextMenu.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    closeFolderMenu(true)
  }
}

function handleWindowResize() {
  closeFolderMenu()
  windowWidth.value = window.innerWidth
}

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) {
      closeFolderMenu()
      if (!isCanvasPresentation.value) isExpanded.value = false
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
  unsubscribeMutation = props.mutationPublisher.subscribe((outcome) => {
    if (
      !props.open ||
      !initialized.value ||
      outcome.galleryEffect !== 'reload-current' ||
      !outcome.affectedFolderKeys.includes(workspace.selectedFolder.value)
    ) {
      return
    }
    void workspace.loadAssets(true)
  })
  document.addEventListener('pointerdown', handleDocumentPointerDown)
  document.addEventListener('keydown', handleDocumentKeydown)
  window.addEventListener('resize', handleWindowResize)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleDocumentPointerDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('resize', handleWindowResize)
  unsubscribeMutation()
})
</script>

<style scoped>
.library-island-enter-active {
  transition:
    opacity var(--motion-enter-duration) var(--motion-enter-easing),
    transform var(--motion-enter-duration) var(--motion-enter-easing);
  will-change: opacity, transform;
  backface-visibility: hidden;
  -webkit-backface-visibility: hidden;
  transform-style: preserve-3d;
}

.library-island-leave-active {
  transition:
    opacity var(--motion-exit-duration) var(--motion-exit-easing),
    transform var(--motion-exit-duration) var(--motion-exit-easing);
  will-change: opacity, transform;
  backface-visibility: hidden;
  -webkit-backface-visibility: hidden;
}

.library-island-enter-from,
.library-island-leave-to {
  opacity: 0;
  transform: translate3d(calc(-100% - 32px), 0, 0);
}
</style>
