<template>
  <PopoverRoot v-slot="slotProps" v-bind="forwardedProps">
    <slot v-bind="slotProps" />
  </PopoverRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  PopoverRoot,
  useForwardPropsEmits,
  type PopoverRootEmits,
  type PopoverRootProps,
} from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<PopoverRootProps>()
const emits = defineEmits<PopoverRootEmits>()

const forwarded = useForwardPropsEmits(props, emits)
const forwardedProps = computed(() => cleanProps(forwarded.value))
</script>
