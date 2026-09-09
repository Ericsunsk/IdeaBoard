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
import { useDisplayImagePreview } from '../../../vue-flow-adapter/useDisplayImagePreview'

const props = withDefaults(
  defineProps<{
    src: string
    assetId?: string | undefined
    alt?: string
    draggable?: boolean
    removable?: boolean
    previewMaxSide?: number
    removeTitle?: string | undefined
    removeAriaLabel?: string | undefined
  }>(),
  {
    alt: '',
    assetId: undefined,
    draggable: false,
    removable: false,
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

const { displayUrl } = useDisplayImagePreview(() => props.src, {
  maxSide: () => props.previewMaxSide,
  assetId: () => props.assetId,
  isVisible: isThumbnailVisible,
})

defineOptions({
  inheritAttrs: false,
})
</script>
