<template>
  <Card :pt="pt">
    <template #title>
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <component :is="titleTag" class="m-0 block truncate">{{ title }}</component>
        </div>
        <div v-if="slots.actions" class="shrink-0">
          <slot name="actions" />
        </div>
      </div>
    </template>
    <template #content>
      <slot />
    </template>
  </Card>
</template>

<script setup lang="ts">
import { useSlots } from 'vue'
import Card from 'primevue/card'
import type { CardPassThroughOptions } from 'primevue/card'

withDefaults(
  defineProps<{
    title: string
    titleTag?: 'h2' | 'h3' | 'span'
    pt?: CardPassThroughOptions | undefined
  }>(),
  { titleTag: 'span', pt: undefined },
)

const slots = useSlots()
</script>
