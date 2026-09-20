<template>
  <DialogTrigger v-bind="cleanProps(props)">
    <slot />
  </DialogTrigger>
</template>

<script setup lang="ts">
import { DialogTrigger, type DialogTriggerProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DialogTriggerProps>()
</script>
