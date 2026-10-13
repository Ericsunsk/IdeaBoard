<template>
  <Button
    :as="href ? 'a' : 'button'"
    :variant="baseButtonVariant"
    :size="props.size"
    :is-disabled="disabled || loading"
    :is-loading="loading"
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
import { Button } from '@/components/ui/button'
import {
  buttonChromeBaseClass,
  buttonChromeColorClasses as adminColorClasses,
  buttonChromeSizeTypography,
  resolveButtonChromeVariant,
  type ButtonChromeSeverity,
  type ButtonChromeVariant,
} from '@/components/ui/button'
import { cn } from '@/lib/utils'

export type AdminButtonVariant = ButtonChromeVariant

export type AdminButtonSize = 'default' | 'sm' | 'lg' | 'icon'

const props = withDefaults(
  defineProps<{
    label?: string | undefined
    variant?: AdminButtonVariant | undefined
    severity?: ButtonChromeSeverity | undefined
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
  return resolveButtonChromeVariant(props.variant ?? undefined, {
    outlined: props.outlined,
    severity: props.severity,
    text: props.text,
  })
}

const baseButtonVariant = computed(resolveEffectiveVariant)

const isDisabledOrLoading = computed(() => Boolean(props.disabled || props.loading))

const computedClasses = computed(() => {
  return cn(
    buttonChromeBaseClass,
    props.rounded && 'rounded-pill',
    buttonChromeSizeTypography(props.size),
    adminColorClasses[baseButtonVariant.value],
    props.text &&
      props.severity === 'danger' &&
      'text-destructive hover:bg-destructive/10 active:bg-destructive/20',
    props.outlined &&
      props.severity === 'danger' &&
      'border-destructive/30 text-destructive hover:bg-destructive/10',
    props.href && isDisabledOrLoading.value && 'pointer-events-none opacity-50 !cursor-not-allowed',
    props.class,
  )
})
</script>
