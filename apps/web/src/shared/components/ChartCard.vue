<template>
  <Card :class="props.class">
    <CardHeader :class="headerClass">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <CardTitle :as="titleTag" :class="titleClass">
            {{ title }}
          </CardTitle>
        </div>
        <div v-if="slots.actions" class="shrink-0">
          <slot name="actions" />
        </div>
      </div>
    </CardHeader>
    <CardContent class="p-6 pt-0">
      <slot />
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { useSlots } from 'vue'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'

// Neutral chart card primitive (#863): header/title skin rides explicit
// class props with user-surface defaults. Admin pages use
// `features/admin/shared` wrappers (`AdminChartCard`,
// `AdminRangedLineChartCard`) to supply the Admin skin instead of a
// feature-specific `variant` flag.
const props = withDefaults(
  defineProps<{
    title: string
    titleTag?: 'h2' | 'h3' | 'span'
    headerClass?: string | undefined
    titleClass?: string | undefined
    class?: string | undefined
  }>(),
  {
    titleTag: 'span',
    headerClass: 'p-6 pb-2',
    titleClass: 'text-ib-icon font-medium text-ib-text',
    class: undefined,
  },
)

const slots = useSlots()
</script>
