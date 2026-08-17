<template>
  <div :class="rootClass">
    <div :class="panelClass">
      <slot v-if="hasToolbar" name="toolbar" />

      <div :class="resolvedContentClass">
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
} from '../lib/adminUiClasses'

const props = defineProps<{
  contentClass?: string
  scrollContent?: boolean
  viewportHeight?: boolean
}>()

const slots = useSlots()
const hasToolbar = computed(() => Boolean(slots.toolbar))
const rootClass = computed(() => (props.viewportHeight ? adminSurfaceRootViewportClass : 'min-w-0'))
const panelClass = computed(() => [
  adminSurfacePanelClass,
  props.viewportHeight && adminSurfacePanelViewportClass,
])
const resolvedContentClass = computed(() => [
  props.scrollContent && adminSurfaceScrollContentClass,
  props.contentClass || (hasToolbar.value ? 'px-6 pb-6' : 'p-6'),
])
</script>
