<template>
  <DialogPortal v-bind="cleanProps(props)">
    <slot />
  </DialogPortal>
</template>

<script setup lang="ts">
import { DialogPortal, type DialogPortalProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DialogPortalProps>()
</script>
