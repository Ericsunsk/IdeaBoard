<template>
  <TabsRoot v-bind="forwardedProps" @update:model-value="emits('update:modelValue', $event)">
    <slot />
  </TabsRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { TabsRoot, useForwardPropsEmits, type TabsRootEmits, type TabsRootProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<TabsRootProps>()
const emits = defineEmits<TabsRootEmits>()

// reka's Tabs wires list, triggers and content through provide/inject, so the
// root has to hand down a live binding rather than a snapshot.
// `useForwardPropsEmits` plus a computed wrapper is what `Select.vue` does; a
// plain `v-bind="cleanProps(props)"` freezes `modelValue` at mount and
// selection silently never updates.
const forwarded = useForwardPropsEmits(props, emits)
const forwardedProps = computed(() => cleanProps(forwarded.value))
</script>
