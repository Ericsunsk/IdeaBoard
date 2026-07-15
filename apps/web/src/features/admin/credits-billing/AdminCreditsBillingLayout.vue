<template>
  <div class="flex min-h-0 min-w-0 flex-col gap-4">
    <nav
      class="flex min-w-0 flex-wrap items-center gap-1 rounded-[var(--radius-ib-panel)] border border-admin-border-soft bg-admin-control-bg p-1.5"
      aria-label="Idea与计费"
    >
      <RouterLink
        v-for="item in navigationItems"
        :key="item.path"
        :to="item.path"
        class="inline-flex min-h-10 items-center gap-2 rounded-[var(--radius-ib-control)] border border-transparent px-4 text-admin-body font-[700] text-admin-text-3 no-underline transition-colors hover:bg-ib-surface-hover hover:text-admin-text-1 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-admin-accent"
        active-class="border-admin-border bg-ib-surface text-admin-text-1"
      >
        <component :is="item.icon" class="size-4" />
        <span>{{ item.label }}</span>
      </RouterLink>
    </nav>
    <RouterView />
  </div>
</template>

<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import { Coins, Gift, Page, ShieldCheck } from '@iconoir/vue'
import {
  ADMIN_CREDIT_PRICING_PATH,
  ADMIN_CREDIT_RECONCILIATION_PATH,
  ADMIN_CREDIT_WALLETS_PATH,
  ADMIN_REWARD_CAMPAIGNS_PATH,
} from '../../../shared/constants/routePaths'

const navigationItems = [
  { path: ADMIN_CREDIT_WALLETS_PATH, label: '用户Idea账户', icon: Coins },
  { path: ADMIN_CREDIT_PRICING_PATH, label: 'SKU 与价格', icon: Page },
  { path: ADMIN_CREDIT_RECONCILIATION_PATH, label: '对账报告', icon: ShieldCheck },
  { path: ADMIN_REWARD_CAMPAIGNS_PATH, label: '奖励活动', icon: Gift },
] as const
</script>
