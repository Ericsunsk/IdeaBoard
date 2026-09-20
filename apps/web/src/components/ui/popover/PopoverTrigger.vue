<template>
  <PopoverTrigger v-bind="cleanProps(props)">
    <slot />
  </PopoverTrigger>
</template>

<script setup lang="ts">
import { PopoverTrigger, type PopoverTriggerProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<PopoverTriggerProps>()
</script>
