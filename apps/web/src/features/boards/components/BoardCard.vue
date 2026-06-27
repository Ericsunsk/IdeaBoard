<template>
  <article
    v-motion="{
      initial: { opacity: 0, y: 12, scale: 0.985 },
      enter: {
        opacity: 1,
        y: 0,
        scale: 1,
        transition: { duration: 220, easing: 'ease-out' },
      },
      hovered: {
        y: -2,
        scale: 1.006,
        transition: { duration: 160, easing: 'ease-out' },
      },
    }"
    class="relative h-[244px] cursor-pointer overflow-hidden rounded-xl border border-board-card-border bg-white/90 text-left transition-colors hover:border-board-card-border-hover"
    role="button"
    tabindex="0"
    @keydown.enter.self.prevent="emit('click-board')"
    @keydown.space.self.prevent="emit('click-board')"
    @click="emit('click-board')"
  >
    <div
      class="relative z-0 flex h-full w-full items-center justify-center overflow-hidden bg-[linear-gradient(135deg,var(--color-board-card-preview-from)_0%,var(--color-board-card-preview-to)_100%)]"
    >
      <img
        v-if="previewSnapshotDataUrl"
        :src="previewSnapshotDataUrl"
        class="block h-full w-full object-cover"
        alt=""
      />
      <div
        v-else
        class="h-full w-full"
        :class="blankBoardPreviewSurfaceClass"
        aria-hidden="true"
      ></div>
      <div
        class="pointer-events-none absolute inset-x-0 bottom-0 h-[92px] bg-[linear-gradient(to_top,var(--color-board-card-preview-fade)_0%,var(--color-board-card-preview-fade-clear)_100%)]"
      ></div>
    </div>

    <div class="pointer-events-none absolute right-14 bottom-3 left-3 z-[2]">
      <InputText
        v-if="isRenaming"
        ref="renameInputEl"
        :value="renameDraft"
        unstyled
        class="pointer-events-auto h-[30px] w-full rounded-lg border border-board-card-input-border bg-white/80 px-2 text-[13px] text-board-card-action-hover outline-none"
        maxlength="60"
        @click.stop
        @input="onRenameInput"
        @blur="emit('commit-rename')"
        @keyup.enter.stop.prevent="emit('commit-rename')"
        @keyup.esc.stop.prevent="emit('cancel-rename')"
      />
      <div v-else class="truncate text-sm font-[650] text-board-card-title">{{ board.name }}</div>
      <div class="mt-[3px] text-xs text-board-card-text">Edited {{ displayDate }}</div>
    </div>

    <div class="absolute right-3 bottom-3 z-[3]" @click.stop @pointerdown.stop>
      <Button
        unstyled
        type="button"
        title="More actions"
        aria-label="More actions"
        aria-haspopup="menu"
        :aria-expanded="isMenuOpen"
        class="inline-flex size-[34px] cursor-pointer items-center justify-center border-0 bg-transparent text-board-card-action hover:text-board-card-action-hover"
        @click="toggleMenu"
      >
        <MoreHoriz class="size-[18px]" />
      </Button>
      <Menu
        ref="menuRef"
        :model="menuItems"
        popup
        append-to="body"
        :pt="menuPt"
        @hide="handleMenuHidden"
      >
        <template #item="{ item, props: itemProps }">
          <a
            v-bind="itemProps.action"
            :class="[popupMenuItemClass, item.key === 'delete' ? popupMenuDangerItemClass : '']"
          >
            {{ item.label }}
          </a>
        </template>
      </Menu>
    </div>
  </article>
</template>

<script setup lang="ts">
import { MoreHoriz } from '@iconoir/vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'
import { blankBoardPreviewSurfaceClass } from '../../canvas-board/preview'
import type { CanvasBoardMeta } from '../lib/canvasBoards'
import {
  boardMenuPt,
  popupMenuDangerItemClass,
  popupMenuItemClass,
  type PrimePopupMenuRef,
} from '../lib/menuStyles'

type RenameInputRef = HTMLInputElement | { $el?: HTMLInputElement } | null

const props = defineProps<{
  board: CanvasBoardMeta
  previewSnapshotDataUrl: string
  isRenaming: boolean
  renameDraft: string
  isMenuOpen: boolean
  displayDate: string
}>()

const emit = defineEmits<{
  (e: 'click-board'): void
  (e: 'toggle-menu'): void
  (e: 'rename-action'): void
  (e: 'duplicate-action'): void
  (e: 'delete-action'): void
  (e: 'rename-input', value: string): void
  (e: 'commit-rename'): void
  (e: 'cancel-rename'): void
  (e: 'menu-hidden'): void
}>()

const renameInputEl = ref<RenameInputRef>(null)
const menuRef = ref<PrimePopupMenuRef | null>(null)
const menuPt = boardMenuPt

const menuItems = computed<MenuItem[]>(() => [
  {
    key: 'rename',
    label: 'Rename',
    command: () => emit('rename-action'),
  },
  {
    key: 'duplicate',
    label: 'Duplicate',
    command: () => emit('duplicate-action'),
  },
  {
    key: 'delete',
    label: 'Delete',
    command: () => emit('delete-action'),
  },
])

watch(
  () => props.isRenaming,
  (isRenaming) => {
    if (!isRenaming) return
    void nextTick(() => {
      const input =
        renameInputEl.value instanceof HTMLInputElement
          ? renameInputEl.value
          : renameInputEl.value?.$el
      input?.focus()
      input?.select()
    })
  },
)

function onRenameInput(event: Event) {
  emit('rename-input', (event.target as HTMLInputElement).value)
}

function toggleMenu(event: Event) {
  emit('toggle-menu')
  menuRef.value?.toggle(event)
}

function handleMenuHidden() {
  emit('menu-hidden')
}

watch(
  () => props.isMenuOpen,
  (isMenuOpen) => {
    if (!isMenuOpen) {
      menuRef.value?.hide()
    }
  },
)
</script>
