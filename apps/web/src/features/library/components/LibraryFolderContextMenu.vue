<template>
  <div
    v-if="contextMenu"
    ref="menuRef"
    :class="[surfaceClass, 'fixed z-[var(--z-popover)] min-w-40 rounded-xl p-1.5 backdrop-blur-md']"
    :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }"
    data-library-folder-menu
    role="menu"
    tabindex="-1"
    :aria-label="contextMenu.folder.name"
    @click.stop
    @keydown="handleKeydown"
  >
    <button
      type="button"
      :class="[itemClass, '!h-8 !py-0 !text-xs cursor-pointer']"
      role="menuitem"
      @click="$emit('rename', contextMenu.folder)"
    >
      {{ t('library.actions.rename') }}
    </button>
    <button
      type="button"
      :class="[itemClass, dangerItemClass, '!h-8 !py-0 !text-xs cursor-pointer']"
      role="menuitem"
      @click="$emit('delete', contextMenu.folder)"
    >
      {{ t('library.actions.delete') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { LibraryFolderTab } from '../application/useLibraryWorkspace'
import { handleMenuKeyboardNavigation } from '../../../core/interaction/menuKeyboardNavigation'

defineProps<{
  contextMenu: {
    folder: LibraryFolderTab
    x: number
    y: number
    trigger: HTMLElement | null
  } | null
  surfaceClass: string
  itemClass: string
  dangerItemClass: string
}>()
const emit = defineEmits<{ rename: [LibraryFolderTab]; delete: [LibraryFolderTab]; escape: [] }>()
const { t } = useI18n()
const menuRef = ref<HTMLElement | null>(null)

watch(menuRef, (menu) => {
  if (menu) void nextTick(() => menu.querySelector<HTMLElement>('[role="menuitem"]')?.focus())
})

function handleKeydown(event: KeyboardEvent) {
  handleMenuKeyboardNavigation(event, menuRef.value, { onEscape: () => emit('escape') })
}
</script>
