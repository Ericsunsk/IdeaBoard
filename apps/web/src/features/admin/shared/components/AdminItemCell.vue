<template>
  <div class="flex items-center gap-3">
    <div
      v-if="icon"
      class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-primary"
    >
      <component :is="icon" class="size-4" />
    </div>
    <div class="min-w-0">
      <div class="text-admin-body font-bold text-foreground">{{ title }}</div>
      <div v-if="description" class="truncate text-admin-caption text-muted-foreground">
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
