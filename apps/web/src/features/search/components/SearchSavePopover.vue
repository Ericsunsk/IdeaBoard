<template>
  <div class="relative shrink-0">
    <button
      ref="triggerButtonRef"
      type="button"
      class="grid size-[var(--search-overlay-btn-size)] place-items-center rounded-full transition shadow-md focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] disabled:opacity-60 cursor-pointer"
      :class="[
        saved
          ? 'bg-ib-success text-ib-text-inverse hover:bg-ib-success/90'
          : 'bg-board-primary text-ib-text-inverse hover:bg-board-primary-hover shadow-board-primary/30',
      ]"
      :aria-label="saved ? 'Remove from Library' : 'Save to Library'"
      :title="saved ? 'Remove from Library' : 'Save to Library Folder'"
      :disabled="saving"
      @click.stop="toggle"
    >
      <BookmarkSolid v-if="saved" class="size-3.5" />
      <Plus v-else class="size-3.5 stroke-[2.5]" />
    </button>

    <Popover
      ref="popover"
      :pt="searchSavePopoverPt"
      @show="handlePopoverShow"
      @hide="handlePopoverHide"
    >
      <div class="w-48 select-none text-board-page-text" @click.stop>
        <!-- 极简无边框搜索栏 (零下划线) -->
        <div class="flex items-center gap-2 px-2 py-1.5">
          <Search class="size-3.5 text-board-page-text-muted shrink-0" aria-hidden="true" />
          <input
            ref="searchInputRef"
            v-model="searchQuery"
            type="text"
            placeholder="Save to folder or create..."
            aria-label="Save to folder or create"
            class="w-full bg-transparent text-xs text-board-page-text placeholder:text-board-page-text-muted outline-none font-normal"
            @keydown.enter.prevent="handleEnterCreate"
          />
        </div>

        <!-- 动态新建候选行 (输入未匹配时出现) -->
        <div v-if="canCreateFromQuery" class="my-0.5 px-1">
          <button
            type="button"
            class="flex w-full items-center justify-between rounded-lg bg-board-primary/10 px-2 py-1.5 text-xs font-medium text-board-primary transition hover:bg-board-primary/20 cursor-pointer border-none"
            :disabled="saving"
            @click="handleEnterCreate"
          >
            <span class="truncate">+ Create "{{ searchQuery.trim() }}"</span>
            <span class="font-mono text-[10px] text-board-primary">↵</span>
          </button>
        </div>

        <!-- 极简灵感夹纯净列表 -->
        <div class="max-h-48 overflow-y-auto space-y-0.5 pt-1 app-scrollbar">
          <div
            v-if="!showQuickSaves && filteredFolders.length === 0 && !canCreateFromQuery"
            class="py-2.5 text-center text-xs text-board-page-text-muted"
          >
            No matching folders
          </div>

          <!-- 系统默认灵感夹：Quick Saves (真实数据) -->
          <button
            v-if="showQuickSaves"
            type="button"
            class="group/fitem flex w-full items-center justify-between rounded-lg bg-transparent px-2.5 py-1.5 text-left text-xs text-board-page-text hover:text-board-primary transition hover:bg-ib-surface-hover cursor-pointer border-none disabled:opacity-50"
            :disabled="saving"
            @click="saveTo(quickSavesFolder?.id)"
          >
            <div class="flex min-w-0 items-center gap-2">
              <Bookmark class="size-3.5 text-board-primary shrink-0" aria-hidden="true" />
              <span class="truncate font-medium">Quick Saves</span>
            </div>
            <span class="text-[11px] text-board-page-text-muted font-sans shrink-0">{{
              quickSavesCount
            }}</span>
          </button>

          <!-- 用户真实自定义灵感夹列表 -->
          <button
            v-for="folder in filteredFolders"
            :key="folder.id"
            type="button"
            class="group/fitem flex w-full items-center justify-between rounded-lg bg-transparent px-2.5 py-1.5 text-left text-xs text-board-page-text hover:text-board-primary transition hover:bg-ib-surface-hover cursor-pointer border-none disabled:opacity-50"
            :disabled="saving"
            @click="saveTo(folder.id)"
          >
            <div class="flex min-w-0 items-center gap-2">
              <Folder
                class="size-3.5 text-board-page-text-muted group-hover/fitem:text-board-primary shrink-0 transition-colors"
                aria-hidden="true"
              />
              <span class="truncate font-medium">{{ folder.name }}</span>
            </div>
            <span class="text-[11px] text-board-page-text-muted font-sans shrink-0">{{
              folder.asset_count
            }}</span>
          </button>
        </div>
      </div>
    </Popover>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import Popover from 'primevue/popover'
import { Bookmark, BookmarkSolid, Folder, Plus, Search } from '@iconoir/vue'
import type { SearchResult } from '../searchResource'
import {
  getLibraryFolders,
  removeSearchReference,
  saveSearchReference,
  type LibraryFolderView,
} from '../../library/libraryResource'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { createPopoverMotionPt } from '../../../shared/motion/popoverMotion'
import type { PopoverPassThroughOptions } from 'primevue/popover'

const props = defineProps<{ result: SearchResult; saved?: boolean; authenticated?: boolean }>()
const emit = defineEmits<{ saved: []; unsaved: []; 'auth-required': [] }>()

