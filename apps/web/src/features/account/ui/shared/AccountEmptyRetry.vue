<template>
  <div
    :class="containerClass"
    :role="props.announce === 'status' ? 'status' : undefined"
    :aria-live="props.announce === 'status' ? undefined : 'polite'"
  >
    <span v-if="props.title" :class="accountEmptyTitleClass">{{ props.title }}</span>
    <span v-if="props.description" :class="accountEmptyDescriptionClass">{{
      props.description
    }}</span>
    <Button variant="ghost" size="sm" :class="retryButtonClass" @click="emit('retry')">
      {{ props.retryLabel }}
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import {
  accountEmptyDescriptionClass,
  accountEmptyRetryClass,
  accountEmptyStateClass,
  accountEmptyStateTallClass,
  accountEmptyTitleClass,
  accountInlineRetryClass,
} from './ui'

const props = withDefaults(
  defineProps<{
    title?: string | undefined
    description?: string | undefined
    retryLabel: string
    variant?: 'default' | 'tall' | 'compact' | undefined
    announce?: 'polite' | 'status' | undefined
  }>(),
  {
    title: undefined,
    description: undefined,
    variant: 'default',
    announce: 'polite',
  },
)

const emit = defineEmits<{
  retry: []
}>()

const containerClass = computed(() => {
  if (props.variant === 'compact') return accountInlineRetryClass
  return props.variant === 'tall' ? accountEmptyStateTallClass : accountEmptyStateClass
})

const retryButtonClass = computed(() =>
  props.title || props.description ? accountEmptyRetryClass : undefined,
)
</script>
