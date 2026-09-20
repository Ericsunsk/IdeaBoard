<template>
  <TabsRoot
    v-bind="{ ...$attrs, ...rootProps }"
    :class="cn('flex flex-col', props.class)"
    @update:model-value="handleUpdate"
  >
    <slot />
  </TabsRoot>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { TabsRoot } from 'reka-ui'
import { cn } from '../lib/classNames'

const props = defineProps<{
  modelValue?: string
  defaultValue?: string
  class?: HTMLAttributes['class']
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

defineOptions({
  inheritAttrs: false,
})

const rootProps = computed(() => {
  const result: Record<string, unknown> = {}
  if (props.defaultValue !== undefined) {
    result.defaultValue = props.defaultValue
  }
  if (props.modelValue !== undefined) {
    result.modelValue = props.modelValue
  }
  return result
})

function handleUpdate(val: string | number) {
  emit('update:modelValue', String(val))
}
</script>
