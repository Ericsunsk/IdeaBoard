<template>
  <div
    class="app-scrollbar relative h-full w-full overflow-y-auto bg-board-page px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] pb-16 text-board-page-text"
  >
    <div class="ib-board-page-background ib-page-dot-bg"></div>
    <header class="relative z-[1] flex flex-wrap items-start justify-between gap-x-4">
      <StudioName />
      <UserWorkspaceNavigation />
      <UserAccountMenu
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
      />
    </header>

    <main class="relative z-[1] mx-auto mt-12 max-w-[1240px]">
      <div class="mx-auto flex max-w-[720px] flex-col items-center text-center">
        <form
          class="mb-7 flex w-full items-center gap-2 rounded-full border border-board-card-border bg-board-control-bg p-1.5"
          @submit.prevent="submitQuery"
        >
          <Search class="ml-3 size-4 shrink-0 text-board-control-icon" aria-hidden="true" />
          <InputText
            v-model="draftQuery"
            unstyled
            class="min-w-0 flex-1 bg-transparent px-2 text-[14px] text-board-control-input-text outline-none"
            placeholder="Search inspiration..."
            aria-label="搜索灵感"
            @input="queryError = ''"
          />
          <Button
            type="submit"
            label="Search"
            class="!h-9 !rounded-full !border-0 !bg-board-primary !px-5 !text-sm !font-bold !text-ib-text-inverse !shadow-none hover:!bg-board-primary-hover"
          />
        </form>
        <p v-if="queryError" class="mt-2 mb-0 text-sm font-semibold text-board-danger">
          {{ queryError }}
        </p>
      </div>

      <div class="mt-12 flex items-center justify-between gap-3">
        <div>
          <h2 class="m-0 text-[22px] font-semibold">
            {{ submittedQuery ? `Results for “${submittedQuery}”` : 'Inspiration' }}
          </h2>
          <p v-if="!hasError" class="mt-1 mb-0 text-sm text-board-page-text-muted">
            {{ statusText }}
          </p>
        </div>
      </div>

      <section
        v-if="loading && results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg"
      >
        <ProgressSpinner
          class="!h-9 !w-9 text-board-primary"
          stroke-width="4"
          aria-label="加载灵感"
        />
      </section>
      <section
        v-else-if="hasError && results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg text-center"
      >
        <div>
          <p class="m-0 font-semibold">暂时无法加载灵感</p>
          <Button
            label="Retry"
            class="!mt-4 !rounded-full !border-0 !bg-board-primary !text-ib-text-inverse"
            @click="reload"
          />
        </div>
      </section>
      <section
        v-else-if="results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg"
      >
        <p class="m-0 text-sm text-board-page-text-muted">没有找到匹配的灵感</p>
      </section>
      <div v-else class="mt-6 columns-1 gap-4 sm:columns-2 lg:columns-3 xl:columns-4">
        <article
          v-for="(result, index) in results"
          :key="`${result.provider}:${result.id}`"
          class="group mb-4 break-inside-avoid overflow-hidden rounded-2xl border border-board-card-border bg-board-card-bg shadow-sm transition-shadow hover:shadow-[var(--shadow-ib-panel)]"
        >
          <button
            type="button"
            class="block w-full cursor-zoom-in text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-board-primary"
            :aria-label="`查看 ${result.title || '灵感图片'}`"
            @click="openLightbox(result)"
          >
            <div
              class="relative overflow-hidden bg-board-loading-bg"
              :style="{ aspectRatio: `${result.width} / ${result.height}` }"
            >
              <img
                :key="`${result.provider}:${result.id}:${imageRetryCount(result.id)}`"
                :src="result.preview_url"
                :alt="result.title || '灵感图片'"
                :loading="index < 8 ? 'eager' : 'lazy'"
                class="h-full w-full object-cover transition-transform duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] group-hover:scale-[1.025]"
                @error="markImageError(result.id)"
              />
            </div>
          </button>
          <div
            v-if="failedImages.has(result.id)"
            class="grid place-items-center gap-2 bg-board-loading-bg px-4 py-5 text-center"
          >
            <span class="text-xs font-semibold text-board-page-text-muted">图片暂时无法加载</span>
            <Button
              label="重试"
              size="small"
              severity="secondary"
              outlined
              class="!rounded-full"
              @click="retryImage(result.id)"
            />
          </div>
          <div class="p-3">
            <div class="truncate text-sm font-bold text-board-page-text">
              {{ result.title || 'Untitled reference' }}
            </div>
            <div class="mt-1 truncate text-xs text-board-page-text-muted">
              {{ result.author }} · {{ result.provider }}
            </div>
          </div>
        </article>
      </div>
      <div ref="sentinel" class="h-10" aria-hidden="true"></div>
      <div v-if="loadingMore" class="flex justify-center py-5">
        <ProgressSpinner
          class="!h-7 !w-7 text-board-primary"
          stroke-width="4"
          aria-label="加载更多灵感"
        />
      </div>
      <p
        v-else-if="results.length > 0 && !hasMore"
        class="py-4 text-center text-xs text-board-page-text-muted"
      >
        已加载全部可用灵感
      </p>
    </main>

    <Dialog
      v-model:visible="lightboxVisible"
      modal
      dismissable-mask
      :show-header="false"
      :style="{ width: 'min(960px, calc(100vw - 32px))' }"
      :pt="{
        root: { class: '!overflow-hidden !rounded-2xl !border-board-card-border' },
        content: { class: '!p-0' },
      }"
    >
      <div v-if="activeResult" class="bg-board-surface-strong">
        <div
          class="flex items-center justify-between gap-3 border-b border-board-card-border px-5 py-4"
        >
          <div class="min-w-0">
            <div class="truncate text-base font-bold">
              {{ activeResult.title || 'Inspiration' }}
            </div>
            <div class="mt-1 text-xs text-board-page-text-muted">
              {{ activeResult.author }} · {{ activeResult.provider }}
            </div>
          </div>
          <Button
            text
            rounded
            aria-label="关闭预览"
            class="!size-9 !p-0"
            @click="lightboxVisible = false"
            ><template #icon><Xmark class="size-4" /></template
          ></Button>
        </div>
        <div class="grid max-h-[76vh] place-items-center bg-black/5 p-4">
          <img
            :src="activeResult.original_url || activeResult.preview_url"
            :alt="activeResult.title || '灵感图片'"
            class="max-h-[68vh] max-w-full object-contain"
          />
        </div>
        <div class="flex justify-end px-5 py-4">
          <a
            :href="activeResult.source_url"
            target="_blank"
            rel="noreferrer"
            class="text-sm font-semibold text-board-primary hover:underline"
            >查看来源</a
          >
        </div>
      </div>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import ProgressSpinner from 'primevue/progressspinner'
