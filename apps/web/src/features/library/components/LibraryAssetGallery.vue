<template>
  <div
    ref="scrollAreaRef"
    class="mt-2 h-full min-h-0 flex-1 overflow-y-auto pr-0.5 pt-1 rounded-[18px] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    :aria-busy="loading"
    @scroll="emit('scroll')"
  >
    <!-- 骨架加载状态 -->
    <div
      v-if="loading && assets.length === 0"
      class="grid h-full min-h-40 grid-cols-2 gap-2 overflow-hidden"
      aria-busy="true"
    >
      <div
        v-for="n in 6"
        :key="n"
        class="h-28 animate-pulse rounded-xl bg-muted"
      ></div>
    </div>

    <!-- 空状态提示 -->
    <div
      v-else-if="!loading && assets.length === 0 && !errorMessage"
      class="grid h-full min-h-40 place-items-center rounded-[18px] border border-dashed border-border px-5 text-center"
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
      <div class="flex items-start" :class="isExpanded ? 'gap-3' : 'gap-2'">
        <div
          v-for="(column, colIndex) in columnBuckets"
          :key="column[0]?.id ?? colIndex"
          class="flex-1 flex flex-col min-w-0"
          :class="isExpanded ? 'gap-3' : 'gap-2'"
        >
          <article
            v-for="asset in column"
            :key="asset.id"
            draggable="true"
            class="group/libcard relative overflow-hidden rounded-xl bg-card/80 backdrop-blur-md shadow-none transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-grab active:cursor-grabbing select-none"
            @dragstart="emit('asset-dragstart', $event, asset)"
          >
            <button
              type="button"
              class="block w-full cursor-zoom-in text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              :aria-label="
                t('library.actions.viewSavedReference', {
                  title: asset.title || t('library.states.savedReference'),
                })
              "
              @click="emit('open-asset', asset)"
            >
              <div
                class="relative overflow-hidden bg-muted/30 backdrop-blur-md"
                :style="{ aspectRatio: `${asset.width || 1} / ${asset.height || 1}` }"
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
                  class="grid size-6 shrink-0 place-items-center rounded-full bg-black/40 text-white backdrop-blur-md hover:bg-destructive transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
                  :aria-label="t('library.actions.removeSavedReference')"
                  :title="t('library.actions.removeSavedReference')"
                  @click="emit('remove-asset', asset)"
                >
                  <Xmark class="size-3" aria-hidden="true" />
                </button>
                <button
                  v-if="isCanvasPresentation"
                  type="button"
                  class="grid size-6 shrink-0 place-items-center rounded-full bg-black/40 text-white backdrop-blur-md hover:bg-primary hover:text-primary-foreground transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
                  :aria-label="t('library.actions.addToCanvas')"
                  :title="t('library.actions.addToCanvas')"
                  @click.stop="emit('add-to-canvas', toCanvasLibraryImportDescriptor(asset))"
                >
                  <Plus class="size-3" aria-hidden="true" />
                </button>
                <button
                  v-else
                  type="button"
                  class="grid size-6 shrink-0 place-items-center rounded-full bg-black/40 text-white backdrop-blur-md hover:bg-primary hover:text-primary-foreground transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
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
                <p class="m-0 truncate text-ib-caption font-medium text-white leading-tight">
                  {{ asset.title || t('library.states.untitledReference') }}
                </p>
                <p class="mt-0.5 mb-0 truncate text-ib-micro text-white/70">
                  {{ asset.author || asset.provider }}
                </p>
              </div>
            </div>
          </article>
        </div>
      </div>

      <!-- 加载更多按钮 -->
      <button
        v-if="hasMore"
        type="button"
        class="mx-auto my-3 block rounded-full border border-border bg-card/80 backdrop-blur-md px-3 py-1.5 text-xs font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50 cursor-pointer"
        :disabled="loading"
        @click="emit('load-more')"
      >
        {{ t('library.actions.loadMore') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Plus, Search, Xmark } from '@iconoir/vue'
import { libraryAssetKey, resolveLibraryAssetCardURL } from '../domain/validators'
import type { LibraryAssetView } from '../domain/transfer'
import {
  toCanvasLibraryImportDescriptor,
  type CanvasLibraryImportDescriptor,
} from '../domain/transfer'

const props = defineProps<{
  assets: LibraryAssetView[]
  loading: boolean
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
  'asset-dragstart': [e: DragEvent, asset: LibraryAssetView]
}>()

const { t } = useI18n()
const scrollAreaRef = ref<HTMLElement | null>(null)
const failedImages = ref(new Set<string>())

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

const columnBuckets = computed(() => {
  const count = columnCount.value
  const buckets: LibraryAssetView[][] = Array.from({ length: count }, () => [])
  const list = props.assets
  for (let i = 0; i < list.length; i++) {
    const bucket = buckets[i % count]
    const asset = list[i]
    if (!bucket || !asset) continue
    bucket.push(asset)
  }
  return buckets
})

defineExpose({
  scrollToTop,
})
</script>
