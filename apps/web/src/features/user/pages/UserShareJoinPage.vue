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
import { USER_BOARDS_PATH, USER_CANVAS_PATH } from '../../../shared/constants/routePaths'
import { joinSharedCanvasBoardByToken } from '../../canvas-board/catalog/joinSharedBoardWorkflow'

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
    errorMessage.value = '加入共享画布失败，请确认链接是否有效'
  } finally {
    loading.value = false
  }
})
</script>