import { Search, Xmark } from '@iconoir/vue'
import StudioName from '../../../shared/components/StudioName.vue'
import UserAccountMenu from '../../../shared/components/UserAccountMenu.vue'
import { HOME_PATH, USER_SEARCH_PATH } from '../../../shared/constants/routePaths'
import { useUserStore } from '../../auth/session/authSession'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import UserWorkspaceNavigation from '../../user-shell/components/UserWorkspaceNavigation.vue'
import { useUserAccountControls } from '../../user-shell/useUserAccountControls'
import {
  isSearchQueryWithinLimit,
  normalizeSearchQuery,
  reportSearchImageFailure,
  searchInspiration,
  type SearchResult,
} from '../searchResource'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const accountControls = useUserAccountControls({
  navigation: { navigate: (path) => router.push(path), logout: () => logoutAndRedirect(HOME_PATH) },
})
const draftQuery = ref('')
const submittedQuery = ref('')
const queryError = ref('')
const results = ref<SearchResult[]>([])
const nextCursor = ref('')
const hasMore = ref(false)
const loading = ref(false)
const loadingMore = ref(false)
const hasError = ref(false)
const lightboxVisible = ref(false)
const activeResult = ref<SearchResult | null>(null)
const sentinel = ref<HTMLElement>()
const failedImages = ref(new Set<string>())
const imageRetryCounts = ref(new Map<string, number>())
let controller: AbortController | null = null
let observer: IntersectionObserver | null = null
const statusText = computed(() =>
  loadingMore.value
    ? '正在加载更多...'
    : submittedQuery.value
      ? `${results.value.length} 个结果`
      : '精选图片参考',
)
async function load(reset: boolean) {
  if (reset) {
    controller?.abort()
    controller = new AbortController()
    results.value = []
    nextCursor.value = ''
    hasMore.value = false
    hasError.value = false
    failedImages.value = new Set()
    imageRetryCounts.value = new Map()
    loading.value = true
  } else {
    if (loadingMore.value || !hasMore.value) return
    loadingMore.value = true
  }
  try {
    const page = await searchInspiration({
      query: submittedQuery.value,
      cursor: nextCursor.value || undefined,
      limit: 24,
      signal: controller?.signal,
    })
    results.value = reset ? page.results : [...results.value, ...page.results]
    nextCursor.value = page.next_cursor || ''
    hasMore.value = page.has_more
  } catch (error) {
    if (!(error instanceof DOMException && error.name === 'AbortError')) hasError.value = true
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}
async function submitQuery() {
  const query = normalizeSearchQuery(draftQuery.value)
  if (!isSearchQueryWithinLimit(query)) {
    queryError.value = '搜索关键词不能超过 200 个字符'
    return
  }
  queryError.value = ''
  await router.replace({ path: USER_SEARCH_PATH, query: query ? { q: query } : {} })
}
async function reload() {
  await load(true)
}
function openLightbox(result: SearchResult) {
  activeResult.value = result
  lightboxVisible.value = true
}
function markImageError(id: string) {
  if (failedImages.value.has(id)) return
  failedImages.value = new Set(failedImages.value).add(id)
  const result = results.value.find((item) => item.id === id)
  if (result) void reportSearchImageFailure(result.provider)
}
function retryImage(id: string) {
  const next = new Map(imageRetryCounts.value)
  next.set(id, (next.get(id) ?? 0) + 1)
  imageRetryCounts.value = next
  const failed = new Set(failedImages.value)
  failed.delete(id)
  failedImages.value = failed
}
function imageRetryCount(id: string) {
  return imageRetryCounts.value.get(id) ?? 0
}
function queryValue() {
  const value = route.query.q
  return Array.isArray(value) ? String(value[0] || '') : String(value || '')
}
watch(
  () => route.query.q,
  async () => {
    submittedQuery.value = normalizeSearchQuery(queryValue())
    draftQuery.value = submittedQuery.value
    queryError.value = ''
    await load(true)
  },
  { immediate: true },
)
onMounted(async () => {
  await nextTick()
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((entry) => entry.isIntersecting)) void load(false)
    },
    { rootMargin: '640px' },
  )
  if (sentinel.value) observer.observe(sentinel.value)
})
onBeforeUnmount(() => {
  controller?.abort()
  observer?.disconnect()
})
</script>
