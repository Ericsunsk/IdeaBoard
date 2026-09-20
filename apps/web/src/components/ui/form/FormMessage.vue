<template>
  <p
    v-if="error"
    :id="formMessageId"
    :class="cn('text-[0.8rem] font-medium text-destructive', props.class)"
  >
    <slot>{{ error }}</slot>
  </p>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useFormField } from './context'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const { formMessageId, error } = useFormField()
</script>
