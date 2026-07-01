<template>
  <Transition name="node-tool-popover">
    <div
      v-if="visible"
      v-bind="$attrs"
      class="node-tool-output-count-popover"
      :class="layoutClass"
    >
      <slot />
    </div>
  </Transition>
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

defineOptions({
  inheritAttrs: false,
})

const layoutClass = props.layout === 'row' ? 'node-tool-popover-row' : 'node-tool-popover-grid'
</script>
