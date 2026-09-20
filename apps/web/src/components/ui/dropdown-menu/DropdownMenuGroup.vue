<template>
  <DropdownMenuGroup v-bind="cleanProps(props)">
    <slot />
  </DropdownMenuGroup>
</template>

<script setup lang="ts">
import { DropdownMenuGroup, type DropdownMenuGroupProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DropdownMenuGroupProps>()
</script>
