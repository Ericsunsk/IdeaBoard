<template>
  <section :class="mainContentClass" aria-label="账户管理">
    <Button
      type="button"
      text
      rounded
      :disabled="logoutLoading"
      class="!absolute !top-4 !right-4 !z-20 !flex !h-8 !w-8 !min-w-8 !items-center !justify-center !rounded-[var(--radius-ib-pill)] !border !border-transparent !bg-transparent !p-0 !text-ib-text-muted !shadow-none !transition-[background-color,border-color,color] !duration-[var(--motion-feedback-duration)] !ease-[var(--motion-feedback-easing)] hover:!border-ib-border-soft hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
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
              v-for="item in group.tabs"
              :key="item.key"
              type="button"
              text
              class="!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !px-2.5 !text-[14px] !font-semibold !shadow-none !transition-[background-color,color,box-shadow] !duration-[var(--motion-control-duration)] !ease-[var(--motion-control-easing)] focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
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
          class="!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !bg-transparent !px-2.5 !text-[14px] !font-semibold !text-ib-danger !shadow-none !transition-[background-color,color,opacity] !duration-[var(--motion-feedback-duration)] !ease-[var(--motion-feedback-easing)] hover:!bg-ib-danger/5 hover:!text-ib-danger focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60"
          :aria-label="logoutLoading ? '正在退出登录' : '退出登录'"
          @click="handleLogout"
        >
          <LogOut class="h-4 w-4 shrink-0" />
          <span class="truncate">{{ logoutLoading ? '退出中...' : '退出登录' }}</span>
        </Button>
      </div>
    </aside>

    <main :class="contentPaneClass">
      <Transition name="account-pane" :css="accountPaneMotionEnabled" mode="out-in">
        <KeepAlive include="UserProfilePane">
          <component
            :is="activeTabEntry.tab.pane.component"
            :key="activeTabEntry.tab.key"
            v-bind="
              activeTabEntry.tab.pane.props({
                active: activeTab === activeTabEntry.tab.key,
                group: activeTabEntry.group,
                tab: activeTabEntry.tab,
                paymentReturn,
              })
            "
          />
        </KeepAlive>
      </Transition>
    </main>
  </section>
</template>

<script setup lang="ts">
import { useUserStore } from '../../../app/stores/user'
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import { LogOut, Xmark } from '@iconoir/vue'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { brandMarkClasses, createBrandMarkClass } from '../../../shared/lib/brandMark'
import { getMotionPreference } from '../../../shared/motion/motionPolicy'
import {
  accountTabRegistry,
  defaultAccountManagementTab,
  resolveAccountTab,
  resolveAccountManagementTab,
  type AccountManagementTab,
} from '../accountTabRegistry'
import { useProfileFeedback } from '../useProfileFeedback'
import type { PaymentReturnContext } from '../paymentReturnRoute'

const props = withDefaults(
  defineProps<{
    initialTab?: AccountManagementTab
    paymentReturn?: PaymentReturnContext | null
  }>(),
  {
    initialTab: defaultAccountManagementTab,
    paymentReturn: null,
  },
)
const emit = defineEmits<{
  close: []
  'update:busy': [busy: boolean]
}>()
const paymentReturn = computed(() => props.paymentReturn)
const userStore = useUserStore()
const { fallbackText, logoURL, projectName } = useSiteBranding()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const profileFeedback = useProfileFeedback()
const { isPending: logoutLoading, runAsyncAction: runLogoutAction } = useAsyncAction()
watch(logoutLoading, (busy) => emit('update:busy', busy), { immediate: true })

const activeTab = ref<AccountManagementTab>(resolveAccountManagementTab(props.initialTab))
watch(
  () => props.initialTab,
  (tab) => {
    activeTab.value = resolveAccountManagementTab(tab)
  },
)
const navGroups = accountTabRegistry
const activeTabEntry = computed(() => resolveAccountTab(activeTab.value))

const mainContentClass =
  'relative grid h-[min(78dvh,720px)] min-h-0 grid-cols-[234px_minmax(0,1fr)] overflow-hidden bg-ib-surface-raised max-[820px]:h-[calc(100dvh-64px)] max-[820px]:grid-cols-1 max-[820px]:grid-rows-[auto_minmax(0,1fr)]'
const contentPaneClass =
  'min-h-0 overflow-x-hidden overflow-y-auto rounded-l-[var(--radius-ib-overlay)] bg-ib-surface-muted px-8 py-7 pr-14 [scrollbar-width:none] max-[820px]:rounded-l-none max-[820px]:rounded-t-[var(--radius-ib-overlay)] max-[820px]:px-4 max-[820px]:py-5 max-[820px]:pr-14'
const accountBrandMarkClass = createBrandMarkClass(brandMarkClasses.sizes.account)
const accountPaneMotionEnabled = getMotionPreference() !== 'reduce'

async function handleLogout() {
  const loggedOut = await runLogoutAction(() => logoutAndRedirect(USER_HOME_PATH), {
    feedback: {
      error: profileFeedback.logoutFailed,
    },
  })
  if (loggedOut) {
    emit('close')
  } else if (loggedOut === false) {
    profileFeedback.logoutFailed()
  }
}

onMounted(() => {
  void userStore.fetchUserInfo()
})
</script>

<style scoped>
.account-pane-enter-active {
  transition: opacity var(--motion-enter-duration) var(--motion-enter-easing);
}

.account-pane-leave-active {
  transition: opacity var(--motion-exit-duration) var(--motion-exit-easing);
}

.account-pane-enter-from,
.account-pane-leave-to {
  opacity: 0;
}

.account-pane-enter-to,
.account-pane-leave-from {
  opacity: 1;
}
</style>
