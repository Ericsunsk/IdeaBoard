<template>
  <SelectItemIndicator v-bind="cleanProps(props)">
    <slot />
  </SelectItemIndicator>
</template>

<script setup lang="ts">
import { SelectItemIndicator, type SelectItemIndicatorProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<SelectItemIndicatorProps>()
</script>
