<template>
  <section :class="mainContentClass" aria-label="账户管理">
    <Button
      type="button"
      text
      rounded
      class="!absolute !top-4 !right-4 !z-20 !flex !h-8 !w-8 !min-w-8 !items-center !justify-center !rounded-[var(--radius-ib-pill)] !border !border-transparent !bg-transparent !p-0 !text-ib-text-muted !shadow-none !transition !duration-[var(--duration-ib-standard)] hover:!border-ib-border-soft hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
      aria-label="关闭账户管理"
      @click="emit('close')"
    >
      <Xmark class="h-4 w-4" />
    </Button>

    <aside
      class="flex min-h-0 flex-col overflow-y-auto bg-ib-surface px-4 py-5 [scrollbar-width:none] max-[820px]:max-h-[48dvh] max-[820px]:px-4 max-[820px]:py-4"
    >
      <div class="mb-7 flex min-w-0 items-center gap-2.5 px-1" :aria-label="projectName">
        <span :class="accountBrandMarkClass">
          <SiteBrandLogo
            :src="logoURL"
            :fallback-text="fallbackText"
            :class="brandMarkClasses.logo"
          />
        </span>
        <span
          class="min-w-0 truncate text-[19px] leading-[29px] font-bold tracking-normal text-ib-text"
        >
          {{ projectName }}
        </span>
      </div>

      <nav
        class="grid min-w-0 gap-5 max-[820px]:grid-cols-3 max-[820px]:gap-3 max-[640px]:grid-cols-1"
        aria-label="账户管理导航"
      >
        <section
          v-for="group in navGroups"
          :key="group.key"
          class="min-w-0"
          :aria-labelledby="`account-nav-${group.key}`"
        >
          <p
            :id="`account-nav-${group.key}`"
            class="mb-2 px-2 text-[12px] leading-5 font-semibold tracking-normal text-ib-text-subtle"
          >
            {{ group.label }}
          </p>
          <div class="grid gap-1.5 max-[640px]:grid-cols-2">
            <Button
              v-for="item in group.items"
              :key="item.key"
              type="button"
              text
              class="!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !px-2.5 !text-[14px] !font-semibold !shadow-none focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
              :class="
                activeTab === item.key
                  ? '!bg-ib-creative-accent-soft !text-ib-creative-accent-pressed'
                  : '!bg-transparent !text-ib-text-muted hover:!bg-ib-surface-hover hover:!text-ib-text'
              "
              :aria-current="activeTab === item.key ? 'page' : undefined"
              @click="activeTab = item.key"
            >
              <component :is="item.icon" class="h-4 w-4 shrink-0" />
              <span class="truncate">{{ item.label }}</span>
            </Button>
          </div>
        </section>
      </nav>

      <div class="mt-auto pt-4">
        <Button
          type="button"
          text
          :disabled="logoutLoading"
          class="!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !bg-transparent !px-2.5 !text-[14px] !font-semibold !text-ib-danger !shadow-none !transition !duration-[var(--duration-ib-standard)] hover:!bg-ib-danger/5 hover:!text-ib-danger focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60"
          :aria-label="logoutLoading ? '正在退出登录' : '退出登录'"
          @click="handleLogout"
        >
          <LogOut class="h-4 w-4 shrink-0" />
          <span class="truncate">{{ logoutLoading ? '退出中...' : '退出登录' }}</span>
        </Button>
      </div>
    </aside>

    <main :class="contentPaneClass">
      <Transition
        mode="out-in"
        enter-active-class="transition-opacity duration-150 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition-opacity duration-100 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <KeepAlive include="UserProfilePane">
          <UserProfilePane v-if="activeTab === 'profile'" key="profile" />

          <UserBillingRecordsPane
            v-else-if="activeTab === 'billing'"
            key="billing"
            :active="activeTab === 'billing'"
          />

          <UserRewardsPane v-else-if="activeTab === 'rewards'" key="rewards" />

          <UserUsagePane
            v-else-if="activeTab === 'usage'"
            key="usage"
            :active="activeTab === 'usage'"
          />

          <UserSecurityPane v-else-if="activeTab === 'security'" key="security" />

          <AccountComingSoonPane
            v-else-if="upcomingPage"
            :key="activeTab"
            :category="upcomingPage.category"
            :title="upcomingPage.title"
            :icon="upcomingPage.icon"
          />
        </KeepAlive>
      </Transition>
    </main>
  </section>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { computed, onMounted, ref, watch, type Component } from 'vue'
