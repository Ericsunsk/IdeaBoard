<template>
  <DialogContent
    v-bind="forwardedProps"
    :class="
      cn(
        'pointer-events-auto fixed left-1/2 top-1/2 z-50 grid w-full max-w-lg -translate-x-1/2 -translate-y-1/2 gap-4 border border-border bg-background p-6 shadow-lg data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 sm:rounded-lg',
        props.class,
      )
    "
    @escape-key-down="emits('escapeKeyDown', $event)"
    @pointer-down-outside="emits('pointerDownOutside', $event)"
    @focus-outside="emits('focusOutside', $event)"
    @interact-outside="emits('interactOutside', $event)"
    @open-auto-focus="emits('openAutoFocus', $event)"
    @close-auto-focus="emits('closeAutoFocus', $event)"
  >
    <slot />

    <DialogClose
      v-if="props.showClose"
      class="absolute right-4 top-4 rounded-sm opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2 disabled:pointer-events-none"
    >
      <Xmark class="h-4 w-4" />
      <span class="sr-only">{{ props.closeLabel }}</span>
    </DialogClose>
  </DialogContent>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import {
  DialogClose,
  DialogContent,
  type DialogContentEmits,
  type DialogContentProps,
} from 'reka-ui'
import { Xmark } from '@iconoir/vue'
import { cn, cleanProps } from '@/lib/utils'

const props = withDefaults(
  defineProps<
    DialogContentProps & {
      class?: HTMLAttributes['class']
      /** 关闭按钮读屏文案，默认 'Close'（shadcn 上游一致），调用方可传 i18n 文案。 */
      closeLabel?: string
      /** 是否显示默认右上角关闭按钮，默认 true。当弹窗内容自带关闭控件时可传入 false。 */
      showClose?: boolean
    }
  >(),
  { closeLabel: 'Close', showClose: true, class: undefined },
)
const emits = defineEmits<DialogContentEmits>()

const forwardedProps = computed(() => {
  const { class: _, closeLabel: _close, showClose: _show, ...rest } = props
  return cleanProps(rest)
})
</script>
