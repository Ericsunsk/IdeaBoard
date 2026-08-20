<template>
  <Transition name="library-island">
    <aside
      v-if="open"
      class="fixed top-[84px] bottom-4 sm:bottom-6 left-4 sm:left-6 z-40 flex flex-col overflow-hidden rounded-[28px] border border-board-card-border bg-board-surface-strong/80 p-5 text-board-page-text backdrop-blur-xl shadow-[0_16px_48px_rgba(45,64,89,0.08),0_2px_8px_rgba(0,0,0,0.02)] will-change-[width,transform] transition-[width,transform] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] max-w-[calc(100vw-32px)]"
      :class="
        isExpanded
          ? 'w-[calc(100vw-32px)] sm:w-[calc(100vw-48px)]'
          : 'w-[calc(100vw-32px)] sm:w-[350px]'
      "
      aria-label="Personal Library"
      @click="closeFolderMenu"
      @dragover.prevent="handleIslandDragOver"
      @dragleave="handleIslandDragLeave"
      @drop.prevent="handleIslandDrop"
    >
      <!-- 🌟 1. 120fps CSS Grid (0fr ⟷ 1fr) 硬件加速智能折叠顶栏控制区 -->
      <div
        class="smooth-grid-collapse shrink-0"
        :style="{
          gridTemplateRows: isHeaderCollapsed ? '0fr' : '1fr',
          marginBottom: isHeaderCollapsed ? '0px' : '0.5rem',
        }"
      >
        <div
          class="smooth-grid-content flex flex-col shrink-0"
          :style="{
            opacity: isHeaderCollapsed ? 0 : 1,
            transform: isHeaderCollapsed ? 'translateY(-12px)' : 'translateY(0)',
            pointerEvents: isHeaderCollapsed ? 'none' : 'auto',
          }"
        >
          <!-- 顶栏：标题 + 展开/收缩按钮 + 微圆关闭按钮 -->
          <div class="flex items-center justify-between pb-2 shrink-0">
            <h2 class="m-0 text-base font-semibold tracking-tight">My Library</h2>
            <div class="flex items-center gap-1.5">
              <button
                type="button"
                class="grid size-8 shrink-0 place-items-center rounded-full bg-black/[0.04] text-board-page-text-muted transition-colors hover:bg-black/[0.08] hover:text-board-page-text focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] cursor-pointer"
                :aria-label="isExpanded ? 'Collapse library' : 'Expand library'"
                :title="isExpanded ? 'Collapse library' : 'Expand library'"
                @click="isExpanded = !isExpanded"
              >
                <NavArrowLeft v-if="isExpanded" class="size-4" aria-hidden="true" />
                <NavArrowRight v-else class="size-4" aria-hidden="true" />
              </button>
              <button
                type="button"
                class="grid size-8 shrink-0 place-items-center rounded-full bg-black/[0.04] text-board-page-text-muted transition-colors hover:bg-black/[0.08] hover:text-board-page-text focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] cursor-pointer"
                aria-label="Close Library"
                title="Close Library"
                @click="emit('close')"
              >
                <Xmark class="size-4" aria-hidden="true" />
              </button>
            </div>
          </div>

          <!-- 灵感夹筛选微胶囊 (自然流式换行，支持拖拽落盘，零滚动条) -->
          <div v-if="workspace.foldersLoading.value" class="py-1">
            <div class="h-7 w-full animate-pulse rounded-full bg-board-card-border/50"></div>
          </div>
          <div v-else class="py-1 shrink-0">
            <div
              class="flex flex-wrap items-center gap-1.5"
              role="tablist"
              aria-label="Library folders"
            >
              <div
                v-for="folder in workspace.folderTabs.value"
                :key="folder.queryKey"
                class="group/folder-pill inline-flex h-7 shrink-0 items-center rounded-full"
                @dragover.prevent.stop="handlePillDragOver(folder.queryKey)"
                @dragleave.stop="handlePillDragLeave"
                @drop.prevent.stop="handlePillDrop($event, folder)"
                @contextmenu.stop="openFolderMenu($event, folder)"
              >
                <button
                  type="button"
                  role="tab"
                  :aria-selected="workspace.selectedFolder.value === folder.queryKey"
                  class="h-7 rounded-full border px-2.5 text-xs font-medium transition-[transform,background-color,border-color,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] flex items-center gap-1.5 whitespace-nowrap focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
                  :class="[
                    workspace.selectedFolder.value === folder.queryKey
                      ? 'border-transparent bg-board-primary text-ib-text-inverse shadow-xs'
                      : 'border-transparent bg-black/[0.04] text-board-page-text-muted hover:bg-black/[0.08] hover:text-board-page-text',
                    hoveredPillKey === folder.queryKey
                      ? '!scale-105 !border-board-primary !bg-board-primary/10'
                      : '',
                  ]"
                  @click="selectFolder(folder.queryKey)"
                >
                  <ReportColumns
                    v-if="folder.queryKey === 'all'"
                    class="size-3 opacity-80"
                    aria-hidden="true"
                  />
                  <Bookmark
                    v-else-if="folder.queryKey === 'quick-saves'"
                    class="size-3 opacity-80"
                    aria-hidden="true"
                  />
                  <Folder v-else class="size-3 opacity-80" aria-hidden="true" />
                  <span>{{ folder.name }}</span>
                  <span class="text-[10px] opacity-70 font-mono">{{ folder.assetCount }}</span>
                </button>
              </div>

              <!-- + New Folder 极简微胶囊 -->
              <button
                type="button"
                class="h-7 shrink-0 rounded-full border border-dashed border-board-card-border bg-transparent px-2.5 text-xs font-medium text-board-page-text-muted transition-colors hover:border-board-primary hover:text-board-primary flex items-center gap-1 whitespace-nowrap focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
                @click="folderDialogVisible = true"
              >
                <Plus class="size-3" aria-hidden="true" />
                <span>New</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 错误状态提示 -->
      <div
        v-if="workspace.errorMessage.value"
        class="mt-2 flex shrink-0 items-center justify-between gap-3 rounded-xl border border-board-danger/40 bg-board-card-bg px-3 py-2 text-xs"
        role="alert"
      >
        <span class="font-semibold">{{ workspace.errorMessage.value }}</span>
        <button
          type="button"
          class="shrink-0 rounded-full bg-board-primary px-3 py-1.5 font-semibold text-ib-text-inverse focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
          @click="workspace.retry"
        >
          Retry
        </button>
      </div>

      <!-- 🌟 2. 纯净 18px 物理圆角裁剪画廊视口 (双列密排，带 onscroll 智能折叠阻尼) -->
      <div
        ref="scrollAreaRef"
        class="app-scrollbar mt-2 h-full min-h-0 flex-1 overflow-y-auto pr-0.5 pt-1 rounded-[18px]"
        :aria-busy="workspace.loading.value"
        @scroll="handleGalleryScroll"
      >
        <div
          v-if="workspace.loading.value && workspace.assets.value.length === 0"
          class="grid h-full min-h-40 grid-cols-2 gap-2 overflow-hidden"
          aria-busy="true"
        >
          <div
            v-for="n in 6"
            :key="n"
            class="h-28 animate-pulse rounded-xl bg-board-card-border/50"
          ></div>
        </div>
        <div
          v-else-if="
            !workspace.loading.value &&
            workspace.assets.value.length === 0 &&
            !workspace.errorMessage.value
          "
          class="grid h-full min-h-40 place-items-center rounded-[18px] border border-dashed border-board-card-border px-5 text-center"
        >
          <div>
            <p class="m-0 text-sm font-semibold">No saved references</p>
            <p class="mt-1 mb-0 text-xs text-board-page-text-muted">
              Drag or save images from Search to build your library.
            </p>
          </div>
        </div>
        <div
          v-else
          class="pb-2"
          :class="
            isExpanded
              ? 'grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7 gap-3'
              : 'grid grid-cols-2 gap-2'
          "
        >
          <article
            v-for="asset in workspace.assets.value"
            :key="asset.id"
            class="group/libcard relative overflow-hidden rounded-xl bg-board-card-bg/80 backdrop-blur-md shadow-none transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
          >
            <button
              type="button"
              class="block w-full cursor-zoom-in text-left focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
              :aria-label="`View ${asset.title || 'saved reference'}`"
              @click="openAsset(asset)"
            >
              <div
                class="relative overflow-hidden bg-board-loading-bg"
                :style="{ aspectRatio: `${asset.width || 1} / ${asset.height || 1}` }"
              >
                <div
                  v-if="failedImages.has(asset.id)"
                  class="grid min-h-24 place-items-center px-2 text-center text-xs text-board-page-text-muted"
                >
                  Image unavailable
                </div>
                <img
                  v-else
                  :src="asset.preview_url"
                  :alt="asset.title || 'Saved reference'"
                  loading="lazy"
                  class="h-full w-full object-cover"
                  @error="markImageError(asset.id)"
                />
              </div>
            </button>

            <!-- 悬浮微渐变覆盖层 -->
            <div
              class="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-0 group-hover/libcard:opacity-100 transition-opacity duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] flex flex-col justify-between p-2 pointer-events-none"
            >
              <div class="flex justify-end pointer-events-auto">
                <button
                  type="button"
                  class="grid size-6 shrink-0 place-items-center rounded-full bg-black/40 text-white backdrop-blur-md hover:bg-board-danger transition-colors focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
                  aria-label="Remove saved reference"
                  title="Remove saved reference"
                  @click="requestRemove(asset)"
                >
                  <Trash class="size-3" aria-hidden="true" />
                </button>
              </div>
              <div class="min-w-0 pointer-events-auto">
                <p class="m-0 truncate text-[11px] font-medium text-white leading-tight">
                  {{ asset.title || 'Untitled reference' }}
                </p>
                <p class="mt-0.5 mb-0 truncate text-[10px] text-white/70">
                  {{ asset.author || asset.provider }}
                </p>
              </div>
            </div>
          </article>

          <button
            v-if="workspace.hasMore.value"
            type="button"
            class="mx-auto my-3 block rounded-full border border-board-card-border bg-board-control-bg/80 backdrop-blur-md px-3 py-1.5 text-xs font-semibold focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] disabled:opacity-50 cursor-pointer"
            :disabled="workspace.loading.value"
            @click="workspace.loadAssets(false)"
          >
            Load more
          </button>
        </div>
      </div>
    </aside>
  </Transition>

  <!-- 用户 Folder 的右键操作菜单 -->
  <div
    v-if="contextMenu"
    ref="folderMenuRef"
    class="fixed z-[80] min-w-40 rounded-xl border border-board-card-border bg-board-surface-strong/80 p-1.5 text-board-page-text shadow-[0_12px_32px_rgba(45,64,89,0.16)] backdrop-blur-md"
    :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }"
    role="menu"
    @click.stop
  >
    <button
      type="button"
      class="flex h-8 w-full items-center rounded-lg px-2.5 text-left text-xs font-semibold text-board-page-text transition-colors hover:bg-ib-surface-hover cursor-pointer"
      role="menuitem"
      @click="openRenameFolder(contextMenu.folder)"
    >
      <span>Rename</span>
    </button>
    <button
      type="button"
      class="flex h-8 w-full items-center rounded-lg px-2.5 text-left text-xs font-semibold text-board-danger transition-colors hover:bg-board-danger/10 cursor-pointer"
      role="menuitem"
      @click="deleteFolder(contextMenu.folder)"
    >
      <span>Delete</span>
    </button>
  </div>

  <!-- 创建新文件夹弹窗 -->
  <Dialog
    v-model:visible="folderDialogVisible"
    modal
    :show-header="false"
    :style="{ width: 'min(420px, calc(100vw - 32px))' }"
    :pt="dialogPt"
  >
    <form class="p-6" @submit.prevent="submitFolder">
      <h3 class="m-0 text-lg font-semibold">New folder</h3>
      <label class="mt-5 block text-sm font-semibold" for="island-folder-name">Folder name</label>
      <InputText
        id="island-folder-name"
        v-model="folderDraft"
        unstyled
        class="mt-2 w-full rounded-xl border border-board-card-border bg-board-control-bg px-3 py-2.5 text-sm outline-none focus:border-board-primary"
        maxlength="100"
        autofocus
        placeholder="e.g. Minimalist Spatial"
      />
      <p v-if="folderError" class="mt-2 mb-0 text-sm text-board-danger">{{ folderError }}</p>
      <div class="mt-6 flex justify-end gap-2">
        <Button
          unstyled
          type="button"
          label="Cancel"
          class="rounded-full px-4 py-2 text-sm font-semibold text-board-page-text-muted hover:bg-ib-surface-hover"
          @click="folderDialogVisible = false"
        />
        <Button
          unstyled
          type="submit"
          label="Create"
          :disabled="folderSaving || !folderDraft.trim()"
          class="rounded-full bg-board-primary px-4 py-2 text-sm font-semibold text-ib-text-inverse disabled:opacity-50"
        />
      </div>
    </form>
  </Dialog>

  <!-- 重命名文件夹弹窗 -->
  <Dialog
    v-model:visible="renameDialogVisible"
    modal
    :show-header="false"
    :style="{ width: 'min(420px, calc(100vw - 32px))' }"
    :pt="dialogPt"
  >
    <form class="p-6" @submit.prevent="submitRenameFolder">
      <h3 class="m-0 text-lg font-semibold">Rename folder</h3>
      <label class="mt-5 block text-sm font-semibold" for="island-rename-folder-name"
        >Folder name</label
      >
      <InputText
        id="island-rename-folder-name"
        v-model="renameDraft"
        unstyled
        class="mt-2 w-full rounded-xl border border-board-card-border bg-board-control-bg px-3 py-2.5 text-sm outline-none focus:border-board-primary"
        maxlength="100"
        autofocus
      />
      <p v-if="renameError" class="mt-2 mb-0 text-sm text-board-danger">{{ renameError }}</p>
      <div class="mt-6 flex justify-end gap-2">
        <Button
          unstyled
          type="button"
          label="Cancel"
          class="rounded-full px-4 py-2 text-sm font-semibold text-board-page-text-muted hover:bg-ib-surface-hover"
          @click="renameDialogVisible = false"
        />
        <Button
          unstyled
          type="submit"
          label="Save"
          :disabled="renameSaving || !renameDraft.trim()"
          class="rounded-full bg-board-primary px-4 py-2 text-sm font-semibold text-ib-text-inverse disabled:opacity-50"
        />
      </div>
    </form>
  </Dialog>

  <!-- 🌟 Demo 同款零遮挡全景高清 Lightbox 弹窗 (bg-black/65 backdrop-blur-2xl) -->
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
      leave-active-class="transition-opacity duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div
        v-if="!!workspace.activeAsset.value"
        class="fixed inset-0 z-[2000] flex flex-col items-center justify-between p-4 sm:p-6 md:p-8 bg-black/65 backdrop-blur-2xl select-none"
        @click="workspace.setActiveAsset(null)"
      >
        <!-- 顶部极简悬浮信息与操作栏 -->
        <div
          class="flex w-full max-w-6xl items-center justify-between z-20 shrink-0 mb-3 px-2"
          @click.stop
        >
          <!-- 左上角：纯净悬浮文字 (Title + Author) -->
          <div class="min-w-0 pr-4">
            <h3
              class="m-0 text-white text-base sm:text-lg font-semibold tracking-tight drop-shadow-md truncate max-w-[240px] sm:max-w-md"
            >
              {{ workspace.activeAsset.value.title || 'Saved reference' }}
            </h3>
            <p class="mt-1 mb-0 text-white/70 text-xs drop-shadow-md">
              {{ workspace.activeAsset.value.author }} · {{ workspace.activeAsset.value.provider }}
            </p>
          </div>

          <!-- 右上角：来源链接 + 极简圆形 ✕ 关闭按钮 -->
          <div class="flex items-center gap-2.5">
            <a
              v-if="workspace.activeAsset.value.source_url"
              :href="workspace.activeAsset.value.source_url"
              target="_blank"
              rel="noreferrer"
              class="h-9 px-4 rounded-full bg-white/10 hover:bg-white/20 text-white text-xs font-semibold backdrop-blur-md border border-white/15 flex items-center gap-1.5 transition no-underline shadow-sm"
            >
              <span>View Source</span>
            </a>

            <button
              type="button"
              class="size-9 rounded-full bg-black/40 hover:bg-white/20 text-white/80 hover:text-white backdrop-blur-xl border border-white/10 flex items-center justify-center transition shadow-md shrink-0 cursor-pointer focus-visible:outline-none"
              aria-label="Close preview"
              title="Close (Esc)"
              @click="workspace.setActiveAsset(null)"
            >
              <Xmark class="size-4" aria-hidden="true" />
            </button>
          </div>
        </div>

        <!-- 中间大图展示区 (彻底去除边框与遮挡，纯粹沉浸大图鉴赏) -->
        <div
          class="flex-1 w-full max-w-6xl flex items-center justify-center overflow-hidden min-h-0"
          @click.stop
        >
          <img
            :src="
              workspace.activeAsset.value.original_url || workspace.activeAsset.value.preview_url
            "
            :alt="workspace.activeAsset.value.title || 'Saved reference'"
            class="max-h-[82vh] max-w-full object-contain rounded-2xl shadow-2xl border-0 ring-0 outline-none select-none"
          />
        </div>
      </div>
    </Transition>
  </Teleport>

  <UserConfirmDialog
    :visible="confirmDialog.visible.value"
    :title="confirmDialog.title.value"
    :message="confirmDialog.message.value"
    :confirm-label="confirmDialog.confirmLabel.value"
    :intent="confirmDialog.intent.value"
    :loading="confirmDialog.loading.value"
    @update:visible="confirmDialog.close"
    @confirm="confirmDialog.confirm"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import {
  Bookmark,
  Folder,
  NavArrowLeft,
  NavArrowRight,
  Plus,
  ReportColumns,
  Trash,
  Xmark,
} from '@iconoir/vue'
import { useLibraryWorkspace, type LibraryFolderTab } from '../useLibraryWorkspace'
import {
  libraryAssetKey,
  saveSearchReference,
  type LibraryAssetView,
  type LibrarySearchReference,
} from '../libraryResource'
import { useAppToast } from '../../../shared/composables/useAppToast'
import UserConfirmDialog from '../../../shared/components/UserConfirmDialog.vue'
import { useUserConfirmDialog } from '../../../shared/composables/useUserConfirmDialog'
import { deleteFolderConfirmation, removeAssetConfirmation } from '../libraryConfirmations'

