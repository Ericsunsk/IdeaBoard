<template>
  <Teleport to="body">
    <Transition name="batch-dock">
      <div
        v-if="selectedCount > 0"
        class="fixed bottom-7 inset-x-0 mx-auto w-fit z-40 flex h-11 items-center gap-2 rounded-full border border-board-card-border bg-board-control-bg/80 p-1.5 backdrop-blur-md shadow-[0_4px_16px_rgba(45,64,89,0.04)] select-none pointer-events-auto"
        role="toolbar"
        aria-label="Batch actions"
      >
        <!-- 已选计数 -->
        <div class="flex items-center gap-2 pl-3 pr-2 border-r border-board-card-border">
          <div
            class="size-5 rounded-full bg-board-primary text-ib-text-inverse flex items-center justify-center text-[11px] font-bold font-mono"
          >
            {{ selectedCount }}
          </div>
          <span class="text-xs font-medium text-board-page-text whitespace-nowrap">Selected</span>
        </div>

        <!-- 批量作为视觉搜索参考图 (Vision Search) -->
        <button
          type="button"
          class="h-8 px-4 rounded-full bg-board-primary hover:bg-board-primary-hover text-ib-text-inverse text-xs font-bold shadow-none transition-colors flex items-center gap-1.5 cursor-pointer focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
          @click="emit('vision-search')"
        >
          <Search class="size-3.5" aria-hidden="true" />
          <span>Vision Search</span>
        </button>

        <!-- 批量存入个人素材库 (Save All to Library) -->
        <button
          type="button"
          class="h-8 px-3 rounded-full border border-board-card-border text-board-page-text-muted hover:text-board-page-text hover:border-board-primary hover:bg-board-primary/10 text-xs font-medium transition-[border-color,background-color,color] flex items-center gap-1.5 cursor-pointer focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
          @click="emit('save-all')"
        >
          <Bookmark class="size-3.5 text-board-primary" aria-hidden="true" />
          <span>Save All</span>
        </button>

        <!-- 取消多选 ✕ -->
        <button
          type="button"
          class="search-reference-action"
          title="Clear selection"
          aria-label="Clear selection"
          @click="emit('clear')"
        >
          <Xmark class="size-3.5" aria-hidden="true" />
        </button>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { Bookmark, Search, Xmark } from '@iconoir/vue'

defineProps<{
  selectedCount: number
}>()

const emit = defineEmits<{
  'vision-search': []
  'save-all': []
  clear: []
}>()
</script>

<style scoped>
.batch-dock-enter-active {
  transition:
    opacity var(--motion-enter-duration) var(--motion-enter-easing),
    transform var(--motion-enter-duration) var(--motion-enter-easing);
}

.batch-dock-leave-active {
  transition:
    opacity var(--motion-exit-duration) var(--motion-exit-easing),
    transform var(--motion-exit-duration) var(--motion-exit-easing);
}

.batch-dock-enter-from,
.batch-dock-leave-to {
  opacity: 0;
  transform: translateY(calc(100% + 3rem));
}

.search-reference-action {
  display: grid;
  width: 2rem;
  height: 2rem;
  flex-shrink: 0;
  cursor: pointer;
  place-items: center;
  border-radius: var(--radius-ib-pill);
  background-color: var(--color-board-loading-bg);
  color: var(--color-board-page-text-muted);
  transition:
    background-color var(--motion-control-duration) var(--motion-control-easing),
    color var(--motion-control-duration) var(--motion-control-easing),
    transform var(--motion-control-duration) var(--motion-control-easing);
}

.search-reference-action:hover {
  background-color: var(--color-board-card-border);
  color: var(--color-board-page-text);
  transform: scale(1.05);
}

.search-reference-action:active {
  transform: scale(0.95);
}

.search-reference-action:focus-visible {
  outline: none;
  box-shadow: var(--shadow-board-focus);
}
</style>
