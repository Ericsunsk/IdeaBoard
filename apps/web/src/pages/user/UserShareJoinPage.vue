<template>
  <div class="share-page">
    <div class="share-card">
      <h1>正在加入共享画布</h1>
      <p>{{ statusText }}</p>
      <button v-if="failed" type="button" class="share-btn" @click="goBoards">
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

<style scoped>
.share-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: #f7f8fb;
}

.share-card {
  width: min(520px, 100%);
  background: #fff;
  border: 1px solid #e7e9ef;
  border-radius: 16px;
  padding: 28px;
  box-shadow: 0 12px 24px rgba(18, 30, 54, 0.08);
}

.share-card h1 {
  margin: 0;
  font-size: 22px;
  color: #1f2a44;
}

.share-card p {
  margin: 14px 0 0;
  color: #5b667f;
  line-height: 1.6;
}

.share-btn {
  margin-top: 20px;
  border: none;
  border-radius: 10px;
  background: #1f2a44;
  color: #fff;
  font-size: 14px;
  padding: 10px 16px;
  cursor: pointer;
}

.share-btn:hover {
  background: #162039;
}
</style>
