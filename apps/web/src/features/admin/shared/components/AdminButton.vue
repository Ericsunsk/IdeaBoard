<template>
  <component
    :is="href ? 'a' : 'button'"
    :class="computedClasses"
    v-bind="elementAttrs"
    :aria-busy="loading || undefined"
  >
    <AdminSpinner v-if="loading" class="size-4 shrink-0" aria-label="加载中" />
    <slot v-else name="icon">
      <component :is="icon" v-if="icon" class="size-4 shrink-0" />
    </slot>
    <span v-if="label">{{ label }}</span>
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed, type Component, type HTMLAttributes } from 'vue'
import AdminSpinner from './AdminSpinner.vue'
import { cn } from '../lib/classNames'

export type AdminButtonVariant =
  | 'default'
  | 'primary'
  | 'destructive'
  | 'danger'
  | 'outline'
  | 'secondary'
  | 'ghost'
  | 'link'
  | 'success'

export type AdminButtonSize = 'default' | 'small' | 'sm' | 'large' | 'lg' | 'icon'

const props = withDefaults(
  defineProps<{
    label?: string | undefined
    variant?: AdminButtonVariant | undefined
    severity?: ('primary' | 'secondary' | 'danger' | 'success' | 'info' | 'warn') | undefined
    size?: AdminButtonSize | undefined
    class?: HTMLAttributes['class'] | undefined
    disabled?: boolean | undefined
    loading?: boolean | undefined
    type?: ('button' | 'submit' | 'reset') | undefined
    text?: boolean | undefined
    outlined?: boolean | undefined
    rounded?: boolean | undefined
    icon?: Component | null | undefined
    href?: string | undefined
    target?: string | undefined
    rel?: string | undefined
  }>(),
  {
    label: undefined,
    variant: 'default',
    severity: undefined,
    size: 'default',
    class: undefined,
    disabled: false,
    loading: false,
    type: 'button',
    text: false,
    outlined: false,
    rounded: false,
    icon: null,
    href: undefined,
    target: undefined,
    rel: undefined,
  },
)

type ButtonStyleInput = {
  text: boolean
  outlined: boolean
  severity: 'primary' | 'secondary' | 'danger' | 'success' | 'info' | 'warn' | undefined
  variant: AdminButtonVariant | undefined
}

function resolveVariantClass(input: ButtonStyleInput): string {
  const { text, outlined, severity, variant } = input
  if (text) {
    if (severity === 'danger') {
      return 'text-admin-danger hover:bg-admin-danger-soft active:bg-admin-danger-soft/80'
    }
    return 'text-admin-text-2 hover:bg-admin-surface-hover hover:text-admin-text-1'
  }
  if (outlined) {
    if (severity === 'danger') {
      return 'border border-admin-danger/30 text-admin-danger bg-white hover:bg-admin-danger-soft'
    }
    return 'border border-admin-border bg-white text-admin-text-1 hover:bg-admin-surface-hover'
  }
  if (severity === 'danger' || variant === 'destructive' || variant === 'danger') {
    return 'bg-admin-danger !text-white hover:bg-admin-danger-hover active:bg-admin-danger-pressed shadow-xs'
  }
  if (severity === 'success' || variant === 'success') {
    return 'bg-admin-success !text-white hover:bg-admin-success/90 shadow-xs'
  }
  if (severity === 'secondary' || variant === 'secondary') {
    return 'bg-admin-surface-muted text-admin-text-1 hover:bg-admin-surface-hover active:bg-admin-surface-pressed border border-admin-border-soft'
  }
  if (variant === 'outline') {
    return 'border border-admin-border bg-white text-admin-text-1 hover:bg-admin-surface-hover active:bg-admin-surface-pressed shadow-xs'
  }
  if (variant === 'ghost') {
    return 'text-admin-text-2 hover:bg-admin-surface-hover hover:text-admin-text-1 active:bg-admin-surface-pressed'
  }
  if (variant === 'link') {
    return 'text-admin-accent underline-offset-4 hover:underline'
  }
  return 'bg-admin-accent !text-white hover:bg-admin-accent-hover active:bg-admin-accent-pressed shadow-xs'
}

function resolveSizeClass(size: AdminButtonSize | undefined): string {
  if (size === 'small' || size === 'sm') return 'h-8 px-3 text-admin-meta'
  if (size === 'large' || size === 'lg') return 'h-10 px-6 text-admin-title-sm'
  if (size === 'icon') return 'size-9 p-0'
  return 'h-9 px-4 py-2 text-admin-body'
}

const computedClasses = computed(() => {
  const variantClass = resolveVariantClass({
    text: props.text,
    outlined: props.outlined,
    severity: props.severity,
    variant: props.variant,
  })
  const sizeClass = resolveSizeClass(props.size)
  const radiusClass = props.rounded ? 'rounded-full' : 'rounded-lg'

  return cn(
    'inline-flex items-center justify-center gap-2 whitespace-nowrap font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20 disabled:pointer-events-none disabled:opacity-50 select-none cursor-pointer',
    sizeClass,
    variantClass,
    radiusClass,
    props.class,
  )
})

// 有 href 即渲染 anchor（旧 PrimeVue Button as="a" 的替代），无 href 保持 button。
const elementAttrs = computed(() => {
  if (props.href) {
    return { href: props.href, target: props.target, rel: props.rel }
  }
  return { disabled: props.disabled || props.loading, type: props.type }
})
</script>
