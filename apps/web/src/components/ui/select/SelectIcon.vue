<template>
  <SelectIcon v-bind="cleanProps(props)" as-child>
    <slot />
  </SelectIcon>
</template>

<script setup lang="ts">
import { SelectIcon, type SelectIconProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<SelectIconProps>()
</script>
