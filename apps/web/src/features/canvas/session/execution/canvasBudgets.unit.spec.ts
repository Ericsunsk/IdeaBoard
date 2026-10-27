import { describe, expect, it } from 'vitest'
import {
  BACKGROUND_CONTINUATION_INITIAL_DELAY_MS,
  BACKGROUND_CONTINUATION_MAX_ATTEMPTS,
  BACKGROUND_CONTINUATION_MAX_DELAY_MS,
  backgroundContinuationDelayMs,
  backgroundContinuationTotalMs,
  PENDING_IMAGE_MATERIALIZATION_ATTEMPTS,
  SHARED_TASK_REQUEST_ATTEMPTS,
} from './canvasBudgets'

/**
 * 冻结预算断言：数值零变更，只 pin 收敛前的既有预算。
 * 后端对照见 `apps/api/internal/platform/riverpolicy` 预算表：
 * GatewayDefaultTimeout=300s < WorkspaceExecutionBudget()=360s
 * < RiverJobTimeoutDrawing=17min。
 */
describe('canvasBudgets', () => {
  it('freezes the shared quote/submit retry budget at 2 attempts', () => {
    expect(SHARED_TASK_REQUEST_ATTEMPTS).toBe(2)
  })

  it('freezes the materialization retry budget at 3 attempts', () => {
    expect(PENDING_IMAGE_MATERIALIZATION_ATTEMPTS).toBe(3)
  })

  it('freezes the background continuation backoff at 8 rounds of 5s to 60s', () => {
    expect(BACKGROUND_CONTINUATION_MAX_ATTEMPTS).toBe(8)
    expect(BACKGROUND_CONTINUATION_INITIAL_DELAY_MS).toBe(5_000)
    expect(BACKGROUND_CONTINUATION_MAX_DELAY_MS).toBe(60_000)
    expect(
      Array.from({ length: BACKGROUND_CONTINUATION_MAX_ATTEMPTS }, (_, attempt) =>
        backgroundContinuationDelayMs(attempt),
      ),
    ).toEqual([5_000, 10_000, 20_000, 40_000, 60_000, 60_000, 60_000, 60_000])
  })

  it('keeps the background continuation total inside the River outer bound', () => {
    // 后台续总时长 = 5+10+20+40+60*4 = 315s；6min waiter 之后续跑，
    // 前后台合计约 675s，必须 < River 17min（1020s）外层 bound。
    const RIVER_JOB_TIMEOUT_DRAWING_MS = 17 * 60_000
    const FOREGROUND_WAITER_MS = 6 * 60_000
    expect(backgroundContinuationTotalMs()).toBe(315_000)
    expect(backgroundContinuationTotalMs()).toBeLessThan(RIVER_JOB_TIMEOUT_DRAWING_MS)
    expect(FOREGROUND_WAITER_MS + backgroundContinuationTotalMs()).toBeLessThan(
      RIVER_JOB_TIMEOUT_DRAWING_MS,
    )
  })

  it('keeps the backend timeout layering intact (gateway < execution < river)', () => {
    // 后端数值镜像（runtime/ports.GatewayDefaultTimeout=300s、
    // riverpolicy.WorkspaceExecutionBudget()=360s、RiverJobTimeoutDrawing=17min）：
    // 前端只读镜像断言层级，不拥有这些数值。
    const GATEWAY_DEFAULT_TIMEOUT_MS = 300_000
    const WORKSPACE_EXECUTION_BUDGET_MS = 360_000
    const RIVER_JOB_TIMEOUT_DRAWING_MS = 17 * 60_000
    expect(GATEWAY_DEFAULT_TIMEOUT_MS).toBeLessThan(WORKSPACE_EXECUTION_BUDGET_MS)
    expect(WORKSPACE_EXECUTION_BUDGET_MS).toBeLessThan(RIVER_JOB_TIMEOUT_DRAWING_MS)
  })
})
