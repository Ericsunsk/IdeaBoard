<template>
  <DialogContent
    v-bind="forwardedProps"
    :class="cn(dialogContentBaseClass, 'pointer-events-auto w-full max-w-lg', props.class)"
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
      class="absolute right-4 top-4 rounded-control opacity-70 ring-offset-background transition-opacity duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none"
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
import { dialogContentBaseClass } from './dialogClasses'

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
