<template>
  <div class="flex items-center gap-1.5">
    <div :class="adminToolbarPageLabelClass">{{ currentPage }} / {{ totalPages }}</div>
    <Button
      text
      :class="adminToolbarPagerButtonClass"
      :disabled="prevDisabled"
      aria-label="上一页"
      @click="$emit('prev')"
    >
      <template #icon>
        <NavArrowLeft class="size-4 text-admin-text-2" />
      </template>
    </Button>
    <Button
      text
      :class="adminToolbarPagerButtonClass"
      :disabled="nextDisabled"
      aria-label="下一页"
      @click="$emit('next')"
    >
      <template #icon>
        <NavArrowRight class="size-4 text-admin-text-2" />
      </template>
    </Button>
  </div>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import { NavArrowLeft, NavArrowRight } from '@iconoir/vue'
import { adminToolbarPageLabelClass, adminToolbarPagerButtonClass } from '../lib/uiClasses'

defineProps<{
  currentPage: number
  totalPages: number
  prevDisabled: boolean
  nextDisabled: boolean
}>()

defineEmits<{
  prev: []
  next: []
}>()
</script>
