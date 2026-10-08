<template>
  <div v-if="view.preview.urls.length > 0" class="flex items-center gap-1">
    <button
      v-for="(url, index) in view.preview.urls"
      :key="url"
      type="button"
      class="block size-8 shrink-0 cursor-pointer overflow-hidden rounded-lg border border-border bg-muted"
      :aria-label="MONITOR_MESSAGES.resultInspectAria(row.task_id, index + 1)"
      @click="emit('inspect', row)"
    >
      <img
        :src="url"
        :alt="MONITOR_MESSAGES.resultImageAltIndex(row.task_id, index + 1)"
        class="block h-full w-full object-cover"
        loading="lazy"
      />
    </button>
    <AdminMono
      v-if="view.preview.overflow > 0"
      class="shrink-0 text-xs font-semibold text-muted-foreground"
    >
      +{{ view.preview.overflow }}
    </AdminMono>
  </div>
  <AdminMono v-else class="text-xs text-muted-foreground">{{ ADMIN_IDENTITY_EMPTY }}</AdminMono>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AdminMono from '../../shared/components/AdminMono.vue'
import { ADMIN_IDENTITY_EMPTY } from '../../shared/lib/cellPlaceholders'
import { MONITOR_MESSAGES } from './pages/messages'
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
