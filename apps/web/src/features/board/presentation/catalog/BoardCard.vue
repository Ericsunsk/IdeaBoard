<template>
  <article
    class="board-card relative h-[244px] cursor-pointer overflow-hidden rounded-xl border border-board-card-border bg-board-card-bg text-left transition-colors hover:border-board-card-border-hover"
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
      <input
        v-if="isRenaming"
        ref="renameInputEl"
        :value="renameDraft"
        type="text"
        :aria-label="t('common.actions.rename')"
        class="pointer-events-auto h-[30px] w-full rounded-lg border border-board-card-input-border bg-board-card-input-bg px-2 text-[13px] text-board-card-action-hover outline-none"
        maxlength="60"
        @click.stop
        @input="onRenameInput"
        @blur="emit('commit-rename')"
        @keyup.enter.stop.prevent="emit('commit-rename')"
        @keyup.esc.stop.prevent="emit('cancel-rename')"
      />
      <button
        v-else
        type="button"
        class="pointer-events-auto block max-w-full cursor-pointer truncate border-0 bg-transparent p-0 text-left text-sm font-[650] text-board-card-title hover:underline focus-visible:outline-none focus-visible:underline"
        :aria-label="board.name"
        @click.stop="emit('click-board')"
      >
        {{ board.name }}
      </button>
      <div class="mt-[3px] text-xs text-board-card-text">
        {{ t('workspace.boards.edited', { date: displayDate }) }}
      </div>
    </div>

    <div class="absolute right-3 bottom-3 z-[3]" @click.stop @pointerdown.stop>
      <DropdownMenu v-model:open="isMenuOverlayOpen" @update:open="onMenuOpenChange">
        <DropdownMenuTrigger as-child>
          <Button
            variant="ghost"
            size="icon"
            :title="t('workspace.boards.moreActions')"
            :aria-label="t('workspace.boards.moreActions')"
            class="size-[34px] border-0 bg-transparent text-board-card-action hover:text-board-card-action-hover"
            @click="emit('toggle-menu')"
          >
            <MoreHoriz class="size-[18px]" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            v-for="item in menuItems"
            :key="item.label"
            :class="item.tone === 'danger' ? 'text-destructive focus:text-destructive' : ''"
            @select="item.command?.()"
          >
            {{ item.label }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  </article>
</template>

<script setup lang="ts">
import { MoreHoriz } from '@iconoir/vue'
import { computed, nextTick, ref, watch } from 'vue'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useI18n } from 'vue-i18n'
import type { CatalogEntry } from './contract'
import { blankBoardPreviewSurfaceClass } from './skin'

const props = defineProps<{
  board: CatalogEntry
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

const { t } = useI18n()

const menuItems = computed(() => [
  {
    key: 'rename',
    label: t('common.actions.rename'),
    command: () => emit('rename-action'),
  },
  {
    key: 'duplicate',
    label: t('workspace.boards.duplicate'),
    command: () => emit('duplicate-action'),
  },
  {
    key: 'delete',
    label: t('common.actions.delete'),
    tone: 'danger',
    command: () => emit('delete-action'),
  },
])

const renameInputEl = ref<HTMLInputElement | null>(null)

watch(
  () => props.isRenaming,
  (isRenaming) => {
    if (!isRenaming) return
    void nextTick(() => {
      renameInputEl.value?.focus()
      renameInputEl.value?.select()
    })
  },
)

function onRenameInput(event: Event) {
  emit('rename-input', (event.target as HTMLInputElement).value)
}

const isMenuOverlayOpen = ref(false)

watch(
  () => props.isMenuOpen,
  (isMenuOpen) => {
    if (!isMenuOpen && isMenuOverlayOpen.value) {
      isMenuOverlayOpen.value = false
    }
  },
)

function onMenuOpenChange(open: boolean) {
  if (!open) emit('menu-hidden')
}
</script>

<style scoped>
.board-card {
  transition:
    border-color var(--motion-control-duration) var(--motion-control-easing),
    transform var(--motion-control-duration) var(--motion-control-easing);
}

.board-card:hover,
.board-card:focus-visible {
  transform: translateY(-2px) scale(1.006);
}

.board-card-enter-active,
.board-card-appear-active {
  transition:
    opacity var(--motion-enter-duration) var(--motion-enter-easing),
    transform var(--motion-enter-duration) var(--motion-enter-easing);
}

.board-card-leave-active {
  transition:
    opacity var(--motion-exit-duration) var(--motion-exit-easing),
    transform var(--motion-exit-duration) var(--motion-exit-easing);
}

.board-card-enter-from,
.board-card-appear-from,
.board-card-leave-to {
  opacity: 0;
  transform: translateY(12px) scale(0.985);
}

.board-card-enter-to,
.board-card-appear-to,
.board-card-leave-from {
  opacity: 1;
  transform: translateY(0) scale(1);
}

@media (prefers-reduced-motion: reduce) {
  .board-card {
    transition: border-color var(--motion-control-duration) var(--motion-control-easing);
  }

  .board-card:hover,
  .board-card:focus-visible {
    transform: none;
  }

  .board-card-enter-active,
  .board-card-appear-active,
  .board-card-leave-active {
    transition: none;
  }

  .board-card-enter-from,
  .board-card-appear-from,
  .board-card-leave-to {
    opacity: 1;
    transform: none;
  }
}
</style>