import Button from 'primevue/button'
import {
  Coins,
  CreditCard,
  Gift,
  Lock,
  LogOut,
  Page,
  StatsUpSquare,
  User,
  Xmark,
} from '@iconoir/vue'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { brandMarkClasses, createBrandMarkClass } from '../../../shared/lib/brandMark'
import { useProfileFeedback } from '../useProfileFeedback'
import AccountComingSoonPane from './AccountComingSoonPane.vue'
import UserBillingRecordsPane from './UserBillingRecordsPane.vue'
import UserProfilePane from './UserProfilePane.vue'
import UserRewardsPane from './UserRewardsPane.vue'
import UserSecurityPane from './UserSecurityPane.vue'
import UserUsagePane from './UserUsagePane.vue'
import type { AccountManagementTab } from '../useAccountManagementDialogController'

type NavItem = {
  key: AccountManagementTab
  label: string
  icon: Component
}
type NavGroup = {
  key: 'account' | 'subscription' | 'benefits'
  label: string
  items: NavItem[]
}
type UpcomingTab = Extract<AccountManagementTab, 'recharge' | 'subscriptions'>
type UpcomingPage = {
  category: string
  title: string
  icon: Component
}

const props = withDefaults(defineProps<{ initialTab?: AccountManagementTab }>(), {
  initialTab: 'profile',
})
const emit = defineEmits<{
  close: []
}>()
const userStore = useUserStore()
const { fallbackText, logoURL, projectName } = useSiteBranding()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const profileFeedback = useProfileFeedback()

const activeTab = ref<AccountManagementTab>(props.initialTab)
watch(
  () => props.initialTab,
  (tab) => {
    activeTab.value = tab
  },
)
const logoutLoading = ref(false)

const navGroups = [
  {
    key: 'account',
    label: '账户设置',
    items: [
      { key: 'profile', label: '个人资料', icon: User },
      { key: 'security', label: '账号安全', icon: Lock },
    ],
  },
  {
    key: 'subscription',
    label: '订阅与套餐',
    items: [
      { key: 'recharge', label: '充值Idea', icon: Coins },
      { key: 'subscriptions', label: '订阅套餐', icon: CreditCard },
    ],
  },
  {
    key: 'benefits',
    label: '权益和账单',
    items: [
      { key: 'rewards', label: '奖励中心', icon: Gift },
      { key: 'billing', label: '账单记录', icon: Page },
      { key: 'usage', label: '用量统计', icon: StatsUpSquare },
    ],
  },
] satisfies NavGroup[]

const upcomingPages: Record<UpcomingTab, UpcomingPage> = {
  recharge: { category: '订阅与套餐', title: '充值Idea', icon: Coins },
  subscriptions: { category: '订阅与套餐', title: '订阅套餐', icon: CreditCard },
}

const upcomingPage = computed(() =>
  isUpcomingTab(activeTab.value) ? upcomingPages[activeTab.value] : null,
)

const mainContentClass =
  'relative grid h-[min(78dvh,720px)] min-h-0 grid-cols-[234px_minmax(0,1fr)] overflow-hidden bg-ib-surface-raised max-[820px]:h-[calc(100dvh-64px)] max-[820px]:grid-cols-1 max-[820px]:grid-rows-[auto_minmax(0,1fr)]'
const contentPaneClass =
  'min-h-0 overflow-x-hidden overflow-y-auto rounded-l-[var(--radius-ib-overlay)] bg-ib-surface-muted px-8 py-7 pr-14 [scrollbar-width:none] max-[820px]:rounded-l-none max-[820px]:rounded-t-[var(--radius-ib-overlay)] max-[820px]:px-4 max-[820px]:py-5 max-[820px]:pr-14'
const accountBrandMarkClass = createBrandMarkClass(brandMarkClasses.sizes.account)

function isUpcomingTab(tab: AccountManagementTab): tab is UpcomingTab {
  return tab in upcomingPages
}

async function handleLogout() {
  if (logoutLoading.value) return

  logoutLoading.value = true
  try {
    if (await logoutAndRedirect(USER_HOME_PATH)) {
      emit('close')
    } else {
      profileFeedback.logoutFailed()
    }
  } finally {
    logoutLoading.value = false
  }
}

onMounted(() => {
  void userStore.fetchUserInfo()
})
</script>
