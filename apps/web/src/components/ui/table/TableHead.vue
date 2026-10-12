<template>
  <th
    data-ui="table-head"
    :scope="props.scope"
    :aria-sort="props.sortable ? props.sortDirection : undefined"
    :class="cn(tableHeadClasses, props.class)"
  >
    <button
      v-if="props.sortable"
      type="button"
      :class="tableSortButtonClasses"
      @click="emits('sort', $event)"
    >
      <slot />
      <Sort aria-hidden="true" class="size-3.5 shrink-0" />
    </button>
    <slot v-else />
  </th>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { Sort } from '@iconoir/vue'
import { cn } from '@/lib/utils'
import { tableHeadClasses, tableSortButtonClasses } from './tableClasses'

/**
 * Column header. Plain headers render text directly so they never steal tab
 * stops; `sortable` wraps the label in a real `<button>` (keyboard-focusable
 * and operable with Enter/Space) and publishes the direction through
 * `aria-sort` on the `<th>`. The parent owns the direction state and flips
 * it in its `sort` handler.
 */
const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class']
    scope?: string
    sortable?: boolean
    sortDirection?: 'ascending' | 'descending' | 'none'
  }>(),
  { class: undefined, scope: 'col', sortable: false, sortDirection: 'none' },
)

const emits = defineEmits<{
  sort: [event: MouseEvent]
}>()
</script>
