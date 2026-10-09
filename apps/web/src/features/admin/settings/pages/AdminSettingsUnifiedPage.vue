<template>
  <div class="flex w-full min-w-0 flex-col items-start gap-8 lg:flex-row xl:gap-10">
    <div class="flex min-w-0 flex-1 w-full flex-col gap-12">
      <section
        id="section-branding"
        :aria-label="SETTINGS_SECTIONS.branding.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminBrandingCard />
      </section>

      <section
        id="section-support-channels"
        :aria-label="SETTINGS_SECTIONS.support.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminSupportChannelsEditor />
      </section>

      <section
        id="section-storage-providers"
        :aria-label="SETTINGS_SECTIONS.storage.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminStoragePage>
          <template #footer>
            <AdminUploadConcurrencyFooter />
          </template>
        </AdminStoragePage>
      </section>

      <section
        id="section-image-library"
        :aria-label="SETTINGS_SECTIONS.image.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminImageProvidersTable
          :configs="searchConfigs"
          :resource="searchResource"
          :loading="searchLoading"
          @change="loadSearchProviders"
        />
      </section>

      <section
        id="section-mail-service"
        :aria-label="SETTINGS_SECTIONS.mail.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminMailConfigCard plain />
      </section>

      <section
        id="section-oauth-login"
        :aria-label="SETTINGS_SECTIONS.oauth.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminOAuthLoginCard />
      </section>

      <section
        id="section-payment-settings"
        :aria-label="SETTINGS_SECTIONS.payment.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminPaymentSettingsCard />
      </section>

      <section
        id="section-registration-ratelimit"
        :aria-label="SETTINGS_SECTIONS.ratelimit.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <div class="flex flex-col gap-8">
          <AdminRegistrationRateLimitEditor />
          <AdminLoginLockoutEditor />
        </div>
      </section>

      <section
        id="section-outbound-policy"
        :aria-label="SETTINGS_SECTIONS.outbound.title"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminOutboundPolicyCard plain />
      </section>
    </div>

    <aside class="sticky top-20 hidden shrink-0 lg:block">
      <AdminSettingsOnThisPage
        :items="TOC_ITEMS"
        :active-id="activeSectionId"
        @select="handleSelect"
      />
    </aside>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { SETTINGS_SECTIONS } from '../shared/sections'
import AdminBrandingCard from '../branding/AdminBrandingCard.vue'
import AdminImageProvidersTable from '../image-library/AdminImageProvidersTable.vue'
import AdminOAuthLoginCard from '../oauth/AdminOAuthLoginCard.vue'
import AdminPaymentSettingsCard from '../payment/AdminPaymentSettingsCard.vue'
import AdminUploadConcurrencyFooter from '../upload/AdminUploadConcurrencyFooter.vue'
import AdminMailConfigCard from '../mail/AdminMailConfigCard.vue'
import AdminOutboundPolicyCard from '../outbound/AdminOutboundPolicyCard.vue'
import AdminRegistrationRateLimitEditor from '../access/AdminRegistrationRateLimitEditor.vue'
import AdminLoginLockoutEditor from '../access/AdminLoginLockoutEditor.vue'
import AdminSupportChannelsEditor from '../support-channels/AdminSupportChannelsEditor.vue'
import AdminStoragePage from '../../storage/pages/AdminStoragePage.vue'
import AdminSettingsOnThisPage, {
  type OnThisPageItem,
} from '../shared/components/AdminSettingsOnThisPage.vue'
import {
  createAdminSearchProviderResource,
  type AdminSearchProviderResource,
} from '../image-library/searchProviderResource'
import { SEARCH_PROVIDERS, type SearchProviderConfig } from '../image-library/searchProviderTypes'
import { useAdminSearchProviderFeedback } from '../shared/useSettingsFeedback'
import { useScrollSpy } from '../shared/useScrollSpy'

const TOC_ITEMS: readonly OnThisPageItem[] = [
  { id: 'section-branding', ...SETTINGS_SECTIONS.branding },
  { id: 'section-support-channels', ...SETTINGS_SECTIONS.support },
  { id: 'section-storage-providers', ...SETTINGS_SECTIONS.storage },
  { id: 'section-image-library', ...SETTINGS_SECTIONS.image },
  { id: 'section-mail-service', ...SETTINGS_SECTIONS.mail },
  { id: 'section-oauth-login', ...SETTINGS_SECTIONS.oauth },
  { id: 'section-payment-settings', ...SETTINGS_SECTIONS.payment },
  { id: 'section-registration-ratelimit', ...SETTINGS_SECTIONS.ratelimit },
  { id: 'section-outbound-policy', ...SETTINGS_SECTIONS.outbound },
] as const

const { activeId: activeSectionId, handleSelect } = useScrollSpy(TOC_ITEMS.map((item) => item.id))

const searchResource: AdminSearchProviderResource = createAdminSearchProviderResource()
const searchFeedback = useAdminSearchProviderFeedback()
const searchConfigs = ref<SearchProviderConfig[]>([])
const searchLoading = ref(false)
let searchRequestGeneration = 0

async function loadSearchProviders() {
  const generation = ++searchRequestGeneration
  searchLoading.value = true
  try {
    const list = await searchResource.list()
    if (generation !== searchRequestGeneration) return
    searchConfigs.value = [...list.providers].sort(
      (left, right) =>
        SEARCH_PROVIDERS.indexOf(left.provider) - SEARCH_PROVIDERS.indexOf(right.provider),
    )
  } catch (err) {
    if (generation !== searchRequestGeneration) return
    searchFeedback.loadFailed(err)
  } finally {
    if (generation === searchRequestGeneration) searchLoading.value = false
  }
}

onMounted(() => {
  void loadSearchProviders()
})
</script>
