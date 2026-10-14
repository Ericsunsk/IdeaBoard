<template>
  <div
    ref="scrollAreaRef"
    class="mt-2 h-full min-h-0 flex-1 overflow-y-auto pr-0.5 pt-1 rounded-overlay [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    :aria-busy="isLoading"
    @scroll="emit('scroll')"
  >
    <!-- 骨架加载状态 -->
    <div
      v-if="isLoading && assets.length === 0"
      class="grid h-full min-h-40 grid-cols-2 gap-2 overflow-hidden"
      aria-busy="true"
    >
      <div v-for="n in 6" :key="n" class="h-28 animate-pulse rounded-panel bg-muted"></div>
    </div>

    <!-- 空状态提示 -->
    <div
      v-else-if="!isLoading && assets.length === 0 && !errorMessage"
      class="grid h-full min-h-40 place-items-center rounded-overlay border border-dashed border-border px-5 text-center"
    >
      <div>
        <p class="m-0 text-sm font-semibold">{{ t('library.empty.noSavedReferences') }}</p>
        <p class="mt-1 mb-0 text-xs text-muted-foreground">
          {{ t('library.empty.dragOrSaveHint') }}
        </p>
      </div>
    </div>

    <!-- 瀑布流资产列表 -->
    <div v-else class="pb-2">
      <div
        ref="galleryGridRef"
        class="relative min-w-0"
        :class="isExpanded ? 'gap-3' : 'gap-2'"
        :style="{ height: `${galleryHeight}px` }"
      >
        <article
          v-for="{ asset, style } in positionedAssets"
          :key="asset.id"
          ref="cardElements"
          :data-asset-id="asset.id"
          :style="style"
          :draggable="false"
          class="group/libcard relative overflow-hidden rounded-panel bg-card/80 backdrop-blur-md shadow-none transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-grab active:cursor-grabbing select-none"
          @dragstart.prevent
        >
          <button
            type="button"
            class="block w-full cursor-zoom-in text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            :aria-label="
              t('library.actions.viewSavedReference', {
                title: asset.title || t('library.states.savedReference'),
              })
            "
            @pointerdown="emit('asset-pointer-down', $event, asset)"
            @click="emit('open-asset', asset)"
          >
            <div
              class="relative overflow-hidden bg-muted/30 backdrop-blur-md"
              :style="{
                aspectRatio: `${asset.width || 1} / ${asset.height || 1}`,
                minHeight: failedImages.has(asset.id) ? '6rem' : undefined,
              }"
            >
              <div
                v-if="failedImages.has(asset.id)"
                class="grid min-h-24 place-items-center px-2 text-center text-xs text-muted-foreground"
              >
                {{ t('library.states.imageUnavailable') }}
              </div>
              <img
                v-else
                :src="resolveLibraryAssetCardURL(asset)"
                :alt="asset.title || t('library.states.savedReference')"
                loading="lazy"
                class="h-full w-full object-cover"
                @error="markImageError(asset.id)"
              />
            </div>
          </button>

          <!-- 悬浮微渐变覆盖层 -->
          <div
            class="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-0 group-hover/libcard:opacity-100 transition-opacity duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] flex flex-col justify-between p-2 pointer-events-none"
          >
            <div class="flex justify-end gap-1.5 pointer-events-auto">
              <button
                type="button"
                :class="[galleryOverlayButtonClass, 'hover:bg-destructive']"
                :aria-label="t('library.actions.removeSavedReference')"
                :title="t('library.actions.removeSavedReference')"
                @click="emit('remove-asset', asset)"
              >
                <Xmark class="size-3" aria-hidden="true" />
              </button>
              <button
                v-if="isCanvasPresentation"
                type="button"
                :class="[
                  galleryOverlayButtonClass,
                  'hover:bg-primary hover:text-primary-foreground',
                ]"
                :aria-label="t('library.actions.addToCanvas')"
                :title="t('library.actions.addToCanvas')"
                @click.stop="emit('add-to-canvas', toCanvasLibraryImportDescriptor(asset))"
              >
                <Plus class="size-3" aria-hidden="true" />
              </button>
              <button
                v-else
                type="button"
                :class="[
                  galleryOverlayButtonClass,
                  'hover:bg-primary hover:text-primary-foreground',
                ]"
                :aria-label="t('library.actions.useAsSearchReference')"
                :title="t('library.actions.useAsSearchReference')"
                @click.stop="emit('select-reference', asset)"
              >
                <Check
                  v-if="referenceKeys?.includes(libraryAssetKey(asset.provider, asset.sourceId))"
                  class="size-3"
                  aria-hidden="true"
                />
                <Search v-else class="size-3" aria-hidden="true" />
              </button>
            </div>
            <div class="min-w-0 pointer-events-auto">
              <p class="m-0 truncate text-admin-caption font-medium text-white leading-tight">
                {{ asset.title || t('library.states.untitledReference') }}
              </p>
              <p class="mt-0.5 mb-0 truncate text-admin-micro text-white/70">
                {{ asset.author || asset.provider }}
              </p>
            </div>
          </div>
        </article>
      </div>

      <!-- 加载更多按钮 -->
      <button
        v-if="hasMore"
        type="button"
        class="mx-auto my-3 block rounded-pill border border-border bg-card/80 backdrop-blur-md px-3 py-1.5 text-xs font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 cursor-pointer"
        :disabled="isLoading"
        @click="emit('load-more')"
      >
        {{ t('library.actions.loadMore') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { Check, Plus, Search, Xmark } from '@iconoir/vue'
import { libraryAssetKey, resolveLibraryAssetCardURL } from '../core/validators'
import type { LibraryAssetView } from '../core/transfer'
import {
  toCanvasLibraryImportDescriptor,
  type CanvasLibraryImportDescriptor,
} from '../core/transfer'

const props = defineProps<{
  assets: LibraryAssetView[]
  isLoading: boolean
  errorMessage: string | null
  hasMore: boolean
  isExpanded: boolean
  isCanvasPresentation: boolean
  referenceKeys?: string[] | undefined
  windowWidth: number
}>()

const emit = defineEmits<{
  scroll: []
  'load-more': []
  'open-asset': [asset: LibraryAssetView]
  'remove-asset': [asset: LibraryAssetView]
  'add-to-canvas': [descriptor: CanvasLibraryImportDescriptor]
  'select-reference': [asset: LibraryAssetView]
  'asset-pointer-down': [e: PointerEvent, asset: LibraryAssetView]
}>()

const { t } = useI18n()
const scrollAreaRef = ref<HTMLElement | null>(null)
const failedImages = ref(new Set<string>())

const galleryOverlayButtonClass =
  'grid size-6 shrink-0 place-items-center rounded-pill bg-black/40 text-white backdrop-blur-md transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer'

function markImageError(assetID: string) {
  failedImages.value = new Set(failedImages.value).add(assetID)
}

function scrollToTop() {
  scrollAreaRef.value?.scrollTo({ top: 0 })
}

const columnCount = computed(() => {
  if (!props.isExpanded) return 2
  const w = props.windowWidth
  if (w < 640) return 2
  if (w < 768) return 3
  if (w < 1024) return 4
  if (w < 1280) return 5
  if (w < 1536) return 6
  return 7
})

const galleryGridRef = ref<HTMLElement | null>(null)
const cardElements = ref<HTMLElement[]>([])
const galleryWidth = ref(0)
const galleryGap = ref(0)
const fallbackHeight = ref(96)
const measuredCards = ref(new Map<string, { width: number; height: number }>())

let measurementFrame: number | null = null
function measureGallery() {
  measurementFrame = null
  const element = galleryGridRef.value
  if (!element) return
  galleryWidth.value = element.getBoundingClientRect().width
  galleryGap.value = parseFloat(getComputedStyle(element).columnGap) || 0
  // The existing unavailable-image state has a min-h-24 (6rem) floor.
  fallbackHeight.value = 6 * (parseFloat(getComputedStyle(document.documentElement).fontSize) || 16)
  const measurements = new Map<string, { width: number; height: number }>()
  for (const card of cardElements.value) {
    const id = card.dataset.assetId
    if (!id) continue
    const { width, height } = card.getBoundingClientRect()
    measurements.set(id, { width, height })
  }
  measuredCards.value = measurements
}

// Defer geometry writes to the next frame so observing the masonry's own
// height cannot cause a ResizeObserver delivery loop.
function scheduleMeasurement() {
  if (measurementFrame === null) measurementFrame = requestAnimationFrame(measureGallery)
}
useResizeObserver(() => [galleryGridRef.value, ...cardElements.value], scheduleMeasurement)
watch([galleryGridRef, () => props.isExpanded], scheduleMeasurement, { flush: 'post' })
onBeforeUnmount(() => {
  if (measurementFrame !== null) cancelAnimationFrame(measurementFrame)
})

// Keep every keyed card under one parent: changing lanes must only change
// geometry, never remount an asset's DOM or discard its focus/local state.
// Placement retains the original round-robin masonry and per-lane heights.
const galleryLayout = computed(() => {
  const count = columnCount.value
  const gap = galleryGap.value
  const width = Math.max(0, (galleryWidth.value - gap * (count - 1)) / count)
  const bottoms = Array<number>(count).fill(0)
  const entries = props.assets.map((asset, index) => {
    const lane = index % count
    const top = bottoms[lane] ?? 0
    const imageHeight = width * ((asset.height || 1) / (asset.width || 1))
    const estimatedHeight = failedImages.value.has(asset.id)
      ? Math.max(imageHeight, fallbackHeight.value)
      : imageHeight
    const isMeasured = measuredCards.value.get(asset.id)
    const height =
      isMeasured && Math.abs(isMeasured.width - width) < 0.1 ? isMeasured.height : estimatedHeight
    bottoms[lane] = top + height + gap
    return {
      asset,
      style: {
        position: 'absolute' as const,
        top: `${top}px`,
        left: `${lane * (width + gap)}px`,
        width: `${width}px`,
      },
    }
  })
  return { entries, height: Math.max(0, ...bottoms.map((bottom) => bottom - gap)) }
})

const positionedAssets = computed(() => galleryLayout.value.entries)
const galleryHeight = computed(() => galleryLayout.value.height)

defineExpose({
  scrollToTop,
})
</script>