const props = defineProps<{ open: boolean; refreshKey: number }>()
const emit = defineEmits<{ close: []; 'asset-saved': [key: string] }>()
const isExpanded = defineModel<boolean>('expanded', { default: false })

const toast = useAppToast()
const confirmDialog = useUserConfirmDialog()
const workspace = useLibraryWorkspace({ syncRoute: false, autoLoad: false })
const initialized = ref(false)
const failedImages = ref(new Set<number>())
const isHeaderCollapsed = ref(false)
const scrollAreaRef = ref<HTMLElement | null>(null)
const hoveredPillKey = ref<string | null>(null)
const folderMenuRef = ref<HTMLElement | null>(null)
const contextMenu = ref<{ folder: LibraryFolderTab; x: number; y: number } | null>(null)

// 新建文件夹表单状态
const folderDialogVisible = ref(false)
const folderDraft = ref('')
const folderError = ref('')
const folderSaving = ref(false)
const renameDialogVisible = ref(false)
const renameFolderKey = ref('')
const renameDraft = ref('')
const renameError = ref('')
const renameSaving = ref(false)

const dialogPt = {
  root: {
    class: '!rounded-2xl !border-board-card-border !bg-board-surface-strong/80 backdrop-blur-md',
  },
  content: { class: '!p-0' },
}

