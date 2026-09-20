<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
      leave-active-class="transition-opacity duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div
        v-if="visible && activeResult"
        class="fixed inset-0 z-[2000] flex flex-col items-center justify-between p-4 sm:p-6 md:p-8 bg-black/65 backdrop-blur-2xl select-none"
        @click="$emit('close')"
      >
        <!-- 顶部极简悬浮信息与操作栏 -->
        <div
          class="flex w-full max-w-6xl items-center justify-between z-20 shrink-0 mb-3 px-2"
          @click.stop
        >
          <!-- 左上角：纯净悬浮文字 (Title + Author) -->
          <div class="min-w-0 pr-4">
            <h3
              class="m-0 text-white text-base sm:text-lg font-semibold tracking-tight drop-shadow-md truncate max-w-[240px] sm:max-w-md"
            >
              {{ activeResult.title || t('search.lightbox.inspiration') }}
            </h3>
            <p class="mt-1 mb-0 text-white/70 text-xs drop-shadow-md">
              {{ activeResult.author }} · {{ activeResult.provider }}
            </p>
          </div>

          <!-- 右上角：来源链接 + 极简圆形 ✕ 关闭按钮 -->
          <div class="flex items-center gap-2">
            <a
              v-if="activeResult.source_url"
              :href="activeResult.source_url"
              target="_blank"
              rel="noreferrer"
              class="h-7 px-3 rounded-full bg-white/10 text-white/90 backdrop-blur-md border border-white/15 transition-[background-color,color,border-color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-board-primary hover:text-white hover:border-transparent flex items-center justify-center text-xs font-medium no-underline cursor-pointer"
            >
              <span>{{ t('search.lightbox.viewSource') }}</span>
            </a>

            <button
              type="button"
              class="grid size-7 place-items-center rounded-full bg-white/10 text-white/90 backdrop-blur-md border border-white/15 transition-[background-color,color,border-color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-board-primary hover:text-white hover:border-transparent focus-visible:outline-none cursor-pointer"
              :aria-label="t('search.lightbox.close')"
              :title="t('search.lightbox.close')"
              @click="$emit('close')"
            >
              <Xmark class="size-3.5" aria-hidden="true" />
            </button>
          </div>
        </div>

        <!-- 中间大图展示区 (纯粹沉浸大图鉴赏) -->
        <div
          class="flex-1 w-full max-w-6xl flex items-center justify-center overflow-hidden min-h-0"
          @click.stop
        >
          <img
            :src="activeResult.original_url || activeResult.preview_url"
            :alt="activeResult.title || t('search.fallbackTitle')"
            class="max-h-[82vh] max-w-full object-contain rounded-2xl shadow-2xl border-0 ring-0 outline-none select-none"
          />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Xmark } from '@iconoir/vue'
import type { SearchResult } from '../../domain/models'

defineProps<{
  visible: boolean
  activeResult: SearchResult | null
}>()

defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
</script>
