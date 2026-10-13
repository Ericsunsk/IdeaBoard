<template>
  <div class="flex w-44 shrink-0 flex-col gap-2.5">
    <div class="px-3 text-admin-caption font-semibold text-muted-foreground select-none">
      {{ title }}
    </div>
    <nav
      class="relative flex flex-col border-l border-border/70"
      style="--toc-item-step: 2rem"
      aria-label="本页内容导航"
    >
      <!-- 滑动指示条：目录项统一单行 h-[var(--toc-item-step)]（32px），指示条位移共用同一变量，直接按 index 定位，无需测量 DOM；若将来出现可换行标题，此处需改回测量定位 -->
      <span
        aria-hidden="true"
        class="absolute top-0 left-[-1px] h-[var(--toc-item-step)] w-0.5 rounded-pill bg-primary transition-[transform,opacity] duration-[var(--motion-feedback-duration)] ease-[var(--motion-control-easing)]"
        :style="indicatorStyle"
      />
      <button
        v-for="item in items"
        :key="item.id"
        type="button"
        :aria-current="activeId === item.id ? 'true' : undefined"
        class="group relative -ml-[2px] flex h-[var(--toc-item-step)] min-h-8 items-center gap-2 px-3 py-1.5 text-left text-admin-caption transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        :class="[
          activeId === item.id
            ? 'border-l-2 border-transparent font-medium text-foreground'
            : 'border-l-2 border-transparent text-muted-foreground hover:text-foreground',
        ]"
        @click="scrollTo(item.id)"
      >
        <component
          :is="item.icon"
          v-if="item.icon"
          class="size-3.5 shrink-0 transition-colors"
          :class="
            activeId === item.id
              ? 'text-primary'
              : 'text-muted-foreground group-hover:text-foreground'
          "
          aria-hidden="true"
        />
        <span class="truncate">{{ item.title }}</span>
      </button>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'

export interface OnThisPageItem {
  id: string
  title: string
  icon?: Component
}

const props = withDefaults(
  defineProps<{
    items: readonly OnThisPageItem[]
    activeId?: string | undefined
    title?: string
  }>(),
  {
    activeId: undefined,
    title: '本页内容',
  },
)

const emit = defineEmits<{
  select: [id: string]
}>()

// 手动 offset 滚动：scrollIntoView 无法精确对齐吸顶顶栏（h-14，56px），
// 此处复刻原 scroll-mt-20（80px）的视觉间隙并多留一点呼吸空间。
const NAV_SCROLL_OFFSET = 88

// 目录项统一单行等高（--toc-item-step，nav 上定义为 2rem = 32px，满足 32px Touch Target 底线），
// 指示条位移共用同一变量；若将来出现可换行标题，此处需改回测量定位。
const activeIndex = computed(() => props.items.findIndex((item) => item.id === props.activeId))
const indicatorStyle = computed<Record<string, string>>(() => ({
  '--toc-index': `${Math.max(activeIndex.value, 0)}`,
  transform: 'translateY(calc(var(--toc-item-step) * var(--toc-index)))',
  opacity: activeIndex.value < 0 ? '0' : '1',
}))

function scrollTo(id: string) {
  // 先 emit 让高亮即时响应；页面侧 useScrollSpy 在 smooth 滚动期间锁定，避免高亮被中间板块抢改。
  emit('select', id)
  if (typeof window === 'undefined') return
  const element = document.getElementById(id)
  if (!element || typeof window.scrollTo !== 'function') return
  const reduceMotion =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const top = element.getBoundingClientRect().top + window.scrollY - NAV_SCROLL_OFFSET
  window.scrollTo({ top: Math.max(top, 0), behavior: reduceMotion ? 'auto' : 'smooth' })
  focusTargetWithoutScroll(element)
}

// 程序化聚焦仅为读屏/键盘用户通告到达位置，不抢视觉滚动（preventScroll）；
// 目标 section 的 tabindex="-1" 由页面侧负责添加，此处仅防御性调用，不可聚焦时静默跳过，绝不抛错。
function focusTargetWithoutScroll(element: Element): void {
  const target = element as HTMLElement
  if (typeof target.focus !== 'function') return
  try {
    target.focus({ preventScroll: true })
  } catch {
    // 不可聚焦元素静默跳过
  }
}
</script>
