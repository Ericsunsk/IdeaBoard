<template>
  <span
    class="inline-flex [flex:0_0_auto] rounded-full border border-[rgba(15,23,42,0.08)]"
    :style="{
      width: `${size}px`,
      height: `${size}px`,
      background: color,
      borderWidth: borderless ? '0' : '1px',
    }"
  ></span>
</template>

<script setup lang="ts">
defineProps<{
  color: string
  size?: number
  borderless?: boolean
}>()
</script>
