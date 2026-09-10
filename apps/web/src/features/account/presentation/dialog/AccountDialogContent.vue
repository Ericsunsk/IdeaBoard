<template>
  <section :class="accountMainContentClass" :aria-label="t('account.dialog.title')">
    <Button
      type="button"
      text
      rounded
      autofocus
      :disabled="logoutLoading"
      :class="accountCloseButtonClass"
      :aria-label="t('account.dialog.close')"
      @click="emit('close')"
    >
      <Xmark class="h-4 w-4" />
    </Button>

    <aside
      class="flex min-h-0 flex-col overflow-y-auto bg-white px-4 py-5 [scrollbar-width:none] max-[820px]:max-h-[48dvh] max-[820px]:px-4 max-[820px]:py-4"
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
            class="mb-2 px-2 text-[12px] leading-5 font-semibold tracking-normal text-ib-text-muted"
          >
            {{ t(group.labelKey) }}
          </p>
          <div class="grid gap-1.5 max-[640px]:grid-cols-2">
            <Button
              v-for="item in group.tabs"
              :key="item.key"
              type="button"
              text
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
          text
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
import { useUserStore } from '../../../auth/session/userSession'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import { LogOut, Xmark } from '@iconoir/vue'
import SiteBrandLogo from '../../../../shared/components/SiteBrandLogo.vue'
import { useAsyncAction } from '../../../../shared/composables/useAsyncAction'
import { useSiteBranding } from '../../../../shared/composables/useSiteBranding'
import { useLogoutRedirect } from '../../../../shared/composables/useLogoutRedirect'
import { HOME_PATH } from '../../../../shared/constants/routePaths'
import { brandMarkClasses, createBrandMarkClass } from '../../../../shared/lib/brandMark'
import { getMotionPreference } from '../../../../shared/motion/motionPolicy'
import {
  accountTabRegistry,
  defaultAccountTab,
  resolveAccountTab,
  resolveAccountTabKey,
  type AccountTab,
} from '../common/tabRegistry'
import { useProfileFeedback } from '../../application/profile/useProfileFeedback'
import type { PaymentReturnContext } from '../../domain'
import {
  accountCloseButtonClass,
  accountContentPaneClass,
  accountLogoutButtonClass,
  accountMainContentClass,
  accountNavTabClass,
} from '../common/ui'

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
const { fallbackText, logoURL, projectName } = useSiteBranding()
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