// 🌟 120fps CSS Grid 折叠控制与阻尼防抖
let lastScrollTop = 0
let scrollDeltaTracker = 0

function handleGalleryScroll(event: Event) {
  closeFolderMenu()
  const el = event.target as HTMLElement
  const currentScroll = el.scrollTop
  const delta = currentScroll - lastScrollTop

  if (currentScroll <= 8) {
    if (isHeaderCollapsed.value) isHeaderCollapsed.value = false
    scrollDeltaTracker = 0
  } else if (delta > 0) {
    scrollDeltaTracker = Math.max(0, scrollDeltaTracker + delta)
    if (scrollDeltaTracker > 20 && !isHeaderCollapsed.value) {
      isHeaderCollapsed.value = true
    }
  } else if (delta < 0) {
    scrollDeltaTracker = Math.min(0, scrollDeltaTracker + delta)
    if (scrollDeltaTracker < -12 && isHeaderCollapsed.value) {
      isHeaderCollapsed.value = false
    }
  }

  lastScrollTop = Math.max(0, currentScroll)
}

function markImageError(assetID: number) {
  failedImages.value = new Set(failedImages.value).add(assetID)
}

function openAsset(asset: LibraryAssetView) {
  workspace.setActiveAsset(asset)
}

function selectFolder(folderKey: string) {
  closeFolderMenu()
  scrollAreaRef.value?.scrollTo({ top: 0 })
  isHeaderCollapsed.value = false
  lastScrollTop = 0
  scrollDeltaTracker = 0
  workspace.selectFolder(folderKey)
}

