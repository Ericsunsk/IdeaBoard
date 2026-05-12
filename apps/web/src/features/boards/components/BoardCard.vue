<template>
  <article
    class="relative h-[244px] cursor-pointer overflow-hidden rounded-[12px] border border-[#d9dfec] bg-[rgba(255,255,255,0.92)] text-left transition-[transform,border-color] duration-[140ms] ease-[ease] hover:translate-y-[-1px] hover:border-[#cdd4e6]"
    role="button"
    tabindex="0"
    @keydown.enter.self.prevent="emit('click-board')"
    @keydown.space.self.prevent="emit('click-board')"
    @click="emit('click-board')"
  >
    <div
      class="relative z-0 flex h-full w-full items-center justify-center overflow-hidden bg-[linear-gradient(135deg,#f7f8ff_0%,#eff2fb_100%)]"
    >
      <img
        v-if="previewSnapshotDataUrl"
        :src="previewSnapshotDataUrl"
        class="block h-full w-full object-cover"
        alt=""
      />
      <div
        v-else
        class="flex h-[58px] w-[58px] items-center justify-center rounded-[16px] bg-[linear-gradient(145deg,#7a6bd5,#8b7bdd)] text-[28px] font-[700] text-[#ffffff]"
      >
        {{ board.name.slice(0, 1).toUpperCase() }}
      </div>
      <div
        class="pointer-events-none absolute right-0 bottom-0 left-0 h-[92px] bg-[linear-gradient(to_top,rgba(245,248,255,0.88)_0%,rgba(245,248,255,0)_100%)]"
      ></div>
    </div>

    <div class="pointer-events-none absolute right-[56px] bottom-[12px] left-[12px] z-[2]">
      <input
        v-if="isRenaming"
        ref="renameInputEl"
        :value="renameDraft"
        class="pointer-events-auto h-[30px] w-full rounded-[8px] border border-[rgba(224,230,245,0.75)] bg-[rgba(255,255,255,0.82)] px-2 text-[13px] text-[#2b3142] outline-none"
        maxlength="60"
        @click.stop
        @input="onRenameInput"
        @blur="emit('commit-rename')"
        @keyup.enter.stop.prevent="emit('commit-rename')"
        @keyup.esc.stop.prevent="emit('cancel-rename')"
      />
      <div
        v-else
        class="overflow-hidden text-[14px] font-[650] text-ellipsis whitespace-nowrap text-[#293042]"
      >
        {{ board.name }}
      </div>
      <div class="mt-[3px] text-[12px] text-[#7b8297]">Edited {{ displayDate }}</div>
    </div>

    <div class="absolute right-[12px] bottom-[12px] z-[3]">
      <button
        class="board-menu-btn inline-flex h-[34px] w-[34px] cursor-pointer items-center justify-center border-none bg-transparent text-[#4e556b] hover:text-[#2b3142]"
        type="button"
        title="More actions"
        aria-label="More actions"
        @click.stop="emit('toggle-menu')"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="5" r="1.8" fill="currentColor" />
          <circle cx="12" cy="12" r="1.8" fill="currentColor" />
          <circle cx="12" cy="19" r="1.8" fill="currentColor" />
        </svg>
      </button>
    </div>

    <div
      v-if="isMenuOpen"
      class="board-action-menu absolute right-[12px] bottom-[48px] z-30 w-[92px] rounded-[10px] border border-[#d8ddea] bg-[#ffffff] p-1.5"
      @click.stop
    >
      <button
        type="button"
        class="h-[32px] w-full cursor-pointer rounded-[8px] border-none bg-transparent px-[10px] text-left text-[13px] font-[600] text-[#3f475d] hover:bg-[#eff3fb]"
        @click.stop="emit('rename-action')"
      >
        Rename
      </button>
      <button
        type="button"
        class="h-[32px] w-full cursor-pointer rounded-[8px] border-none bg-transparent px-[10px] text-left text-[13px] font-[600] text-[#3f475d] hover:bg-[#eff3fb]"
        @click.stop="emit('duplicate-action')"
      >
        Duplicate
      </button>
      <button
        type="button"
        class="h-[32px] w-full cursor-pointer rounded-[8px] border-none bg-transparent px-[10px] text-left text-[13px] font-[600] text-[#c23b42] hover:bg-[#fdeced]"
        @click.stop="emit('delete-action')"
      >
        Delete
      </button>
    </div>
  </article>
</template>

<script setup lang="ts">
import type { CanvasBoardMeta } from '../lib/canvasBoards'

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
}>()

const renameInputEl = ref<HTMLInputElement | null>(null)

watch(
  () => props.isRenaming,
  (isRenaming) => {
    if (!isRenaming) return
    void nextTick(() => {
      if (!renameInputEl.value) return
      renameInputEl.value.focus()
      renameInputEl.value.select()
    })
  },
)

function onRenameInput(event: Event) {
  emit('rename-input', (event.target as HTMLInputElement).value)
}
</script>
