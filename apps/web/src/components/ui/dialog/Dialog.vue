<template>
  <DialogRoot v-bind="forwardedProps">
    <slot />
  </DialogRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  DialogRoot,
  useForwardPropsEmits,
  type DialogRootEmits,
  type DialogRootProps,
} from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DialogRootProps>()
const emits = defineEmits<DialogRootEmits>()

const forwarded = useForwardPropsEmits(props, emits)
const forwardedProps = computed(() => cleanProps(forwarded.value))
</script>
