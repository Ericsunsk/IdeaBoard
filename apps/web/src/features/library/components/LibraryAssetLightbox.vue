<template>
  <!-- 🌟 Demo 同款零遮挡全景高清 Lightbox 弹窗 (bg-black/65 backdrop-blur-2xl) -->
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
      leave-active-class="transition-opacity duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
      enter-from-class="opacity-0"
      leave-to-class="opacity-0"
    >
      <div
        v-if="!!asset"
        class="fixed inset-0 z-[var(--z-lightbox)] flex flex-col items-center justify-between p-4 sm:p-6 md:p-8 bg-black/65 backdrop-blur-2xl select-none"
        @click="emit('close')"
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
              {{ asset.title || t('library.states.savedReference') }}
            </h3>
            <p class="mt-1 mb-0 text-white/70 text-xs drop-shadow-md">
              {{ asset.author }} · {{ asset.provider }}
            </p>
          </div>

          <!-- 右上角：来源链接 + 极简圆形 ✕ 关闭按钮 -->
          <div class="flex items-center gap-2">
            <a
              v-if="asset.sourceUrl"
              :href="asset.sourceUrl"
              target="_blank"
              rel="noreferrer"
              :class="lightboxScrimLinkClass"
            >
              <span>{{ t('library.actions.viewSource') }}</span>
            </a>

            <button
              type="button"
              :class="lightboxScrimIconButtonClass"
              :aria-label="t('library.actions.closePreview')"
              :title="t('library.actions.closePreviewEsc')"
              @click="emit('close')"
            >
              <Xmark class="size-3.5" aria-hidden="true" />
            </button>
          </div>
        </div>

        <!-- 中间大图展示区 (彻底去除边框与遮挡，纯粹沉浸大图鉴赏) -->
        <div
          class="flex-1 w-full max-w-6xl flex items-center justify-center overflow-hidden min-h-0"
          @click.stop
        >
          <img
            :src="asset.originalUrl || asset.previewUrl"
            :alt="asset.title || t('library.states.savedReference')"
            class="max-h-[82vh] max-w-full object-contain rounded-2xl shadow-2xl border-0 ring-0 outline-none select-none"
          />
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Xmark } from '@iconoir/vue'
import {
  lightboxScrimIconButtonClass,
  lightboxScrimLinkClass,
} from '../../../shared/theme/lightboxScrim'
import type { LibraryAssetView } from '../domain/transfer'

const props = defineProps<{
  asset: LibraryAssetView | null
}>()

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.asset) {
    emit('close')
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>
