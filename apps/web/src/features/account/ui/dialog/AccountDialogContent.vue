<template>
  <section :class="accountMainContentClass" :aria-label="t('account.dialog.title')">
    <Button
      type="button"
      variant="ghost"
      autofocus
      :disabled="logoutLoading"
      :class="accountCloseButtonClass"
      :aria-label="t('account.dialog.close')"
      @click="emit('close')"
    >
      <Xmark class="h-4 w-4" />
    </Button>

    <aside
      class="flex min-h-0 max-h-[48dvh] flex-col overflow-y-auto bg-card px-4 py-4 [scrollbar-width:none] md:max-h-none md:py-5"
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
          class="min-w-0 truncate text-[19px] leading-[29px] font-bold tracking-normal text-foreground"
        >
          {{ projectName }}
        </span>
      </div>

      <nav
        class="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-3 md:grid-cols-1 md:gap-5"
        :aria-label="t('account.dialog.navigation')"
      >
        <section
          v-for="group in navGroups"
          :key="group.key"
          class="min-w-0"
          :aria-labelledby="`account-nav-${group.key}`"
        >
          <p
            :id="`account-nav-${group.key}`"
            class="mb-2 px-2 text-admin-meta leading-5 font-semibold tracking-normal text-muted-foreground"
          >
            {{ t(group.labelKey) }}
          </p>
          <div class="grid grid-cols-2 gap-1.5 sm:grid-cols-1">
            <Button
              v-for="item in group.tabs"
              :key="item.key"
              type="button"
              variant="ghost"
              :class="accountNavTabClass(activeTab === item.key)"
              :aria-current="activeTab === item.key ? 'page' : undefined"
              @click="activeTab = item.key"
            >
              <component :is="item.icon" class="h-4 w-4 shrink-0" />
              <span class="truncate">{{ t(item.labelKey) }}</span>
            </Button>
          </div>
        </section>
      </nav>

      <div class="mt-auto pt-4">
        <Button
          type="button"
          variant="ghost"
          :disabled="logoutLoading"
          :class="accountLogoutButtonClass"
          :aria-label="logoutLoading ? t('account.signingOut') : t('account.signOut')"
          @click="handleLogout"
        >
          <LogOut class="h-4 w-4 shrink-0" />
          <span class="truncate">{{
            logoutLoading ? t('account.signingOut') : t('account.signOut')
          }}</span>
        </Button>
      </div>
    </aside>

    <main :class="accountContentPaneClass">
      <Transition name="account-pane" :css="accountPaneMotionEnabled" mode="out-in">
        <KeepAlive include="ProfilePane">
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
import { useUserStore } from '../../../auth'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { LogOut, Xmark } from '@iconoir/vue'
import SiteBrandLogo from '../../../../core/components/SiteBrandLogo.vue'
import { useAsyncAction } from '../../../../core/composables/useAsyncAction'
import { useSiteBranding } from '../../../../core/composables/useSiteBranding'
import { useLogoutRedirect } from '../../../../core/composables/useLogoutRedirect'
import { HOME_PATH } from '../../../workspace/routePaths'
import { brandMarkClasses, createBrandMarkClass } from '../../../../core/lib/brandMark'
import { getMotionPreference } from '../../../../core/motion/motionPolicy'
import {
  accountTabRegistry,
  defaultAccountTab,
  resolveAccountTab,
  resolveAccountTabKey,
  type AccountTab,
} from '../shared/tabRegistry'
import { useProfileFeedback } from '../../session/profile/useProfileFeedback'
import type { PaymentReturnContext } from '../../core'
import {
  accountCloseButtonClass,
  accountContentPaneClass,
  accountLogoutButtonClass,
  accountMainContentClass,
  accountNavTabClass,
} from '../shared/ui'

const props = withDefaults(
  defineProps<{
    initialTab?: AccountTab
    paymentReturn?: PaymentReturnContext | null
  }>(),
  {
    initialTab: defaultAccountTab,
    paymentReturn: null,
  },
)
const emit = defineEmits<{
  close: []
  'update:busy': [busy: boolean]
}>()
const { t } = useI18n()
const paymentReturn = computed(() => props.paymentReturn)
const userStore = useUserStore()
const { fallbackText, logoURL, projectName } = useSiteBranding({
  fallback: t('account.branding.loadFailed'),
})
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const profileFeedback = useProfileFeedback()
const { isPending: logoutLoading, runAsyncAction: runLogoutAction } = useAsyncAction()
watch(logoutLoading, (busy) => emit('update:busy', busy), { immediate: true })

const activeTab = ref<AccountTab>(resolveAccountTabKey(props.initialTab))
watch(
  () => props.initialTab,
  (tab) => {
    activeTab.value = resolveAccountTabKey(tab)
  },
)
const navGroups = accountTabRegistry
const activeTabEntry = computed(() => resolveAccountTab(activeTab.value))

const accountBrandMarkClass = createBrandMarkClass(brandMarkClasses.sizes.account)
const accountPaneMotionEnabled = getMotionPreference() !== 'reduce'

async function handleLogout() {
  const loggedOut = await runLogoutAction(() => logoutAndRedirect(HOME_PATH), {
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
