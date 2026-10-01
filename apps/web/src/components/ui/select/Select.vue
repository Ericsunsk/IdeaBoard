<template>
  <SelectRoot
    v-bind="forwardedProps"
    @update:model-value="emits('update:modelValue', $event)"
  >
    <slot />
  </SelectRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  SelectRoot,
  useForwardPropsEmits,
  type SelectRootEmits,
  type SelectRootProps,
} from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<SelectRootProps>()
const emits = defineEmits<SelectRootEmits>()

// reka's Select wires trigger and content through provide/inject, so the root
// has to hand down a live binding rather than a snapshot. `useForwardPropsEmits`
// plus a computed wrapper is what `Popover.vue` and `DropdownMenu.vue` do; a
// plain `v-bind="cleanProps(props)"` freezes `modelValue` at mount and the list
// silently never opens.
const forwarded = useForwardPropsEmits(props, emits)
const forwardedProps = computed(() => cleanProps(forwarded.value))
</script>
