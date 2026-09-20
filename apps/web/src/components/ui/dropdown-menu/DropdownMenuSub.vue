<template>
  <DropdownMenuSub v-bind="cleanProps(props)" @update:open="emit('update:open', $event)">
    <slot />
  </DropdownMenuSub>
</template>

<script setup lang="ts">
import { DropdownMenuSub, type DropdownMenuSubEmits, type DropdownMenuSubProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<DropdownMenuSubProps>()
const emit = defineEmits<DropdownMenuSubEmits>()
</script>
