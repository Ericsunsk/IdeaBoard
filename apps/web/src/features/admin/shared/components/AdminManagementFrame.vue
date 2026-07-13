<template>
  <div :class="adminSurfaceRootViewportClass">
    <div :class="[adminSurfacePanelClass, adminSurfacePanelViewportClass]">
      <slot v-if="slots.toolbar" name="toolbar" />
      <div v-else-if="hasToolbar" :class="adminSurfaceToolbarClass">
        <div class="flex flex-wrap items-center gap-3">
          <slot name="toolbar-start" />
        </div>
        <div class="flex flex-wrap items-center justify-end gap-3">
          <slot name="toolbar-end" />
        </div>
      </div>

      <div :class="frameContentClass">
        <slot />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
import {
  adminSurfacePanelClass,
  adminSurfacePanelViewportClass,
  adminSurfaceRootViewportClass,
  adminSurfaceScrollContentClass,
  adminSurfaceToolbarClass,
} from '../lib/adminUiClasses'

const props = defineProps<{
  contentClass?: string
}>()

const slots = useSlots()
const hasToolbar = computed(() =>
  Boolean(slots.toolbar || slots['toolbar-start'] || slots['toolbar-end']),
)
const frameContentClass = computed(() => [
  adminSurfaceScrollContentClass,
  props.contentClass || (hasToolbar.value ? 'px-6 pb-6' : 'p-6'),
])
</script>
