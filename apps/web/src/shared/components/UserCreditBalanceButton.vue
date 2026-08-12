<template>
  <Button
    unstyled
    type="button"
    class="user-credit-pill"
    :class="`is-${balance.status}`"
    :data-balance-status="balance.status"
    :title="balance.ariaLabel"
    :aria-label="balance.ariaLabel"
    :aria-busy="balance.status === 'loading'"
    @pointerdown.stop
    @mousedown.stop
    @click.stop="balance.command"
  >
    <LightBulb class="user-credit-pill__icon" aria-hidden="true" />
    <span class="user-credit-pill__balance font-mono">{{ balance.displayValue }}</span>
  </Button>
</template>

<script setup lang="ts">
import { LightBulb } from '@iconoir/vue'
import Button from 'primevue/button'
import type { UserIdeaBalanceControl } from './userAccountMenu.types'

defineProps<{
  balance: UserIdeaBalanceControl
}>()
</script>

<style scoped>
.user-credit-pill {
  display: inline-flex;
  width: auto;
  min-width: 72px;
  max-width: 152px;
  height: var(--ib-page-chrome-control-size, 40px);
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 0 13px 0 11px;
  flex: none;
  overflow: hidden;
  border: 1px solid var(--user-credit-pill-border, var(--color-ib-border-soft));
  border-radius: 999px;
  background: var(--user-credit-pill-bg, var(--color-ib-surface));
  color: var(--color-ib-text);
  box-shadow: var(--shadow-ib-control);
  cursor: pointer;
  appearance: none;
  transition:
    background-color var(--motion-control-duration) var(--motion-control-easing),
    border-color var(--motion-control-duration) var(--motion-control-easing),
    color var(--motion-control-duration) var(--motion-control-easing);
}

.user-credit-pill:hover {
  border-color: var(--user-credit-pill-border-hover, var(--color-ib-border-hover));
  background: var(--color-ib-surface-hover);
}

.user-credit-pill:active {
  border-color: var(--user-credit-pill-border-active, var(--color-ib-border-pressed));
  background: var(--color-ib-surface-pressed);
}

.user-credit-pill:focus-visible {
  outline: none;
  box-shadow: var(--shadow-ib-focus);
}

.user-credit-pill__icon {
  width: 17px;
  height: 17px;
  flex: none;
  color: var(--color-ib-warning);
  stroke-width: 1.8;
}

.user-credit-pill__balance {
  min-width: 2ch;
  max-width: 12ch;
  overflow: hidden;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  line-height: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-credit-pill.is-loading .user-credit-pill__balance {
  color: var(--color-ib-text-subtle);
}

.user-credit-pill.is-error .user-credit-pill__balance {
  color: var(--color-ib-danger);
}

@media (max-width: 640px) {
  .user-credit-pill {
    min-width: 64px;
    max-width: 116px;
    gap: 5px;
    padding-inline: 9px;
  }

  .user-credit-pill__balance {
    max-width: 8ch;
    font-size: 12px;
  }
}
</style>
