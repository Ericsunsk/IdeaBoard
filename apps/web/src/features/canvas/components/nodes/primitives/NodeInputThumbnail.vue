<template>
  <div
    ref="thumbnailRef"
    class="node-input-thumbnail node-input-thumbnail-item"
    role="button"
    tabindex="0"
    v-bind="$attrs"
  >
    <img
      :src="displayUrl || undefined"
      :alt="alt"
      :draggable="draggable"
      :class="{ invisible: !displayUrl }"
    />
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
import { ref } from 'vue'
import { useElementVisibility } from '@vueuse/core'
import { Xmark } from '@iconoir/vue'
import { useDisplayImagePreview } from '../../../vue-flow-adapter/useDisplayImagePreview'

const props = withDefaults(
  defineProps<{
    src: string
    mediaAssetId?: number
    alt?: string
    draggable?: boolean
    removable?: boolean
    previewMaxSide?: number
    removeTitle?: string
    removeAriaLabel?: string
  }>(),
  {
    alt: '',
    mediaAssetId: undefined,
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

const thumbnailRef = ref<HTMLElement | null>(null)
const isThumbnailVisible = useElementVisibility(thumbnailRef)

const { displayUrl } = useDisplayImagePreview(() => props.src, {
  maxSide: () => props.previewMaxSide,
  mediaAssetId: () => props.mediaAssetId,
  isVisible: isThumbnailVisible,
})

defineOptions({
  inheritAttrs: false,
})
</script>
