<template>
  <div class="grid min-h-screen place-items-center bg-ib-page p-6 text-ib-text">
    <Card
      :elevated="false"
      class="w-[min(520px,100%)] p-6 rounded-ib-overlay border border-ib-border-soft bg-ib-surface"
    >
      <div class="flex flex-col items-start gap-4">
        <Spinner
          v-if="loading"
          class="h-8 w-8"
          stroke-width="4"
          :label="t('account.shareJoin.spinnerAria')"
        />
        <h1 class="m-0 text-ib-modal-title-mobile font-bold text-ib-text">{{ titleText }}</h1>
        <p class="m-0 text-ib-title-sm leading-7 text-ib-text-muted">{{ statusText }}</p>
        <Button v-if="failed" variant="outline" @click="goBoards">
          {{ t('account.shareJoin.backToBoards') }}
        </Button>
      </div>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { USER_BOARDS_PATH, USER_CANVAS_PATH } from '../../../shared/constants/routePaths'
import { stringParam } from '../../../shared/router/queryParams'
import { joinSharedBoardByToken } from '../../board'

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
  return (stringParam(route.query.token) ?? '').trim()
}

function goBoards() {
  void router.replace(USER_BOARDS_PATH)
}

async function joinSharedBoard() {
  const { boardId } = await joinSharedBoardByToken(getTokenFromQuery())
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
