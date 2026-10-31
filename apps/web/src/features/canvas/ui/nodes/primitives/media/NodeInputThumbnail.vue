<template>
  <div
    ref="thumbnailRef"
    class="node-input-thumbnail node-input-thumbnail-item"
    role="button"
    tabindex="0"
    v-bind="$attrs"
  >
    <img
      :src="thumbnailSrc"
      :alt="alt"
      :draggable="isDraggable"
      loading="lazy"
      decoding="async"
      :class="{ invisible: !thumbnailSrc, 'is-tier-loading': !!thumbnailSrc && !isThumbnailLoaded }"
      @load="onThumbnailLoad"
      @error="onThumbnailError"
    />
    <button
      v-if="isRemovable"
      class="node-input-thumbnail-remove"
      type="button"
      :title="resolvedRemoveTitle"
      :aria-label="resolvedRemoveAriaLabel"
      @mousedown.stop
      @pointerdown.stop
      @click.stop="emit('remove')"
    >
      <Xmark class="size-2.5" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useElementVisibility } from '@vueuse/core'
import { Xmark } from '@iconoir/vue'
import { useDisplayImagePreview, useTierFallback } from '../../../../session/flow-adapter'

const props = withDefaults(
  defineProps<{
    src: string
    assetId?: string | undefined
    alt?: string
    isDraggable?: boolean
    isRemovable?: boolean
    previewMaxSide?: number
    removeTitle?: string | undefined
    removeAriaLabel?: string | undefined
  }>(),
  {
    alt: '',
    assetId: undefined,
    isDraggable: false,
    isRemovable: false,
    previewMaxSide: 128,
    removeTitle: undefined,
    removeAriaLabel: undefined,
  },
)

const emit = defineEmits<{
  (e: 'remove'): void
}>()

const { t } = useI18n()

const resolvedRemoveTitle = computed(
  () => props.removeTitle ?? t('canvas.controls.removeImageInput'),
)
const resolvedRemoveAriaLabel = computed(
  () => props.removeAriaLabel ?? t('canvas.controls.removeImageInput'),
)
const thumbnailRef = ref<HTMLElement | null>(null)
const isThumbnailVisible = useElementVisibility(thumbnailRef)

const { displayUrl, contentFallbackUrl } = useDisplayImagePreview(() => props.src, {
  maxSide: () => props.previewMaxSide,
  assetId: () => props.assetId,
  isVisible: isThumbnailVisible,
})
const {
  src: thumbnailSrc,
  isLoaded: isThumbnailLoaded,
  onLoad: onThumbnailLoad,
  onError: onThumbnailError,
} = useTierFallback(displayUrl, contentFallbackUrl, () => [props.src, props.assetId])

defineOptions({
  inheritAttrs: false,
})
</script>

<style scoped>
.node-input-thumbnail img {
  transition: filter var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.node-input-thumbnail img.is-tier-loading {
  filter: blur(6px);
}
</style>
