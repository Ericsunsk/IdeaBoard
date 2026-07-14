<template>
  <Card :class="adminMasonryCardClass" :pt="adminCardPt">
    <template #title>
      <div class="flex min-w-0 items-center justify-between gap-3">
        <span class="truncate text-admin-title-sm font-bold text-admin-title-color">{{
          title
        }}</span>
        <Tag :value="status" class="shrink-0" :severity="statusSeverity" rounded />
      </div>
    </template>

    <template #content>
      <div class="flex flex-col gap-5">
        <slot />
        <div v-if="$slots.actions" class="flex flex-wrap items-center gap-2 pt-1">
          <slot name="actions" />
        </div>
      </div>
    </template>
  </Card>
</template>

<script setup lang="ts">
import Card from 'primevue/card'
import Tag from 'primevue/tag'
import { adminCardPt } from '../shared/lib/adminCardPt'
import { adminMasonryCardClass } from '../shared/lib/adminUiClasses'

withDefaults(
  defineProps<{
    status: string
    statusSeverity?: 'danger' | 'info' | 'secondary' | 'success' | 'warn'
    title: string
  }>(),
  { statusSeverity: 'secondary' },
)
</script>
