<template>
  <div class="flex min-h-screen items-center justify-center bg-[#f7f8fb] p-6">
    <div
      class="w-[min(520px,100%)] rounded-2xl border border-[#e7e9ef] bg-white p-7 shadow-[0_12px_24px_rgba(18,30,54,0.08)]"
    >
      <h1 class="m-0 text-[22px] text-[#1f2a44]">正在加入共享画布</h1>
      <p class="mt-3.5 mb-0 leading-[1.6] text-[#5b667f]">{{ statusText }}</p>
      <button
        v-if="failed"
        type="button"
        class="mt-5 cursor-pointer rounded-[10px] border-0 bg-[#1f2a44] px-4 py-2.5 text-sm text-white hover:bg-[#162039]"
        @click="goBoards"
      >
        返回画布列表
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import boardApi from '../../shared/api/board'
import { hydrateCanvasBoardsFromServer } from '../../features/boards/lib/boardSync'

const route = useRoute()
const router = useRouter()

const loading = ref(true)
const failed = ref(false)
const errorMessage = ref('')

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
    query: {
      boardId,
      board_id: boardId,
    },
    hash: `#board_id=${encodeURIComponent(boardId)}`,
  })
}

onMounted(async () => {
  try {
    await joinSharedBoard()
  } catch (error: any) {
    failed.value = true
    errorMessage.value = String(error?.message || '加入画布失败，请稍后再试')
  } finally {
    loading.value = false
  }
})
</script>
