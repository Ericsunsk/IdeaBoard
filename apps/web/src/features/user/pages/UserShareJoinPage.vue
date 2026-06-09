<template>
  <div class="grid min-h-screen place-items-center bg-surface-50 p-6">
    <Card
      class="w-[min(520px,100%)] rounded-2xl border border-surface-200 bg-surface-0 shadow-[0_12px_24px_rgba(18,30,54,0.08)]"
    >
      <template #content>
        <div class="flex flex-col items-start gap-4">
          <ProgressSpinner
            v-if="loading"
            class="h-8 w-8"
            stroke-width="4"
            aria-label="正在加入共享画布"
          />
          <h1 class="m-0 text-[22px] font-bold text-surface-900">{{ titleText }}</h1>
          <p class="m-0 text-[15px] leading-7 text-surface-600">{{ statusText }}</p>
          <Button
            v-if="failed"
            label="返回画布列表"
            severity="secondary"
            outlined
            @click="goBoards"
          />
        </div>
      </template>
    </Card>
  </div>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import Card from 'primevue/card'
import ProgressSpinner from 'primevue/progressspinner'
import boardApi from '../../../shared/api/board'
import { hydrateCanvasBoardsFromServer } from '../../boards/lib/boardSync'

const route = useRoute()
const router = useRouter()

const loading = ref(true)
const failed = ref(false)
const errorMessage = ref('')

const titleText = computed(() => {
  if (loading.value) return '正在加入共享画布'
  if (failed.value) return '加入共享画布失败'
  return '加入共享画布成功'
})

const statusText = computed(() => {
  if (loading.value) return '请稍候，系统正在校验分享令牌并同步画布...'
  if (failed.value) return errorMessage.value || '加入画布失败，请检查分享链接是否有效。'
  return '加入成功，正在跳转...'
})

function getTokenFromQuery(): string {
  const raw = route.query.token
  if (Array.isArray(raw)) {
    return (raw[0] || '').trim()
  }
  return String(raw || '').trim()
}

function goBoards() {
  void router.replace('/user/boards')
}

async function joinSharedBoard() {
  const token = getTokenFromQuery()
  if (!token) {
    throw new Error('缺少分享令牌')
  }

  const res = await boardApi.joinByShareToken(token)
  const boardId = String(res.data?.board?.board_id || '').trim()
  if (!boardId) {
    throw new Error('服务端未返回可用画布 ID')
  }

  await hydrateCanvasBoardsFromServer()
  await router.replace({
    path: '/user/workspace',
    query: { boardId },
  })
}

onMounted(async () => {
  try {
    await joinSharedBoard()
  } catch (error: unknown) {
    failed.value = true
    errorMessage.value = error instanceof Error ? error.message : '加入画布失败，请稍后再试'
  } finally {
    loading.value = false
  }
})
</script>