function closeFolderMenu() {
  contextMenu.value = null
}

function openFolderMenu(event: MouseEvent, folder: LibraryFolderTab) {
  if (!folder.isMutable) return
  event.preventDefault()
  const menuWidth = 160
  const menuHeight = 84
  const edge = 8
  const maxX = Math.max(edge, window.innerWidth - menuWidth - edge)
  const maxY = Math.max(edge, window.innerHeight - menuHeight - edge)
  contextMenu.value = {
    folder,
    x: Math.min(Math.max(event.clientX, edge), maxX),
    y: Math.min(Math.max(event.clientY, edge), maxY),
  }
}

function openRenameFolder(folder: LibraryFolderTab) {
  closeFolderMenu()
  renameFolderKey.value = folder.queryKey
  renameDraft.value = folder.name
  renameError.value = ''
  renameDialogVisible.value = true
}

async function submitRenameFolder() {
  const name = renameDraft.value.trim()
  if (!name || !renameFolderKey.value) return
  renameSaving.value = true
  renameError.value = ''
  try {
    await workspace.renameFolder(renameFolderKey.value, name)
    renameDialogVisible.value = false
  } catch (error) {
    renameError.value = error instanceof Error ? error.message : '重命名收藏夹失败'
  } finally {
    renameSaving.value = false
  }
}

