<template>
  <DropdownMenuTrigger v-bind="cleanProps(props)">
    <slot />
  </DropdownMenuTrigger>
</template>

<script setup lang="ts">
import { DropdownMenuTrigger, type DropdownMenuTriggerProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DropdownMenuTriggerProps>()
</script>
