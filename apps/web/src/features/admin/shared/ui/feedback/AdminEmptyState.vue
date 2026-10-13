<template>
  <div
    class="flex flex-col items-center justify-center text-center select-none"
    :class="compact ? 'py-6 px-4' : 'py-12 px-6'"
    role="status"
  >
    <div
      class="flex shrink-0 items-center justify-center rounded-overlay border border-border bg-muted/60 text-muted-foreground"
      :class="compact ? 'size-10 mb-2' : 'size-12 mb-3'"
    >
      <component :is="icon || EmptyPage" :class="compact ? 'size-5' : 'size-6'" />
    </div>
    <div class="text-admin-body font-semibold text-muted-foreground">
      {{ title }}
    </div>
    <div v-if="description" class="mt-1 max-w-sm text-admin-caption text-muted-foreground">
      {{ description }}
    </div>
    <div v-if="$slots.actions" class="mt-3 flex items-center gap-2">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { EmptyPage } from '@iconoir/vue'

withDefaults(
  defineProps<{
    title?: string
    description?: string
    icon?: Component | undefined
    compact?: boolean
  }>(),
  {
    title: '暂无数据',
    description: '',
    icon: undefined,
    compact: false,
  },
)
</script>
