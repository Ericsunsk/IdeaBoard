<template>
  <SelectValue v-bind="cleanProps(props)">
    <slot />
  </SelectValue>
</template>

<script setup lang="ts">
import { SelectValue, type SelectValueProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<SelectValueProps>()
</script>
