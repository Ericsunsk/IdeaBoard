import type { Component } from 'vue'
import { Coins, CreditCard, Gift, Lock, Page, StatsUpSquare, User } from '@iconoir/vue'
import ComingSoonPane from './ComingSoonPane.vue'
import BillingPane from '../billing/BillingPane.vue'
import ProfilePane from '../profile/ProfilePane.vue'
import RechargePane from '../recharge/RechargePane.vue'
import RewardsPane from '../rewards/RewardsPane.vue'
import SecurityPane from '../security/SecurityPane.vue'
import UsagePane from '../usage/UsagePane.vue'
import { defaultAccountTab, resolveAccountTabKey, type AccountTab } from '../../core/tabs'
import type { PaymentReturnContext } from '../../core/return'

export { defaultAccountTab, resolveAccountTabKey, type AccountTab }

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
  component: ComingSoonPane,
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
        pane: { component: ProfilePane, props: () => ({}) },
      },
      {
        key: 'security',
        labelKey: 'account.tabs.security',
        icon: Lock,
        pane: { component: SecurityPane, props: () => ({}) },
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
          component: RechargePane,
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
        pane: { component: RewardsPane, props: () => ({}) },
      },
      {
        key: 'billing',
        labelKey: 'account.tabs.billingTransactions',
        icon: Page,
        pane: activeQueryPane(BillingPane),
      },
      {
        key: 'usage',
        labelKey: 'account.tabs.usage',
        icon: StatsUpSquare,
        pane: activeQueryPane(UsagePane),
      },
    ],
  },
] as const satisfies readonly AccountTabGroupDefinition[]

export type AccountTabEntry = {
  group: AccountTabGroupDefinition
  tab: AccountTabDefinition
}

const accountTabEntries = accountTabRegistry.flatMap((group) =>
  group.tabs.map((tab) => ({ group, tab })),
)

export function resolveAccountTab(value: string | undefined): AccountTabEntry {
  return (
    accountTabEntries.find(({ tab }) => tab.key === value) ??
    accountTabEntries.find(({ tab }) => tab.key === defaultAccountTab)!
  )
}