function deleteFolder(folder: LibraryFolderTab) {
  closeFolderMenu()
  if (!folder.isMutable) return
  confirmDialog.request(deleteFolderConfirmation(folder.name), async () => {
    await workspace.deleteFolder(folder.queryKey)
  })
}

function requestRemove(asset: LibraryAssetView) {
  confirmDialog.request(removeAssetConfirmation(), async () => {
    await workspace.requestRemove(asset)
  })
}

function handleDocumentPointerDown(event: PointerEvent) {
  if (folderMenuRef.value?.contains(event.target as Node)) return
  closeFolderMenu()
}

function handleWindowResize() {
  closeFolderMenu()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && workspace.activeAsset.value) {
    workspace.setActiveAsset(null)
  }
}

// 拖拽落盘事件处理
function parseDraggedReference(e: DragEvent): LibrarySearchReference | null {
  try {
    const raw = e.dataTransfer?.getData('application/json')
    if (!raw) return null
    return JSON.parse(raw) as LibrarySearchReference
  } catch {
    return null
  }
}

function handleIslandDragOver(e: DragEvent) {
  e.dataTransfer!.dropEffect = 'copy'
}

function handleIslandDragLeave() {
  // 静默保持纯净，无外描边干扰
}

async function handleIslandDrop(e: DragEvent) {
  const reference = parseDraggedReference(e)
  if (!reference) return

  // 存入当前选中的文件夹
  const currentKey = workspace.selectedFolder.value
  const currentFolder = workspace.folders.value.find((f) => f.query_key === currentKey)
  const targetFolderId =
    currentFolder && !currentFolder.is_virtual && !currentFolder.is_system
      ? currentFolder.id
      : undefined

  try {
    await saveSearchReference(reference, targetFolderId)
    const key = libraryAssetKey(reference.provider, reference.id)
    emit('asset-saved', key)
    await Promise.all([workspace.loadFolders(), workspace.loadAssets(true)])
    toast.success(`Saved "${reference.title || 'Image'}" to Library`)
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '保存素材失败')
  }
}

