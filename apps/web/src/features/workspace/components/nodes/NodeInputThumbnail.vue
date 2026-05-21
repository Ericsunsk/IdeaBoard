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
      <svg
        width="10"
        height="10"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.4"
        stroke-linecap="round"
      >
        <path d="M6 6l12 12" />
        <path d="M18 6L6 18" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
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