const toast = useAppToast()
const popover = ref<InstanceType<typeof Popover>>()
const triggerButtonRef = ref<HTMLButtonElement>()
const searchInputRef = ref<HTMLInputElement>()
const folders = ref<LibraryFolderView[]>([])
const saving = ref(false)
const searchQuery = ref('')
let foldersRefresh: Promise<void> | null = null

const searchSavePopoverPt = {
  ...createPopoverMotionPt(),
  root: {
    class:
      '!rounded-[var(--radius-ib-control)] !border !border-board-card-border/70 !bg-[var(--search-glass-bg)] !p-1.5 !shadow-[var(--search-dock-shadow)] backdrop-blur-[var(--search-glass-blur)]',
  },
  content: { class: '!p-0' },
} satisfies PopoverPassThroughOptions

const quickSavesFolder = computed(() => {
  return folders.value.find((f) => f.name.toLowerCase() === 'quick saves' || f.is_default)
})

const quickSavesCount = computed(() => {
  return quickSavesFolder.value ? quickSavesFolder.value.asset_count : 0
})

const displayFolders = computed(() => {
  return folders.value.filter(
    (f) =>
      !f.is_system &&
      !f.is_default &&
      f.name.toLowerCase() !== 'all assets' &&
      f.name.toLowerCase() !== 'quick saves',
  )
})

const showQuickSaves = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return true
  return 'quick saves'.includes(q)
})

const filteredFolders = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return displayFolders.value
  return displayFolders.value.filter((f) => f.name.toLowerCase().includes(q))
})

const canCreateFromQuery = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return false
  if (q === 'quick saves' || q === 'all assets') return false
  return !displayFolders.value.some((f) => f.name.toLowerCase() === q)
})

async function refreshFolders() {
  if (foldersRefresh) return foldersRefresh
  const request = (async () => {
    try {
      folders.value = await getLibraryFolders()
    } catch (error) {
      if (import.meta.env.DEV) console.debug('Failed to load library folders', error)
    }
  })()
  foldersRefresh = request
  await request
  foldersRefresh = null
}

function positionPopoverAbove() {
  const btn = triggerButtonRef.value
  const pop =
    (popover.value as unknown as { $el?: HTMLElement; container?: HTMLElement })?.$el ||
    (popover.value as unknown as { $el?: HTMLElement; container?: HTMLElement })?.container
  if (!btn || !pop) return
  const rect = btn.getBoundingClientRect()
  const popWidth = pop.offsetWidth || 192
  const popHeight = pop.offsetHeight || 220

  // 气泡右边缘精准对齐加号按钮右侧向左展开 (完整收在卡片内)
  let left = rect.right - popWidth
  if (left < 16) left = 16
  if (left + popWidth > window.innerWidth - 16) {
    left = window.innerWidth - popWidth - 16
  }

  // 气泡底边缘精准吸附在加号按钮正上方 (8px 黄金间距)
  let top = rect.top - popHeight - 8
  if (top < 70) {
    top = rect.bottom + 8
  }

  pop.style.setProperty('left', `${left}px`, 'important')
  pop.style.setProperty('top', `${top}px`, 'important')
}

async function handleUnsave() {
  if (saving.value) return
  saving.value = true
  try {
    await removeSearchReference(props.result.provider, props.result.id)
    emit('unsaved')
    toast.success('Removed from your Library')
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '移除素材失败')
  } finally {
    saving.value = false
  }
}

function toggle(event: Event) {
  if (props.authenticated === false) {
    emit('auth-required')
    return
  }
  if (props.saved) {
    void handleUnsave()
    return
  }
  const target =
    triggerButtonRef.value || (event.currentTarget as HTMLElement) || (event.target as HTMLElement)
  popover.value?.toggle(event, target)
  void refreshFolders()
  void nextTick(() => {
    positionPopoverAbove()
    requestAnimationFrame(positionPopoverAbove)
  })
}

function handlePopoverShow() {
  searchQuery.value = ''
  void nextTick(() => {
    positionPopoverAbove()
    searchInputRef.value?.focus()
  })
}

function handlePopoverHide() {
  searchQuery.value = ''
}

async function persist(folderId?: number, newFolderName?: string) {
  saving.value = true
  try {
    await saveSearchReference(props.result, folderId, newFolderName)
    emit('saved')
    popover.value?.hide()
    const targetName =
      newFolderName || folders.value.find((f) => f.id === folderId)?.name || 'Quick Saves'
    toast.success(`Saved to "${targetName}" in your Personal Library!`)
    void refreshFolders()
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '保存素材失败')
  } finally {
    saving.value = false
  }
}

function saveTo(folderId?: number) {
  void persist(folderId)
}

async function handleEnterCreate() {
  const name = searchQuery.value.trim()
  const normalizedName = name.toLowerCase()
  if (!normalizedName || saving.value) return
  if (normalizedName === 'all assets') return

  await refreshFolders()

  if (normalizedName === 'quick saves') {
    saveTo(quickSavesFolder.value?.id)
    return
  }

  const existingFolder = displayFolders.value.find(
    (folder) => folder.name.toLowerCase() === normalizedName,
  )
  if (existingFolder) {
    saveTo(existingFolder.id)
    return
  }

  if (canCreateFromQuery.value) void persist(undefined, name)
}
</script>
