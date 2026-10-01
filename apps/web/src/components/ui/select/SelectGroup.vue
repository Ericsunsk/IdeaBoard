<template>
  <SelectGroup v-bind="cleanProps(props)">
    <slot />
  </SelectGroup>
</template>

<script setup lang="ts">
import { SelectGroup, type SelectGroupProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<SelectGroupProps>()
</script>
