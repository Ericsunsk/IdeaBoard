import { onMounted, onUnmounted, readonly, ref } from 'vue'

interface UseScrollSpyOptions {
  /**
   * 点击目录后的兜底解锁超时（ms）。
   * 主要解锁路径是 `scrollend` 事件；该超时仅兼容不支持 `scrollend` 的环境。
   */
  lockMs?: number
}

function canUseDom(): boolean {
  return typeof window !== 'undefined' && typeof document !== 'undefined'
}

/**
 * 目录式 scroll spy：返回当前高亮板块与点击处理。
 * observer 收窄为顶部一条细带，同一时刻最多一个板块命中；点击后锁定期间
 * 以点击项为准，避免滚动途中高亮被中间板块抢改。
 *
 * 锁定解除：优先监听 `scrollend`，`lockMs` 仅作兜底超时。
 */
export function useScrollSpy(sectionIds: readonly string[], options?: UseScrollSpyOptions) {
  const lockMs = options?.lockMs ?? 1000
  const activeId = ref<string>(sectionIds[0] ?? '')
  let observer: IntersectionObserver | null = null
  let locked = false
  let fallbackTimer: ReturnType<typeof setTimeout> | null = null
  let scrollEndHandler: (() => void) | null = null

  function unlock() {
    locked = false
    if (fallbackTimer !== null) {
      clearTimeout(fallbackTimer)
      fallbackTimer = null
    }
    if (scrollEndHandler !== null && canUseDom()) {
      try {
        document.removeEventListener('scrollend', scrollEndHandler)
      } catch {
        // 忽略：测试环境下移除监听不应抛错。
      }
    }
    scrollEndHandler = null
  }

  function lock() {
    if (fallbackTimer !== null) {
      clearTimeout(fallbackTimer)
      fallbackTimer = null
    }
    if (scrollEndHandler !== null && canUseDom()) {
      try {
        document.removeEventListener('scrollend', scrollEndHandler)
      } catch {
        // 忽略，继续重建监听。
      }
      scrollEndHandler = null
    }
    locked = true
    if (canUseDom()) {
      scrollEndHandler = () => {
        unlock()
      }
      try {
        document.addEventListener('scrollend', scrollEndHandler)
      } catch {
        scrollEndHandler = null
      }
    }
    // 兜底超时：兼容不支持 scrollend 的环境。
    fallbackTimer = setTimeout(unlock, lockMs)
  }

  function setActiveId(id: string) {
    activeId.value = id
  }

  function handleSelect(id: string) {
    // 点击即高亮；锁定滚动途中的 observer，避免中间板块抢改。
    activeId.value = id
    lock()
  }

  function handleIntersections(entries: IntersectionObserverEntry[]) {
    if (locked) return
    const visible = entries.filter((entry) => entry.isIntersecting)
    if (visible.length > 0) {
      const first = visible.sort(
        (left, right) => left.boundingClientRect.top - right.boundingClientRect.top,
      )[0]
      if (first?.target.id) {
        setActiveId(first.target.id)
      }
    }
  }

  onMounted(() => {
    if (typeof IntersectionObserver !== 'undefined') {
      observer = new IntersectionObserver(handleIntersections, {
        // 单阈值 + 细带即可，多阈值只会增加回调量与抖动。
        rootMargin: '-20% 0px -65% 0px',
        threshold: 0,
      })

      sectionIds.forEach((id) => {
        const element = document.getElementById(id)
        if (element) observer?.observe(element)
      })
    }
  })

  onUnmounted(() => {
    observer?.disconnect()
    observer = null
    unlock()
  })

  return { activeId: readonly(activeId), handleSelect }
}
