<template>
  <span
    class="node-color-swatch"
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

<style scoped>
.node-color-swatch {
  display: inline-flex;
  border-radius: 999px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  flex: 0 0 auto;
}
</style>
