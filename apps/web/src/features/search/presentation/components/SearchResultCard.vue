<template>
  <article
    :key="`${result.provider}:${result.id}`"
    :style="cardStyle"
    draggable="true"
    class="group relative overflow-hidden rounded-[var(--search-card-radius)] bg-card/85 cursor-grab active:cursor-grabbing select-none"
    @dragstart="$emit('cardDragStart', $event)"
    @dragend="$emit('cardDragEnd')"
  >
    <button
      type="button"
      class="block w-full cursor-zoom-in text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      :aria-label="t('search.viewResult', { title: result.title || t('search.fallbackTitle') })"
      @click="$emit('openLightbox')"
    >
      <div
        class="relative overflow-hidden bg-muted/30"
        :style="{ aspectRatio: `${result.width} / ${result.height}` }"
      >
        <img
          :key="`${result.provider}:${result.id}:${imageRetryCount}`"
          :src="result.preview_url || result.thumbnail_url"
          :srcset="`${result.thumbnail_url} 400w, ${result.preview_url} 1200w`"
          sizes="(min-width: 1280px) 20vw, (min-width: 1024px) 25vw, (min-width: 640px) 33vw, 100vw"
          :alt="result.title || t('search.fallbackTitle')"
          :loading="index < 4 ? 'eager' : 'lazy'"
          :fetchpriority="index < 2 ? 'high' : 'auto'"
          class="block h-full w-full object-cover select-none pointer-events-none"
          @error="$emit('markImageError')"
        />
      </div>
    </button>

    <!-- 🌟 Hover 渐变半透操作浮层 (顶栏左上角多选对勾 + 右上角搜图 + 底栏标题/作者 + 存入个人素材库) -->
    <div
      class="pointer-events-none absolute inset-0 z-20 flex flex-col justify-between p-3.5 transition-opacity duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] bg-gradient-to-t from-black/80 via-transparent to-black/20 group-hover:opacity-100 group-focus-within:opacity-100"
      :class="isSelected ? 'opacity-100' : 'opacity-0'"
    >
      <!-- 顶栏：左上角极简多选对勾 + 右上角搜图参考圆纽 -->
      <div class="pointer-events-auto flex items-center justify-between">
        <button
          type="button"
          class="grid size-[var(--search-overlay-btn-size)] place-items-center rounded-full transition-colors duration-[var(--motion-control-duration)] cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          :class="
            isSelected
              ? '!bg-primary text-primary-foreground shadow-xs'
              : 'bg-[var(--search-overlay-btn-bg)] text-white/80 hover:bg-[var(--search-overlay-btn-bg-hover)] hover:text-white backdrop-blur-[var(--ib-glass-blur)]'
          "
          :title="
            isSelected ? t('search.tooltips.deselectImage') : t('search.tooltips.selectImage')
          "
          :aria-label="
            isSelected ? t('search.tooltips.deselectImage') : t('search.tooltips.selectImage')
          "
          @click.stop="$emit('toggleSelect')"
        >
          <Check class="size-3.5 stroke-[2.5]" aria-hidden="true" />
        </button>

        <button
          type="button"
          class="grid size-[var(--search-overlay-btn-size)] place-items-center rounded-full bg-[var(--search-overlay-btn-bg)] text-white backdrop-blur-[var(--ib-glass-blur)] transition-colors hover:bg-primary hover:text-primary-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-pointer"
          :class="{
            '!bg-primary !text-primary-foreground shadow-xs': isUsedAsReference,
          }"
          :title="t('search.tooltips.useAsReference')"
          :aria-label="t('search.tooltips.useAsReference')"
          @click.stop="$emit('useAsReference')"
        >
          <Search class="size-3.5" aria-hidden="true" />
        </button>
      </div>

      <!-- 底部信息与存入素材库按钮 -->
      <div class="pointer-events-auto flex items-end justify-between gap-3">
        <div class="min-w-0 flex-1">
          <p class="m-0 truncate text-xs font-medium text-white leading-tight">
            {{ result.title || t('search.lightbox.untitledReference') }}
          </p>
          <p class="mt-1 mb-0 truncate text-ib-caption text-white/70 leading-tight">
            {{
              t('search.attribution.byAuthorProvider', {
                author: result.author,
                provider: result.provider,
              })
            }}
          </p>
        </div>

        <button
          type="button"
          class="grid size-[var(--search-overlay-btn-size)] place-items-center rounded-full shadow-md transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60 cursor-pointer"
          :class="
            isSaved
              ? 'bg-emerald-700 text-white hover:bg-emerald-700/90'
              : 'bg-primary text-primary-foreground hover:bg-primary/90 shadow-primary/30'
          "
          :aria-label="
            isSaved ? t('search.tooltips.removeFromLibrary') : t('search.tooltips.saveToLibrary')
          "
          :title="
            isSaved ? t('search.tooltips.removeFromLibrary') : t('search.tooltips.saveToLibrary')
          "
          @click.stop="$emit('toggleSave', $event)"
        >
          <BookmarkSolid v-if="isSaved" class="size-3.5" />
          <Plus v-else class="size-3.5 stroke-[2.5]" />
        </button>
      </div>
    </div>

    <div
      v-if="hasImageError"
      class="grid place-items-center gap-2 bg-muted/30 px-4 py-5 text-center"
    >
      <span class="text-xs font-semibold text-muted-foreground">{{
        t('common.errors.imageLoadFailed')
      }}</span>
      <Button variant="outline" size="sm" class="!rounded-full" @click="$emit('retryImage')">
        {{ t('common.actions.retry') }}
      </Button>
    </div>
  </article>
</template>

<script setup lang="ts">
import type { CSSProperties } from 'vue'
import { useI18n } from 'vue-i18n'
import { BookmarkSolid, Check, Plus, Search } from '@iconoir/vue'
import { Button } from '@/components/ui/button'
import type { SearchResult } from '../../domain/models'

defineProps<{
  result: SearchResult
  index: number
  cardStyle?: CSSProperties | undefined
  isSelected: boolean
  isUsedAsReference: boolean
  isSaved: boolean
  hasImageError: boolean
  imageRetryCount: number
}>()

defineEmits<{
  (e: 'cardDragStart', event: DragEvent): void
  (e: 'cardDragEnd'): void
  (e: 'openLightbox'): void
  (e: 'markImageError'): void
  (e: 'toggleSelect'): void
  (e: 'useAsReference'): void
  (e: 'toggleSave', event: MouseEvent): void
  (e: 'retryImage'): void
}>()

const { t } = useI18n()
</script>
