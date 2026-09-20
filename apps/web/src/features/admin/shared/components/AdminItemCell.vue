<template>
  <div class="flex items-center gap-3">
    <div
      v-if="icon"
      class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-admin-border-soft bg-admin-surface-muted text-admin-accent"
    >
      <component :is="icon" class="size-4" />
    </div>
    <div class="min-w-0">
      <div class="text-admin-body font-bold text-admin-title-color">{{ title }}</div>
      <div v-if="description" class="truncate text-admin-caption text-admin-text-3">
        {{ description }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  title: string
  description?: string | undefined
  icon?: Component | undefined
}>()
</script>
