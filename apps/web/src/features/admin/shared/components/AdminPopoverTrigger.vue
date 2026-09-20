<template>
  <PopoverTrigger :as-child="asChild" v-bind="$attrs">
    <slot />
  </PopoverTrigger>
</template>

<script setup lang="ts">
import { PopoverTrigger } from 'reka-ui'

defineProps<{ asChild?: boolean }>()
</script>
