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
  /** Locale key; copy resolves in the rendering component so it never freezes at import. */
  labelKey: string
  icon: Component
  pane: AccountTabPane
}

type AccountTabGroupDefinition = {
  key: string
  labelKey: string
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
    categoryKey: group.labelKey,
    titleKey: tab.labelKey,
    icon: tab.icon,
  }),
}

export const accountTabRegistry = [
  {
    key: 'account',
    labelKey: 'account.groups.general',
    tabs: [
      {
        key: 'profile',
        labelKey: 'account.tabs.profile',
        icon: User,
        pane: { component: UserProfilePane, props: () => ({}) },
      },
      {
        key: 'security',
        labelKey: 'account.tabs.security',
        icon: Lock,
        pane: { component: UserSecurityPane, props: () => ({}) },
      },
    ],
  },
  {
    key: 'billing',
    labelKey: 'account.groups.billing',
    tabs: [
      {
        key: 'recharge',
        labelKey: 'account.tabs.topUp',
        icon: Coins,
        pane: {
          component: UserRechargePane,
          props: ({ active, paymentReturn }) => ({ active, paymentReturn }),
        },
      },
      {
        key: 'subscriptions',
        labelKey: 'account.tabs.plans',
        icon: CreditCard,
        pane: comingSoonPane,
      },
      {
        key: 'rewards',
        labelKey: 'account.tabs.rewards',
        icon: Gift,
        pane: { component: UserRewardsPane, props: () => ({}) },
      },
      {
        key: 'billing',
        labelKey: 'account.tabs.billingTransactions',
        icon: Page,
        pane: activeQueryPane(UserBillingRecordsPane),
      },
      {
        key: 'usage',
        labelKey: 'account.tabs.usage',
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
