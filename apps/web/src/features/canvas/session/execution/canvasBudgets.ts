/**
 * Canvas 超时/重试预算表（冻结数值，只收敛不断调）。
 *
 * 数值零变更：本模块是 `pendingImageWorkflowRunner.ts` 与
 * `pending-execution/core.ts` 散落预算常量的唯一事实来源，数值与收敛前
 * 完全一致。调预算必须同步更新后端
 * `apps/api/internal/platform/riverpolicy` 预算表注释与断言。
 *
 * | 层 | 常量 | 预算 | 说明 |
 * | --- | --- | --- | --- |
 * | Gateway 单次调用 | `GatewayDefaultTimeout`（后端 `runtime/ports`） | 300s | 嵌套在 worker 360s 执行预算内（ADR-0010 rule 4） |
 * | Worker 单次执行 | `WorkspaceExecutionBudget()`（后端 `riverpolicy`） | 360s（`min(360s, drawing)`） | 慢任务上限；provider 发送超限收敛为可重试超时 |
 * | River 作业外层 | `RiverJobTimeoutDrawing`（后端 `riverpolicy`） | 17min（900s `MaxTimeoutSeconds` + 2min 开销） | 作业全局 bound + 未注册 worker 的 client fallback |
 * | 前端 quote/submit 共用重试 | `SHARED_TASK_REQUEST_ATTEMPTS` | 2 次尝试 | assert 与 submit 各最多 2 次，仅可重试错误重试 |
 * | 前端后台续轮询 | `BACKGROUND_CONTINUATION_MAX_ATTEMPTS` + 退避 `INITIAL→MAX` | 8 轮，延迟 5s 起指数退避、60s 封顶，共享 315s 后台 deadline；6min 前台之后续跑，前后台硬上限 675s < River 17min（I/O 计入预算） | 终态即物化，耗尽仍 pending 以 progressTimeout 收尾 |
 * | 前端 Board 落子有界重试 | `PENDING_IMAGE_MATERIALIZATION_ATTEMPTS` | 3 次 | 图写瞬态原地重试；契约违例不消耗预算直接失败 |
 */

export const SHARED_TASK_REQUEST_ATTEMPTS = 2

/**
 * 长任务后台续轮询（#P1-2）：6min waiter 超时后不断 lease，后台以指数
 * 退避（5s 起，60s 封顶）继续 resolve，最多 8 轮；所有请求和退避
 * 共享额外 315s 的 deadline；终态即物化，硬失败走常规失败面，耗尽仍 pending 则以
 * progressTimeout 提示收尾。
 */
export const BACKGROUND_CONTINUATION_MAX_ATTEMPTS = 8
export const BACKGROUND_CONTINUATION_INITIAL_DELAY_MS = 5_000
export const BACKGROUND_CONTINUATION_MAX_DELAY_MS = 60_000

/**
 * Board 落子有界重试（#P1-5）：图写失败（revision 冲突等瞬态）在 facts
 * 仍 active 时原地重试，最多 3 次；契约违例是确定性实现错误，不重试
 * 直接进入失败面。
 */
export const PENDING_IMAGE_MATERIALIZATION_ATTEMPTS = 3

/** 后台续第 `attempt` 轮（0-based）前的等待时长，与收敛前公式一致。 */
export function backgroundContinuationDelayMs(attempt: number): number {
  return Math.min(
    BACKGROUND_CONTINUATION_INITIAL_DELAY_MS * 2 ** attempt,
    BACKGROUND_CONTINUATION_MAX_DELAY_MS,
  )
}

/** 8 轮后台续退避总时长（ms）：5+10+20+40+60*4 = 315s。 */
export function backgroundContinuationTotalMs(): number {
  let total = 0
  for (let attempt = 0; attempt < BACKGROUND_CONTINUATION_MAX_ATTEMPTS; attempt += 1) {
    total += backgroundContinuationDelayMs(attempt)
  }
  return total
}

export const CANVAS_FOREGROUND_WAIT_MS = 6 * 60 * 1000
export const CANVAS_TOTAL_WAIT_MS = CANVAS_FOREGROUND_WAIT_MS + backgroundContinuationTotalMs()
