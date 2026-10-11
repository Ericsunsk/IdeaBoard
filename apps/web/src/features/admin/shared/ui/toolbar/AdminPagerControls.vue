<template>
  <div class="flex items-center gap-1.5">
    <div :class="adminToolbarPageLabelClass">{{ currentPage }} / {{ totalPages }}</div>
    <AdminButton
      variant="ghost"
      size="icon"
      :class="adminCompactIconButtonClass"
      :disabled="prevDisabled"
      aria-label="上一页"
      @click="$emit('prev')"
    >
      <NavArrowLeft class="size-4 text-muted-foreground" />
    </AdminButton>
    <AdminButton
      variant="ghost"
      size="icon"
      :class="adminCompactIconButtonClass"
      :disabled="nextDisabled"
      aria-label="下一页"
      @click="$emit('next')"
    >
      <NavArrowRight class="size-4 text-muted-foreground" />
    </AdminButton>
  </div>
</template>

<script setup lang="ts">
import { NavArrowLeft, NavArrowRight } from '@iconoir/vue'
import AdminButton from '../AdminButton.vue'
import { adminToolbarPageLabelClass, adminCompactIconButtonClass } from '../../core/uiClasses'

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
