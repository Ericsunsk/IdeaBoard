import { nextTick, onMounted, watch, type Ref, type WatchSource } from 'vue'

type AutoResizeTextareaOptions = {
  watchSource?: WatchSource<unknown> | WatchSource<unknown>[]
  onResize?: (metrics: { naturalHeight: number; singleLineHeight: number }) => void
}

function resolveSingleLineHeight(input: HTMLTextAreaElement): number {
  const style = window.getComputedStyle(input)
  const lineHeight = Number.parseFloat(style.lineHeight)
  const paddingTop = Number.parseFloat(style.paddingTop)
  const paddingBottom = Number.parseFloat(style.paddingBottom)
  return (
    (Number.isFinite(lineHeight) ? lineHeight : 18) +
    (Number.isFinite(paddingTop) ? paddingTop : 0) +
    (Number.isFinite(paddingBottom) ? paddingBottom : 0)
  )
}

export function useAutoResizeTextarea(
  inputRef: Ref<HTMLTextAreaElement | null>,
  options: AutoResizeTextareaOptions = {},
) {
  function resize() {
    const input = inputRef.value
    if (!input) return
    const singleLineHeight = resolveSingleLineHeight(input)
    const previousMinHeight = input.style.minHeight
    input.style.minHeight = '0'
    input.style.height = 'auto'
    const naturalHeight = input.scrollHeight
    input.style.minHeight = previousMinHeight
    input.style.height = `${naturalHeight}px`
    input.style.overflowY = 'hidden'
    options.onResize?.({ naturalHeight, singleLineHeight })
  }

  function resizeAfterRender() {
    void nextTick(resize)
  }

  onMounted(resizeAfterRender)

  if (options.watchSource) {
    watch(options.watchSource, resizeAfterRender)
  }

  return {
    resize,
    resizeAfterRender,
  }
}
