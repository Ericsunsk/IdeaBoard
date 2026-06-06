<template>
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div class="min-w-0">
      <span class="block truncate">{{ title }}</span>
    </div>
    <div v-if="slots.actions" class="shrink-0">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useSlots } from 'vue'

defineProps<{
  title: string
}>()

const slots = useSlots()
</script>
