<template>
  <div
    class="node-input-thumbnail node-input-thumbnail-item"
    role="button"
    tabindex="0"
    v-bind="$attrs"
  >
    <img :src="displayUrl" :alt="alt" :draggable="draggable" />
    <button
      v-if="removable"
      class="node-input-thumbnail-remove"
      type="button"
      :title="removeTitle"
      :aria-label="removeAriaLabel"
      @mousedown.stop
      @pointerdown.stop
      @click.stop="emit('remove')"
    >
      <Xmark class="size-2.5" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import { useDisplayImagePreview } from '../../composables/useDisplayImagePreview'

const props = withDefaults(
  defineProps<{
    src: string
    alt?: string
    draggable?: boolean
    removable?: boolean
    previewMaxSide?: number
    removeTitle?: string
    removeAriaLabel?: string
  }>(),
  {
    alt: '',
    draggable: false,
    removable: false,
    previewMaxSide: 128,
    removeTitle: '移除图片输入',
    removeAriaLabel: '移除图片输入',
  },
)

const emit = defineEmits<{
  (e: 'remove'): void
}>()

const { displayUrl } = useDisplayImagePreview(() => props.src, {
  maxSide: props.previewMaxSide,
})

defineOptions({
  inheritAttrs: false,
})
</script>

<style src="./styles/node-input-thumbnails.css"></style>
