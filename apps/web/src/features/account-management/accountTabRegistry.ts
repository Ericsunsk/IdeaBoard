import type { Component } from 'vue'
import { Coins, CreditCard, Gift, Lock, Page, StatsUpSquare, User } from '@iconoir/vue'
import AccountComingSoonPane from './components/AccountComingSoonPane.vue'
import UserBillingRecordsPane from './components/UserBillingRecordsPane.vue'
import UserProfilePane from './components/UserProfilePane.vue'
import UserRechargePane from './components/UserRechargePane.vue'
import UserRewardsPane from './components/UserRewardsPane.vue'
import UserSecurityPane from './components/UserSecurityPane.vue'
import UserUsagePane from './components/UserUsagePane.vue'
import type { PaymentReturnContext } from './paymentReturnRoute'

type AccountTabPaneContext = {
  active: boolean
  group: AccountTabGroupDefinition
  tab: AccountTabDefinition
  paymentReturn: PaymentReturnContext | null
}

type AccountTabPane = {
  component: Component
  props: (context: AccountTabPaneContext) => Record<string, unknown>
}

type AccountTabDefinition = {
  key: string
  label: string
  icon: Component
  pane: AccountTabPane
}

type AccountTabGroupDefinition = {
  key: string
  label: string
  tabs: readonly AccountTabDefinition[]
}

function activeQueryPane(component: Component): AccountTabPane {
  return {
    component,
    props: ({ active }) => ({ active }),
  }
}

const comingSoonPane: AccountTabPane = {
  component: AccountComingSoonPane,
  props: ({ group, tab }) => ({
    category: group.label,
    title: tab.label,
    icon: tab.icon,
  }),
}

export const accountTabRegistry = [
  {
    key: 'account',
    label: '账户设置',
    tabs: [
      {
        key: 'profile',
        label: '个人资料',
        icon: User,
        pane: { component: UserProfilePane, props: () => ({}) },
      },
      {
        key: 'security',
        label: '账号安全',
        icon: Lock,
        pane: { component: UserSecurityPane, props: () => ({}) },
      },
    ],
  },
  {
    key: 'subscription',
    label: '订阅与套餐',
    tabs: [
      {
        key: 'recharge',
        label: '充值积分',
        icon: Coins,
        pane: {
          component: UserRechargePane,
          props: ({ active, paymentReturn }) => ({ active, paymentReturn }),
        },
      },
      { key: 'subscriptions', label: '订阅套餐', icon: CreditCard, pane: comingSoonPane },
    ],
  },
  {
    key: 'benefits',
    label: '权益和账单',
    tabs: [
      {
        key: 'rewards',
        label: '奖励中心',
        icon: Gift,
        pane: { component: UserRewardsPane, props: () => ({}) },
      },
      {
        key: 'billing',
        label: '账单记录',
        icon: Page,
        pane: activeQueryPane(UserBillingRecordsPane),
      },
      {
        key: 'usage',
        label: '用量统计',
        icon: StatsUpSquare,
        pane: activeQueryPane(UserUsagePane),
      },
    ],
  },
] as const satisfies readonly AccountTabGroupDefinition[]

export type AccountManagementTab = (typeof accountTabRegistry)[number]['tabs'][number]['key']

export type AccountTabEntry = {
  group: AccountTabGroupDefinition
  tab: AccountTabDefinition
}

const accountTabEntries = accountTabRegistry.flatMap((group) =>
  group.tabs.map((tab) => ({ group, tab })),
)

export const defaultAccountManagementTab: AccountManagementTab = 'profile'

export function resolveAccountTab(value: string | undefined): AccountTabEntry {
  return (
    accountTabEntries.find(({ tab }) => tab.key === value) ??
    accountTabEntries.find(({ tab }) => tab.key === defaultAccountManagementTab)!
  )
}

export function resolveAccountManagementTab(value: string | undefined): AccountManagementTab {
  return resolveAccountTab(value).tab.key as AccountManagementTab
}
