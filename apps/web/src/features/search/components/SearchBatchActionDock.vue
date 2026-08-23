<template>
  <Teleport to="body">
    <Transition name="search-batch-action-dock">
      <div
        v-if="selectedCount > 0"
        class="search-batch-action-dock fixed inset-x-0 bottom-7 z-40 mx-auto h-[var(--search-dock-height)] w-fit gap-2 p-1.5"
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
          class="search-reference-control"
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
