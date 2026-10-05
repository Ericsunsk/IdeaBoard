<template>
  <Card :class="props.class" :elevated="props.elevated">
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
    /** Forwarded to `Card`. Flat surfaces pass `false` instead of cancelling a default shadow. */
    elevated?: boolean | undefined
  }>(),
  {
    titleTag: 'span',
    headerClass: 'p-6 pb-2',
    titleClass: 'text-admin-icon font-medium text-foreground',
    class: undefined,
    elevated: undefined,
  },
)

const slots = useSlots()
</script>
