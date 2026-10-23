<template>
  <div class="flex min-w-0 items-center gap-1.5 font-mono text-xs text-foreground">
    <component :is="icon" v-if="icon" class="size-4 shrink-0 text-muted-foreground" />
    <div class="flex min-w-0 flex-col">
      <span class="truncate text-foreground">{{ props.service }}</span>
      <span v-if="subtitle" class="truncate text-admin-caption text-muted-foreground">{{
        subtitle
      }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import { getServiceIcon } from '@/core/components/serviceIcons'
import { getAdminServiceGroupMeta, getAdminServiceMeta } from '../../core/serviceAdminMeta'

const props = defineProps<{
  service: string
  fallbackSubtitle?: string | undefined
  fallbackIcon?: Component | undefined
}>()

const icon = computed(() => getServiceIcon(props.service) ?? props.fallbackIcon)
const subtitle = computed(() => {
  const meta = getAdminServiceMeta(props.service)
  return meta ? getAdminServiceGroupMeta(meta.groupKey).title : props.fallbackSubtitle
})
</script>
