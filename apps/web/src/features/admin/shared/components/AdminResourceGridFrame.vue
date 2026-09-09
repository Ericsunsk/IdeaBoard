<template>
  <AdminSurfaceFrame viewport-height scroll-content :content-class="contentClass">
    <template v-if="$slots.toolbar" #toolbar>
      <slot name="toolbar" />
    </template>

    <div v-if="isLoading" :class="adminLoadingStateClass">
      <ProgressSpinner class="!h-9 !w-9 text-admin-accent" stroke-width="4" />
    </div>

    <div v-else-if="isEmpty" :class="resolvedGridClass">
      <slot name="prepend" />
      <div v-if="emptyTitle || emptyDescription" :class="adminEmptyStateClass">
        <div class="text-admin-title font-semibold text-admin-title-color">
          {{ emptyTitle }}
        </div>
        <div v-if="emptyDescription" class="mt-2 text-admin-meta text-admin-subtitle-color">
          {{ emptyDescription }}
        </div>
      </div>
    </div>

    <div v-else :class="resolvedGridClass">
      <slot name="prepend" />
      <slot />
    </div>
  </AdminSurfaceFrame>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ProgressSpinner from 'primevue/progressspinner'
import AdminSurfaceFrame from './AdminSurfaceFrame.vue'
import {
  adminEmptyStateClass,
  adminLoadingStateClass,
  adminMasonryGridClass,
} from '../lib/uiClasses'

const props = defineProps<{
  contentClass?: string | undefined
  emptyDescription?: string
  emptyTitle: string
  gridClass?: string
  isEmpty: boolean
  isLoading: boolean
}>()

const resolvedGridClass = computed(() => props.gridClass || adminMasonryGridClass)
</script>
