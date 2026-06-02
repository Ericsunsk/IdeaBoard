<template>
  <div
    v-if="visible"
    v-motion="{
      initial: { opacity: 0, y: 6, scale: 0.985 },
      enter: {
        opacity: 1,
        y: 0,
        scale: 1,
        transition: { duration: 150, easing: 'ease-out' },
      },
    }"
    class="node-tool-output-count-popover"
    :class="layoutClass"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    visible: boolean
    layout?: 'grid' | 'row'
  }>(),
  {
    layout: 'grid',
  },
)

const layoutClass = props.layout === 'row' ? 'node-tool-popover-row' : 'node-tool-popover-grid'
</script>
