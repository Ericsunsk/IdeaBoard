<template>
  <DropdownMenuRadioGroup
    v-bind="cleanProps(props)"
    @update:model-value="emits('update:modelValue', $event)"
  >
    <slot />
  </DropdownMenuRadioGroup>
</template>

<script setup lang="ts">
import {
  DropdownMenuRadioGroup,
  type DropdownMenuRadioGroupEmits,
  type DropdownMenuRadioGroupProps,
} from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DropdownMenuRadioGroupProps>()
const emits = defineEmits<DropdownMenuRadioGroupEmits>()
</script>
