<template>
  <div v-if="view.preview.urls.length > 0" class="flex items-center gap-1">
    <button
      v-for="(url, index) in view.preview.urls"
      :key="url"
      type="button"
      class="block size-8 shrink-0 cursor-pointer overflow-hidden rounded-lg border border-admin-border-soft bg-admin-surface-muted"
      :aria-label="`查看任务 ${row.task_id} 结果 ${index + 1} 详情`"
      @click="emit('inspect', row)"
    >
      <img
        :src="url"
        :alt="`任务 ${row.task_id} 结果 ${index + 1}`"
        class="block h-full w-full object-cover"
        loading="lazy"
      />
    </button>
    <span
      v-if="view.preview.overflow > 0"
      class="shrink-0 font-mono text-xs font-semibold text-admin-text-3"
    >
      +{{ view.preview.overflow }}
    </span>
  </div>
  <span v-else class="font-mono text-xs text-admin-text-3">-</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { getTaskRowView } from './columns'
import type { AdminTaskRow } from '../application/useAdminTaskList'

const props = defineProps<{
  row: AdminTaskRow
}>()

const emit = defineEmits<{
  inspect: [row: AdminTaskRow]
}>()

const view = computed(() => getTaskRowView(props.row))
</script>
