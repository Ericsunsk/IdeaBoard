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
        ref="dialogRef"
        class="fixed inset-0 z-[var(--z-lightbox)] flex flex-col items-center justify-between p-4 sm:p-6 md:p-8 bg-black/65 backdrop-blur-2xl select-none"
        role="dialog"
        aria-modal="true"
        aria-labelledby="search-lightbox-title"
        tabindex="-1"
        @click="$emit('close')"
        @keydown="handleKeydown"
      >
        <!-- 顶部极简悬浮信息与操作栏 -->
        <div
          class="flex w-full max-w-6xl items-center justify-between z-20 shrink-0 mb-3 px-2"
          @click.stop
        >
          <!-- 左上角：纯净悬浮文字 (Title + Author) -->
          <div class="min-w-0 pr-4">
            <h3
              id="search-lightbox-title"
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
              :class="lightboxScrimLinkClass"
            >
              <span>{{ t('search.lightbox.viewSource') }}</span>
            </a>

            <button
              ref="closeButtonRef"
              type="button"
              :class="lightboxScrimIconButtonClass"
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
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Xmark } from '@iconoir/vue'
import {
  lightboxScrimIconButtonClass,
  lightboxScrimLinkClass,
} from '../../../../theme/lightboxScrim'
import type { SearchResult } from '../../core/models'

const props = defineProps<{
  visible: boolean
  activeResult: SearchResult | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const dialogRef = ref<HTMLElement | null>(null)
const closeButtonRef = ref<HTMLButtonElement | null>(null)
const isOpen = computed(() => props.visible && props.activeResult !== null)
let wasOpen = false
let focusReturnTarget: HTMLElement | null = null

watch(
  isOpen,
  (open) => {
    if (open && !wasOpen) {
      const activeElement = document.activeElement
      focusReturnTarget =
        activeElement instanceof HTMLElement && activeElement !== document.body
          ? activeElement
          : null
      void nextTick(() => closeButtonRef.value?.focus())
    }
    if (!open && wasOpen) {
      const target = focusReturnTarget
      focusReturnTarget = null
      if (target?.isConnected) void nextTick(() => target.focus())
    }
    wasOpen = open
  },
  { immediate: true },
)

function requestClose() {
  emit('close')
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    requestClose()
    return
  }
  if (event.key !== 'Tab') return

  const focusable = Array.from(
    dialogRef.value?.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable="true"]',
    ) ?? [],
  )
  if (!focusable.length) {
    event.preventDefault()
    dialogRef.value?.focus()
    return
  }

  const currentIndex = focusable.indexOf(document.activeElement as HTMLElement)
  const nextIndex = event.shiftKey
    ? currentIndex <= 0
      ? focusable.length - 1
      : currentIndex - 1
    : currentIndex === focusable.length - 1
      ? 0
      : currentIndex + 1
  event.preventDefault()
  focusable[nextIndex]?.focus()
}
</script>
