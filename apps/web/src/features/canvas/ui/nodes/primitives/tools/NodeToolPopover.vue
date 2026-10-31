<template>
  <Transition v-bind="transientOverlayMotion">
    <div
      v-if="isVisible"
      v-bind="$attrs"
      class="node-tool-output-count-popover"
      :class="layoutClass"
    >
      <slot />
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { createPopoverMotionTransition } from '../../../../../../core/motion/popoverMotion'

const props = withDefaults(
  defineProps<{
    isVisible: boolean
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
const transientOverlayMotion = createPopoverMotionTransition()
</script>
