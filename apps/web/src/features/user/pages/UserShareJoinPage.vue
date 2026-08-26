<template>
  <div class="grid min-h-screen place-items-center bg-ib-page p-6 text-ib-text">
    <Card
      class="w-[min(520px,100%)] rounded-[var(--radius-ib-overlay)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-panel)]"
    >
      <template #content>
        <div class="flex flex-col items-start gap-4">
          <ProgressSpinner
            v-if="loading"
            class="h-8 w-8"
            stroke-width="4"
            :aria-label="t('account.shareJoin.spinnerAria')"
          />
          <h1 class="m-0 text-[22px] font-bold text-ib-text">{{ titleText }}</h1>
          <p class="m-0 text-[15px] leading-7 text-ib-text-muted">{{ statusText }}</p>
          <Button
            v-if="failed"
            :label="t('account.shareJoin.backToBoards')"
            severity="secondary"
            outlined
            class="!rounded-[var(--radius-ib-control)] !border-ib-border !bg-ib-surface !text-ib-text-muted !shadow-none hover:!border-ib-border-hover hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]"
            @click="goBoards"
          />
        </div>
      </template>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import ProgressSpinner from 'primevue/progressspinner'
import { USER_BOARDS_PATH, USER_CANVAS_PATH } from '../../../shared/constants/routePaths'
import { joinSharedCanvasBoardByToken } from '../../canvas-board/catalog/joinSharedBoardWorkflow'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const loading = ref(true)
const failed = ref(false)
const errorMessage = ref('')

const titleText = computed(() => {
  if (loading.value) return t('account.shareJoin.loadingTitle')
  if (failed.value) return t('account.shareJoin.failedTitle')
  return t('account.shareJoin.successTitle')
})

const statusText = computed(() => {
  if (loading.value) return t('account.shareJoin.loadingStatus')
  if (failed.value) return errorMessage.value || t('account.shareJoin.failedStatus')
  return t('account.shareJoin.successStatus')
})

function getTokenFromQuery(): string {
  const raw = route.query.token
  if (Array.isArray(raw)) {
    return (raw[0] || '').trim()
  }
  return String(raw || '').trim()
}

function goBoards() {
  void router.replace(USER_BOARDS_PATH)
}

async function joinSharedBoard() {
  const { boardId } = await joinSharedCanvasBoardByToken(getTokenFromQuery())
  await router.replace({
    path: USER_CANVAS_PATH,
    query: { boardId },
  })
}

onMounted(async () => {
  try {
    await joinSharedBoard()
  } catch (_error: unknown) {
    failed.value = true
  } finally {
    loading.value = false
  }
})
</script>
