<template>
  <div class="flex w-full min-w-0 flex-col items-start gap-8 lg:flex-row xl:gap-10">
    <!-- 左侧内容区：平铺各个板块，自适应宽度纯净排版 -->
    <div class="flex min-w-0 flex-1 w-full flex-col gap-12">
      <!-- 1. 站点设置 -->
      <section id="section-branding" aria-label="站点设置" class="scroll-mt-20" tabindex="-1">
        <AdminBrandingCard />
      </section>

      <!-- 2. 客户支持 -->
      <section
        id="section-support-channels"
        aria-label="客户支持"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminSupportChannelsEditor />
      </section>

      <!-- 3. 图片库 (Unsplash & Pexels) -->
      <section id="section-image-library" aria-label="图片库" class="scroll-mt-20" tabindex="-1">
        <AdminAlert v-if="searchLoadError" severity="error" class="mb-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>{{ searchLoadError }}</span>
            <ActionButton variant="outline" @click="loadSearchProviders">
              {{ SETTINGS_MESSAGES.retry }}
            </ActionButton>
          </div>
        </AdminAlert>

        <AdminImageProvidersTable
          :configs="searchConfigs"
          :resource="searchResource"
          :loading="searchLoading"
          @change="loadSearchProviders"
        />
      </section>

      <!-- 4. 存储配置 -->
      <section
        id="section-storage-providers"
        aria-label="存储配置"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminStoragePage />
      </section>

      <!-- 5. 上传并发限制 -->
      <section
        id="section-upload-concurrency"
        aria-label="上传并发限制"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminUploadConcurrencyEditor />
      </section>

      <!-- 6. 邮件发送服务 -->
      <section
        id="section-mail-service"
        aria-label="邮件发送服务"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminMailConfigCard plain />
      </section>

      <!-- 7. 注册准入与 API 限流 -->
      <section
        id="section-registration-ratelimit"
        aria-label="注册准入与 API 限流"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminRegistrationRateLimitEditor />
      </section>

      <!-- 8. 私有地址访问策略 -->
      <section
        id="section-outbound-policy"
        aria-label="私有地址访问策略"
        class="scroll-mt-20"
        tabindex="-1"
      >
        <AdminOutboundPolicyCard plain />
      </section>
    </div>

    <!-- 右侧目录导航栏：仅在宽屏 (lg:) 展示并固定 Sticky，贴右靠齐 -->
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
import {
  Database,
  Globe,
  Headset,
  Mail,
  MediaImage,
  Server,
  ShieldCheck,
  Upload,
} from '@iconoir/vue'
import AdminBrandingCard from '../AdminBrandingCard.vue'
import AdminImageProvidersTable from '../AdminImageProvidersTable.vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminMailConfigCard from '../AdminMailConfigCard.vue'
import AdminOutboundPolicyCard from '../AdminOutboundPolicyCard.vue'
import AdminRegistrationRateLimitEditor from '../AdminRegistrationRateLimitEditor.vue'
import AdminSupportChannelsEditor from '../AdminSupportChannelsEditor.vue'
import AdminUploadConcurrencyEditor from '../AdminUploadConcurrencyEditor.vue'
import AdminStoragePage from '../../storage/pages/AdminStoragePage.vue'
import AdminSettingsOnThisPage, {
  type OnThisPageItem,
} from '../components/AdminSettingsOnThisPage.vue'
import {
  createAdminSearchProviderResource,
  type AdminSearchProviderResource,
} from '../searchProviderResource'
import { SEARCH_PROVIDERS, type SearchProviderConfig } from '../searchProviderTypes'
import { formatAdminDisplayError } from '../../shared/lib/toast'
import { SETTINGS_MESSAGES } from './messages'
import { useScrollSpy } from '../useScrollSpy'

const TOC_ITEMS: readonly OnThisPageItem[] = [
  { id: 'section-branding', title: '站点设置', icon: Globe },
  { id: 'section-support-channels', title: '客户支持', icon: Headset },
  { id: 'section-image-library', title: '图片库', icon: MediaImage },
  { id: 'section-storage-providers', title: '存储配置', icon: Database },
  { id: 'section-upload-concurrency', title: '上传并发限制', icon: Upload },
  { id: 'section-mail-service', title: '邮件服务', icon: Mail },
  { id: 'section-registration-ratelimit', title: '注册与限流', icon: ShieldCheck },
  { id: 'section-outbound-policy', title: SETTINGS_MESSAGES.outboundTitle, icon: Server },
] as const

const { activeId: activeSectionId, handleSelect } = useScrollSpy(TOC_ITEMS.map((item) => item.id))

// 图片库配置状态维护
const searchResource: AdminSearchProviderResource = createAdminSearchProviderResource()
const searchConfigs = ref<SearchProviderConfig[]>([])
const searchLoading = ref(false)
const searchLoadError = ref('')

async function loadSearchProviders() {
  searchLoading.value = true
  searchLoadError.value = ''
  try {
    const list = await searchResource.list()
    searchConfigs.value = [...list.providers].sort(
      (left, right) =>
        SEARCH_PROVIDERS.indexOf(left.provider) - SEARCH_PROVIDERS.indexOf(right.provider),
    )
  } catch (err) {
    searchLoadError.value = formatAdminDisplayError(
      err,
      SETTINGS_MESSAGES.searchProvidersLoadFailed,
    )
  } finally {
    searchLoading.value = false
  }
}

onMounted(() => {
  void loadSearchProviders()
})
</script>
