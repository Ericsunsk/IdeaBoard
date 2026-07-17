import { describe, expect, it } from 'vitest'
import {
  formatFindingDetailsEntries,
  getFindingRiskPresentation,
  getFindingTypePresentation,
} from './reconciliationFindingPresentation'

describe('reconciliation finding presentation', () => {
  it('provides stable human-facing names for the typed finding contract', () => {
    expect(getFindingTypePresentation('wallet_projection_mismatch')).toEqual({
      title: 'Wallet 余额投影不一致',
      summary: '账户余额 projection 与不可变 Ledger 汇总结果不一致。',
    })
  })

  it('falls back safely when a newer backend finding type is not known yet', () => {
    expect(getFindingTypePresentation('future_integrity_check')).toEqual({
      title: '未知账本异常',
      summary: '服务返回了尚未识别的 Finding 类型：future_integrity_check。',
    })
  })

  it('combines severity, risk and isolation into one risk expression', () => {
    expect(
      getFindingRiskPresentation({ severity: 'critical', risk: 'high', isolated: true } as never),
    ).toEqual({ label: '严重 · 高风险 · Wallet 已限制', severity: 'danger' })
    expect(
      getFindingRiskPresentation({ severity: 'warning', risk: 'low', isolated: false } as never),
    ).toEqual({ label: '警告 · 低风险 · 仅报告', severity: 'warn' })
    expect(
      getFindingRiskPresentation({ severity: 'warning', risk: 'low', isolated: true } as never),
    ).toEqual({ label: '警告 · 低风险 · Wallet 已限制', severity: 'danger' })
  })

  it('renders complete nested details without throwing', () => {
    expect(formatFindingDetailsEntries({ task: { id: 'task-1' }, count: 2 })).toEqual([
      { key: 'task', value: '{\n  "id": "task-1"\n}' },
      { key: 'count', value: '2' },
    ])
  })
})
