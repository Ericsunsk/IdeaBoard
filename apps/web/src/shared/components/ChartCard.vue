<template>
  <Card :class="cn(variant === 'admin' && 'shadow-none', props.class)">
    <CardHeader :class="variant === 'admin' ? 'p-6 pb-4' : 'p-6 pb-2'">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <CardTitle
            :as="titleTag"
            :class="
              variant === 'admin'
                ? 'text-base font-bold text-admin-text-1'
                : 'text-ib-icon font-medium text-ib-text'
            "
          >
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
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    title: string
    titleTag?: 'h2' | 'h3' | 'span'
    variant?: 'default' | 'admin'
    class?: string | undefined
  }>(),
  {
    titleTag: 'span',
    variant: 'default',
    class: undefined,
  },
)

const slots = useSlots()
</script>
