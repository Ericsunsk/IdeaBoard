<template>
  <AdminManagementFrame
    :content-class="contentClass"
    :is-scrolling="isScrolling"
    @scroll="$emit('scroll')"
  >
    <template #toolbar-start>
      <slot name="toolbar-start" />
    </template>

    <template #toolbar-end>
      <slot name="toolbar-end" />
    </template>

    <div v-if="isLoading" :class="adminLoadingStateClass">
      <ProgressSpinner class="!h-9 !w-9 text-admin-accent" stroke-width="4" />
    </div>

    <div v-else-if="isEmpty" :class="adminEmptyStateClass">
      <div class="text-admin-title font-semibold text-admin-title-color">
        {{ emptyTitle }}
      </div>
      <div class="mt-2 text-admin-meta text-admin-subtitle-color">
        {{ emptyDescription }}
      </div>
    </div>

    <div v-else :class="resolvedGridClass">
      <slot />
    </div>
  </AdminManagementFrame>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ProgressSpinner from 'primevue/progressspinner'
import AdminManagementFrame from './AdminManagementFrame.vue'
import {
  adminEmptyStateClass,
  adminLoadingStateClass,
  adminMasonryGridClass,
} from '../lib/adminUiClasses'

const props = defineProps<{
  contentClass?: string
  emptyDescription: string
  emptyTitle: string
  gridClass?: string
  isEmpty: boolean
  isLoading: boolean
  isScrolling?: boolean
}>()
defineEmits<{ scroll: [] }>()

const resolvedGridClass = computed(() => props.gridClass || adminMasonryGridClass)
</script>
