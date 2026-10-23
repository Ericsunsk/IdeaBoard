<template>
  <RangedLineChartCard
    v-bind="forwarded"
    v-model="selectedRange"
    :elevated="false"
    header-class="p-6 pb-4"
    title-class="text-base font-bold text-foreground"
    range-group-class="inline-flex h-8 items-center rounded-control border border-border bg-muted p-1 text-xs text-muted-foreground"
    range-selected-class="bg-card font-semibold text-foreground shadow-raised focus-visible:ring-ring"
    range-unselected-class="text-muted-foreground hover:text-foreground focus-visible:ring-ring"
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
} from '@/core/components/RangedLineChartCard.vue'

// Admin skin adapter for the neutral shared ranged chart primitive (#863).
// Visual styling stays here so Admin route pages never copy chart skin and
// generic `shared` modules never carry Admin semantics. Data/option props
// are typed from the shared primitive (skinned keys omitted); only the
// Admin skin is pinned here.
type AdminRangedLineChartProps<TRange extends string> = Omit<
  RangedLineChartProps<TRange>,
  'headerClass' | 'titleClass' | 'rangeGroupClass' | 'rangeSelectedClass' | 'rangeUnselectedClass'
> & {
  class?: string | undefined
}

const forwarded = defineProps<AdminRangedLineChartProps<TRange>>()

const selectedRange = defineModel<TRange>({ required: true })

defineOptions({ inheritAttrs: false })
</script>
