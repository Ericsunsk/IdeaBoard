<template>
  <div :class="rootClass">
    <div :class="panelClass">
      <div :class="adminSurfaceToolbarClass">
        <div class="flex flex-wrap items-center gap-3">
          <slot name="toolbar-start" />
        </div>
        <div class="flex flex-wrap items-center justify-end gap-3">
          <slot name="toolbar-end" />
        </div>
      </div>

      <div :class="resolvedContentClass">
        <slot />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  adminSurfacePanelClass,
  adminSurfacePanelViewportClass,
  adminSurfaceRootViewportClass,
  adminSurfaceToolbarClass,
} from '../lib/adminUiClasses'

const props = defineProps<{
  contentClass?: string
  viewportHeight?: boolean
}>()

const rootClass = computed(() => (props.viewportHeight ? adminSurfaceRootViewportClass : 'min-w-0'))
const panelClass = computed(() => [
  adminSurfacePanelClass,
  props.viewportHeight && adminSurfacePanelViewportClass,
])
const resolvedContentClass = computed(() => props.contentClass || 'px-6 pb-6')
</script>
