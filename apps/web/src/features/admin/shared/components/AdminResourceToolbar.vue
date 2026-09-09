<template>
  <div :class="adminSurfaceToolbarClass">
    <div :class="toolbarStartGroupClass">
      <slot name="start">
        <slot name="search" />
        <div v-if="hasMetricsContent" :class="toolbarMetricGroupClass">
          <slot name="metrics" :metric-class="adminToolbarMetricClass" />
        </div>
      </slot>
    </div>

    <div v-if="hasEndContent" :class="toolbarEndGroupClass">
      <slot name="end">
        <div v-if="hasFiltersContent" :class="toolbarEndSlotGroupClass">
          <slot name="filters" />
        </div>
        <div v-if="hasActionsContent" :class="toolbarEndSlotGroupClass">
          <slot name="actions" :icon-button-class="adminFramedIconButtonClass" />
        </div>
        <div v-if="hasPagerContent" :class="toolbarPagerGroupClass">
          <slot name="pager" />
        </div>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
import {
  adminSurfaceToolbarClass,
  adminFramedIconButtonClass,
  adminToolbarMetricClass,
} from '../lib/uiClasses'

const slots = useSlots()

const toolbarStartGroupClass = 'flex min-w-0 flex-wrap items-center gap-3'
const toolbarMetricGroupClass = 'flex min-w-0 flex-wrap items-center gap-3'
const toolbarEndGroupClass =
  'flex min-w-0 flex-wrap items-center justify-start gap-3 sm:justify-end'
const toolbarEndSlotGroupClass = 'flex min-w-0 flex-wrap items-center gap-3'
const toolbarPagerGroupClass = 'flex shrink-0 items-center'

const hasMetricsContent = computed(() => Boolean(slots.metrics))
const hasFiltersContent = computed(() => Boolean(slots.filters))
const hasActionsContent = computed(() => Boolean(slots.actions))
const hasPagerContent = computed(() => Boolean(slots.pager))
const hasEndContent = computed(
  () =>
    Boolean(slots.end) ||
    hasFiltersContent.value ||
    hasActionsContent.value ||
    hasPagerContent.value,
)
</script>
