<template>
  <Button
    :as="href ? 'a' : 'button'"
    :variant="baseButtonVariant"
    :size="baseButtonSize"
    :disabled="disabled || loading"
    :loading="loading"
    :type="!href ? type : undefined"
    :href="isDisabledOrLoading ? undefined : href"
    :aria-disabled="isDisabledOrLoading ? 'true' : undefined"
    :target="target"
    :rel="rel"
    :class="computedClasses"
  >
    <slot v-if="!loading" name="icon">
      <component :is="icon" v-if="icon" class="size-4 shrink-0" />
    </slot>
    <span v-if="label">{{ label }}</span>
    <slot />
  </Button>
</template>

<script setup lang="ts">
import { computed, type Component, type HTMLAttributes } from 'vue'
import { Button, type ButtonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export type AdminButtonVariant =
  | 'default'
  | 'primary'
  | 'secondary'
  | 'destructive'
  | 'danger'
  | 'outline'
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

function resolveEffectiveVariant(): AdminButtonVariant {
  if (props.text) {
    return 'ghost'
  }
  if (props.outlined) {
    return 'outline'
  }
  if (props.severity === 'danger') {
    return 'destructive'
  }
  if (props.severity === 'secondary') {
    return 'secondary'
  }
  if (props.severity === 'success') {
    return 'success'
  }
  return props.variant ?? 'default'
}

const baseButtonVariant = computed<ButtonVariants['variant']>(() => {
  const v = resolveEffectiveVariant()
  if (v === 'destructive' || v === 'danger') return 'destructive'
  if (v === 'outline') return 'outline'
  if (v === 'secondary') return 'secondary'
  if (v === 'ghost') return 'ghost'
  if (v === 'link') return 'link'
  if (v === 'success') return 'success'
  return 'default'
})

const baseButtonSize = computed<ButtonVariants['size']>(() => {
  if (props.size === 'small' || props.size === 'sm') return 'compact'
  if (props.size === 'large' || props.size === 'lg') return 'lg'
  if (props.size === 'icon') return 'icon'
  return 'default'
})

const adminColorClasses: Record<AdminButtonVariant, string> = {
  default:
    'bg-admin-accent !text-white hover:bg-admin-accent-hover active:bg-admin-accent-pressed shadow-xs',
  primary:
    'bg-admin-accent !text-white hover:bg-admin-accent-hover active:bg-admin-accent-pressed shadow-xs',
  secondary:
    'bg-admin-surface-muted text-admin-text-1 hover:bg-admin-surface-hover active:bg-admin-surface-pressed border border-admin-border-soft',
  destructive:
    'bg-admin-danger !text-white hover:bg-admin-danger-hover active:bg-admin-danger-pressed shadow-xs',
  danger:
    'bg-admin-danger !text-white hover:bg-admin-danger-hover active:bg-admin-danger-pressed shadow-xs',
  outline:
    'border border-admin-border bg-white text-admin-text-1 hover:bg-admin-surface-hover active:bg-admin-surface-pressed shadow-xs',
  ghost:
    'text-admin-text-2 hover:bg-admin-surface-hover hover:text-admin-text-1 active:bg-admin-surface-pressed',
  link: 'text-admin-accent underline-offset-4 hover:underline',
  success: 'bg-admin-success !text-white hover:bg-admin-success/90 shadow-xs',
}

const isDisabledOrLoading = computed(() => Boolean(props.disabled || props.loading))

const computedClasses = computed(() => {
  const effVariant = resolveEffectiveVariant()
  return cn(
    'focus-visible:ring-2 focus-visible:ring-admin-accent/20 select-none cursor-pointer',
    props.rounded ? 'rounded-full' : 'rounded-lg',
    (props.size === 'small' || props.size === 'sm') && 'text-admin-meta',
    (props.size === 'large' || props.size === 'lg') && 'text-admin-title-sm',
    (!props.size || props.size === 'default') && 'text-admin-body',
    adminColorClasses[effVariant],
    props.text &&
      props.severity === 'danger' &&
      'text-admin-danger hover:bg-admin-danger-soft active:bg-admin-danger-soft/80',
    props.outlined &&
      props.severity === 'danger' &&
      'border-admin-danger/30 text-admin-danger hover:bg-admin-danger-soft',
    props.href && isDisabledOrLoading.value && 'pointer-events-none opacity-50 !cursor-not-allowed',
    props.class,
  )
})
</script>
