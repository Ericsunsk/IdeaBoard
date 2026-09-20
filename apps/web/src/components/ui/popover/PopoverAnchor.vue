<template>
  <PopoverAnchor v-bind="cleanProps(props)">
    <slot />
  </PopoverAnchor>
</template>

<script setup lang="ts">
import { PopoverAnchor, type PopoverAnchorProps } from 'reka-ui'
import { cleanProps } from '@/lib/utils'

const props = defineProps<PopoverAnchorProps>()
</script>
