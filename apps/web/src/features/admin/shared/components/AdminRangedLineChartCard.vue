<template>
  <RangedLineChartCard
    v-bind="forwarded"
    v-model="selectedRange"
    :elevated="false"
    header-class="p-6 pb-4"
    title-class="text-base font-bold text-admin-text-1"
    range-group-class="inline-flex h-8 items-center rounded-lg border border-admin-border-soft bg-admin-surface-muted p-1 text-xs text-admin-text-3"
    range-selected-class="bg-white font-semibold text-admin-text-1 shadow-xs focus-visible:ring-admin-accent"
    range-unselected-class="text-admin-text-2 hover:text-admin-text-1 focus-visible:ring-admin-accent"
  >
    <template #before-chart>
      <slot name="before-chart" />
    </template>
    <template #chart-fallback>
      <slot name="chart-fallback" />
    </template>
    <template #after-chart>
      <slot name="after-chart" />
    </template>
  </RangedLineChartCard>
</template>

<script setup lang="ts" generic="TRange extends string">
import RangedLineChartCard, {
  type RangedLineChartProps,
} from '../../../../shared/components/RangedLineChartCard.vue'

// Admin skin adapter for the neutral shared ranged chart primitive (#863).
// Visual styling stays here so Admin route pages never copy chart skin and
// generic `shared` modules never carry Admin semantics. Data/option props
// are typed from the shared primitive (skinned keys omitted); only the
// Admin skin is pinned here.
type AdminRangedLineChartProps<TRange extends string> = Omit<
  RangedLineChartProps<TRange>,
  | 'headerClass'
  | 'titleClass'
  | 'rangeGroupClass'
  | 'rangeSelectedClass'
  | 'rangeUnselectedClass'
> & {
  class?: string | undefined
}

const forwarded = defineProps<AdminRangedLineChartProps<TRange>>()

const selectedRange = defineModel<TRange>({ required: true })

defineOptions({ inheritAttrs: false })
</script>
