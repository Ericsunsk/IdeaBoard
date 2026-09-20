<template>
  <AdminTableRow :hoverable="false">
    <AdminTableCell
      :colspan="props.colspan"
      :class="cn('px-3 py-10 text-center align-middle', props.class)"
    >
      <slot />
    </AdminTableCell>
  </AdminTableRow>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import AdminTableRow from './AdminTableRow.vue'
import AdminTableCell from './AdminTableCell.vue'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    colspan?: number | undefined
  }>(),
  {
    class: undefined,
    colspan: 1,
  },
)
</script>
