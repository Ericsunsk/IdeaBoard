<template>
  <DialogClose v-bind="cleanProps(props)">
    <slot />
  </DialogClose>
</template>

<script setup lang="ts">
import { DialogClose, type DialogCloseProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DialogCloseProps>()
</script>
