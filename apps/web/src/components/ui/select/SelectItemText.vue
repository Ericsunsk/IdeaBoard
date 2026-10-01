<template>
  <SelectItemText v-bind="cleanProps(props)">
    <slot />
  </SelectItemText>
</template>

<script setup lang="ts">
import { SelectItemText, type SelectItemTextProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<SelectItemTextProps>()
</script>
