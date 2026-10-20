<template>
  <div
    v-if="balance.status === 'anonymous'"
    class="page-chrome-surface inline-flex size-[var(--page-chrome-control-size,40px)] shrink-0 items-center justify-center rounded-pill select-none"
    :title="balance.ariaLabel"
    :aria-label="balance.ariaLabel"
  >
    <LightBulb class="user-credit-pill__icon" aria-hidden="true" />
  </div>
  <button
    v-else
    type="button"
    class="page-chrome-surface user-credit-pill"
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
  </button>
</template>

<script setup lang="ts">
import { LightBulb } from '@iconoir/vue'
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
  height: var(--page-chrome-control-size, 40px);
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 0 13px 0 11px;
  flex: none;
  overflow: hidden;
  border-radius: var(--radius-pill);
  color: var(--foreground);
  cursor: pointer;
  appearance: none;
  transition:
    background-color var(--motion-control-duration) var(--motion-control-easing),
    border-color var(--motion-control-duration) var(--motion-control-easing),
    color var(--motion-control-duration) var(--motion-control-easing),
    transform var(--motion-control-duration) var(--motion-control-easing);
}

.user-credit-pill:active:not(:disabled) {
  transform: scale(0.97);
}

.user-credit-pill:hover {
  border-color: var(--color-page-chrome-border-hover);
  background: var(--color-page-chrome-bg-hover);
  color: var(--color-page-chrome-color-hover);
}

.user-credit-pill:active {
  border-color: var(--color-page-chrome-border-hover);
  background: var(--color-page-chrome-bg-hover);
}

.user-credit-pill:focus-visible {
  outline: none;
  box-shadow: var(--shadow-focus-ring);
}

.user-credit-pill__icon {
  width: 17px;
  height: 17px;
  flex: none;
  color: var(--warning);
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
  color: var(--muted-foreground);
}

.user-credit-pill.is-error .user-credit-pill__balance {
  color: var(--destructive);
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
    font-size: var(--text-admin-meta);
  }
}
</style>