function handlePillDragOver(key: string) {
  hoveredPillKey.value = key
}

function handlePillDragLeave() {
  hoveredPillKey.value = null
}

async function handlePillDrop(e: DragEvent, folder: LibraryFolderTab) {
  hoveredPillKey.value = null
  const reference = parseDraggedReference(e)
  if (!reference) return

  const targetFolder = workspace.folders.value.find((f) => f.query_key === folder.queryKey)
  const targetFolderId =
    targetFolder && !targetFolder.is_virtual && !targetFolder.is_system
      ? targetFolder.id
      : undefined

  try {
    await saveSearchReference(reference, targetFolderId)
    const key = libraryAssetKey(reference.provider, reference.id)
    emit('asset-saved', key)
    workspace.selectFolder(folder.queryKey)
    await workspace.loadFolders()
    toast.success(`Saved "${reference.title || 'Image'}" to ${folder.name}`)
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '保存素材失败')
  }
}

async function submitFolder() {
  const name = folderDraft.value.trim()
  if (!name) return
  folderSaving.value = true
  folderError.value = ''
  try {
    await workspace.createNewFolder(name)
    folderDialogVisible.value = false
    folderDraft.value = ''
  } catch (error) {
    folderError.value = error instanceof Error ? error.message : '创建收藏夹失败'
  } finally {
    folderSaving.value = false
  }
}

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) {
      closeFolderMenu()
      isExpanded.value = false
    }
    if (isOpen && !initialized.value) {
      initialized.value = true
      void workspace.initialize()
    }
  },
  { immediate: true },
)

watch(
  () => props.refreshKey,
  () => {
    if (props.open && initialized.value) {
      void Promise.all([workspace.loadFolders(), workspace.loadAssets(true)])
    }
  },
)

onMounted(() => {
  document.addEventListener('pointerdown', handleDocumentPointerDown)
  window.addEventListener('keydown', handleKeydown)
  window.addEventListener('resize', handleWindowResize)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleDocumentPointerDown)
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', handleWindowResize)
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

/* 🌟 120fps 硬件加速折叠动效 (CSS Grid 0fr <-> 1fr) */
.smooth-grid-collapse {
  display: grid;
  transition:
    grid-template-rows var(--motion-control-duration) var(--motion-control-easing),
    margin-bottom var(--motion-control-duration) var(--motion-control-easing);
  will-change: grid-template-rows, margin-bottom;
}

.smooth-grid-content {
  overflow: hidden;
  min-height: 0;
  transition:
    opacity var(--motion-control-duration) var(--motion-control-easing),
    transform var(--motion-control-duration) var(--motion-control-easing);
  will-change: opacity, transform;
}
</style>
