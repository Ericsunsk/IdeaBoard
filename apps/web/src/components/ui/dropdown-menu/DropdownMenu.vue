<template>
  <DropdownMenuRoot v-slot="slotProps" v-bind="forwardedProps">
    <slot v-bind="slotProps" />
  </DropdownMenuRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  DropdownMenuRoot,
  useForwardPropsEmits,
  type DropdownMenuRootEmits,
  type DropdownMenuRootProps,
} from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DropdownMenuRootProps>()
const emits = defineEmits<DropdownMenuRootEmits>()

const forwarded = useForwardPropsEmits(props, emits)
const forwardedProps = computed(() => cleanProps(forwarded.value))
</script>
