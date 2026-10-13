<template>
  <Button
    :variant="uiVariant"
    :size="uiSize"
    :is-disabled="props.disabled"
    :is-loading="props.loading"
    :type="props.type"
    :title="props.title"
    :aria-label="props.ariaLabel"
    :class="mergedClass"
    :style="passthroughStyle"
    v-bind="passthroughAttrs"
  >
    <template v-if="!props.loading && $slots.icon">
      <slot name="icon" />
    </template>
    <span v-if="props.label">{{ props.label }}</span>
    <slot />
  </Button>
</template>

<script setup lang="ts">
import { computed, useAttrs, type StyleValue } from 'vue'
import type { ClassValue } from 'clsx'
import { cn } from '@/lib/utils'
import { Button, type ButtonVariants } from '@/components/ui/button'
import {
  buttonChromeBaseClass,
  buttonChromeColorClasses as actionColorClasses,
  buttonChromeSizeTypography,
  resolveButtonChromeUiVariant,
  resolveButtonChromeVariant,
  type ButtonChromeSeverity as ActionButtonSeverity,
  type ButtonChromeVariant as ActionButtonVariant,
} from '@/components/ui/button'

// Shared action buttons reuse the Admin variant and severity contract.
export type { ActionButtonSeverity, ActionButtonVariant }

export type ActionButtonSize = 'default' | 'small' | 'sm' | 'large' | 'lg' | 'icon'

const props = withDefaults(
  defineProps<{
    variant?: ActionButtonVariant | undefined
    size?: ActionButtonSize | undefined
    severity?: ActionButtonSeverity | undefined
    outlined?: boolean | undefined
    label?: string | undefined
    disabled?: boolean | undefined
    loading?: boolean | undefined
    type?: 'button' | 'submit' | 'reset' | undefined
    title?: string | undefined
    ariaLabel?: string | undefined
  }>(),
  {
    variant: 'default',
    size: 'small',
    severity: undefined,
    outlined: false,
    label: undefined,
    disabled: false,
    loading: false,
    type: 'button',
    title: undefined,
    ariaLabel: undefined,
  },
)

defineOptions({ inheritAttrs: false })

const attrs = useAttrs()

const passthroughAttrs = computed(() => {
  const rest = { ...attrs }
  delete rest.class
  delete rest.style
  return rest
})

const passthroughStyle = computed(() => attrs.style as StyleValue | undefined)

function resolveEffectiveVariant(): ActionButtonVariant {
  return resolveButtonChromeVariant(props.variant ?? undefined, {
    outlined: props.outlined,
    severity: props.severity,
  })
}

const uiVariant = computed<ButtonVariants['variant']>(() => {
  return resolveButtonChromeUiVariant(resolveEffectiveVariant())
})

const uiSize = computed<NonNullable<ButtonVariants['size']>>(() => {
  if (props.size === 'small' || props.size === 'sm') return 'action'
  if (props.size === 'large' || props.size === 'lg') return 'lg'
  if (props.size === 'icon') return 'icon'
  return 'default'
})

const mergedClass = computed(() => {
  const effVariant = resolveEffectiveVariant()
  return cn(
    buttonChromeBaseClass,
    buttonChromeSizeTypography(uiSize.value),
    actionColorClasses[effVariant],
    props.outlined &&
      props.severity === 'danger' &&
      'border-destructive/30 text-destructive hover:bg-destructive/10',
    attrs.class as ClassValue,
  )
})
</script>
